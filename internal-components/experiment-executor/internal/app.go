package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/duobitx/yass-simulator/internal-components/experiment-executor/internal/geocalc"
	"github.com/duobitx/yass-simulator/internal-components/experiment-executor/internal/model"
	"github.com/duobitx/yass-simulator/internal-components/go-common/cmodel"
	proto "github.com/duobitx/yass-simulator/internal-components/go-common/proto/go"
	yassv1 "github.com/duobitx/yass-simulator/yass-operator/api/v1"
	com "github.com/m-szalik/com-facade"
	"github.com/m-szalik/goutils"
	"github.com/pkg/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	experimentEndTopic = "experiment/end-request"
)

type AppType struct {
	mainCtx             context.Context
	facade              com.Facade
	ExperimentDefData   *cmodel.ExperimentDefinition
	k8sClient           client.Client
	nodes               map[string]*model.FsNodeState
	nodesLock           sync.Mutex
	namespace           string
	experimentStartedAt *time.Time
	experimentTime      *time.Time
}

func (t *AppType) handleOnlineUpdate(_ context.Context, data []byte) error {
	msg := &proto.FsNodeOnlineState{}
	err := json.Unmarshal(data, msg)
	if err != nil {
		return err
	}
	if t.ExperimentDefData.Name != msg.FsNodeId.Experiment {
		return nil
	}
	t.nodesLock.Lock()
	defer t.nodesLock.Unlock()
	if state, ok := t.nodes[msg.FsNodeId.Name]; !ok {
		state = &model.FsNodeState{
			Online: msg.Online,
			IP:     msg.Ip,
		}
		t.nodes[msg.FsNodeId.Name] = state
	} else {
		state.Online = msg.Online
		state.IP = msg.Ip
	}
	return nil
}

func NewApp(ctx context.Context, facade com.Facade) (*AppType, error) {
	expData, err := LoadExperimentJson()
	if err != nil {
		return nil, errors.Wrapf(err, "cannot load experiment json data")
	}
	scheme := runtime.NewScheme()
	err = clientgoscheme.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}
	err = yassv1.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}

	var k8sClient client.Client
	if goutils.Env("MOCK_K8S", false) {
		slog.Info("Using fake k8s client")
		k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()
	} else {
		cfg, err := ctrl.GetConfig()
		if err != nil {
			return nil, err
		}
		k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
		if err != nil {
			return nil, fmt.Errorf("creating k8s client: %w", err)
		}
	}

	app := &AppType{
		mainCtx:           ctx,
		facade:            facade,
		ExperimentDefData: expData,
		k8sClient:         k8sClient,
		nodes:             map[string]*model.FsNodeState{},
		nodesLock:         sync.Mutex{},
		namespace:         goutils.Env("NAMESPACE", ""),
	}

	err = facade.Subscribe("online-states/#", func(sCtx context.Context, topic string, retained bool, data []byte) {
		err := app.handleOnlineUpdate(sCtx, data)
		if err != nil {
			slog.Error("error handling incoming update data", "data", string(data), "topic", topic, "error", err)
		}
	})
	if err != nil {
		return nil, err
	}
	return app, nil
}

func (t *AppType) Start(ctxParent context.Context) error {
	if t.experimentStartedAt != nil {
		return errors.New("experiment already started")
	}
	experimentCtx, cancel := context.WithCancelCause(ctxParent)
	// do not call cancel by default, we want that to continue.
	err := t.facade.Publish(experimentCtx, experimentEndTopic, 0, true, "")
	if err != nil {
		cancel(err)
		return errors.Wrapf(err, "cannot publish to %s", experimentEndTopic)
	}
	err = t.facade.Subscribe(experimentEndTopic, func(sCtx context.Context, topic string, retained bool, data []byte) {
		if len(data) > 0 {
			req := &proto.AgentExperimentEndRequest{}
			err := json.Unmarshal(data, req)
			if err != nil {
				slog.Default().Warn("cannot unmarshal content from topic", "topic", experimentEndTopic, "error", err)
				return
			}
			var endErr *ExperimentEndError
			switch req.Status {
			case proto.Status_EXPERIMENT_END_REQUEST_FAILURE:
				endErr = NewExperimentEndErrorWithComment(ExperimentEndDueToScenarioFailure, req.Comment)
			case proto.Status_EXPERIMENT_END_REQUEST_SUCCESS:
				endErr = NewExperimentEndErrorWithComment(ExperimentEndDueToScenarioSuccess, req.Comment)
			default:
				endErr = NewExperimentEndErrorWithCause(ExperimentEndDueToUnexpectedError, fmt.Errorf("unsupported value fro req.Status - %d", req.Status))
			}
			if endErr != nil {
				cancel(endErr)
			}
		}
	})
	if err != nil {
		return errors.Wrapf(err, "cannot subscribe to %s", experimentEndTopic)
	}
	var experimentEndAt time.Time // experiment time
	dataCh, errCh := geocalc.RunGeoCalc(experimentCtx, 5*time.Second)
	go func() {
		var lastTime time.Time
		for {
			select {
			case err := <-errCh:
				if err != nil {
					slog.Default().Error("geocalc error", "error", err)
				}
			case upd := <-dataCh:
				lastTime = upd.CurrentTime
				t.experimentTime = &lastTime
				if !experimentEndAt.IsZero() && !experimentEndAt.After(lastTime) {
					if err := t.sendTimeUpdate(lastTime, false); err != nil {
						slog.Default().Error("cannot send time update", "error", err)
					}
					slog.Default().Info("experiment time ended", "shouldEndAt", experimentEndAt, "now", lastTime)
					err := t.experimentTimedOutUpdateExperimentResource()
					if err != nil {
						slog.Default().Error("error updating experiment.status resource", "error", err)
					}
					cancel(NewExperimentEndError(ExperimentEndDueToTimeout))
				} else {
					if err := t.sendTimeUpdate(upd.CurrentTime, true); err != nil {
						slog.Default().Error("cannot send time update", "error", err)
					}
					if err := t.handleGeoUpdate(experimentCtx, upd); err != nil {
						slog.Default().Error("cannot send geo update", "error", err)
					}
				}
			case <-experimentCtx.Done():
				if err := t.sendTimeUpdate(lastTime, false); err != nil {
					slog.Default().Error("cannot send time update after experimentCtx canceled", "error", err)
				}
				return
			}
		}
	}()
	startAt := time.Now()
	if t.ExperimentDefData.StartTime != nil {
		startAt = *t.ExperimentDefData.StartTime
	}
	t.experimentStartedAt = &startAt
	if t.ExperimentDefData.MaxDuration != nil {
		experimentEndAt = startAt.Add(*t.ExperimentDefData.MaxDuration)
	}
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-experimentCtx.Done():
				return
			case <-ticker.C:
				err := t.updateK8sResource()
				if err != nil {
					slog.Error("error updating experiment time", "error", err)
				}
			}
		}
	}()
	slog.Default().Info("starting experiment", "startTime", startAt, "maxDuration", t.ExperimentDefData.MaxDuration)
	t.experimentStartedAt = &startAt
	return nil
}

func (t *AppType) sendTimeUpdate(now time.Time, ongoing bool) error {
	obj := &proto.TimeUpdate{
		Ongoing: ongoing,
		Now:     now.UnixMilli(),
	}
	return t.facade.Publish(t.mainCtx, "updates/_time_", 0, true, obj)
}

func (t *AppType) experimentTimedOutUpdateExperimentResource() error {
	if t.namespace == "" {
		slog.Default().Warn("namespace not set, experiment resource will not be updated")
		return nil
	}
	exp := &yassv1.Experiment{}
	err := t.k8sClient.Get(t.mainCtx, client.ObjectKey{
		Namespace: t.namespace,
		Name:      t.ExperimentDefData.Name,
	}, exp)
	if err != nil {
		return err
	}
	if exp.Status.ExperimentState == yassv1.ExperimentStateOngoing {
		slog.Default().Info("Experiment timed out")
		exp.Status.ExperimentState = yassv1.ExperimentStateTimedOut
		return t.k8sClient.Status().Update(t.mainCtx, exp)
	}
	return nil
}

func (t *AppType) handleGeoUpdate(_ context.Context, upd *geocalc.GlobalGeoCalcUpdate) error {
	nowMillis := upd.CurrentTime.UnixMilli()
	jeh := goutils.JoinErrorHelper{}
	for _, data := range upd.FsNodeInfos {
		networkParams := make([]*proto.FsNodeUpdateNetworkParamEntry, 0, len(data.ReachableFsNodes))
		for _, peer := range data.ReachableFsNodes {
			ipFsState, ok := t.nodes[peer.NameTo]
			if !ok {
				// Peer hasn't published its online-state on MQTT yet; skip until it does.
				slog.Default().Warn("cannot resolve IP for fsNode", "fsNode", peer.NameTo, "processingFsNode", data.Name)
				continue
			}
			np := &proto.FsNodeUpdateNetworkParamEntry{
				Ip:       ipFsState.IP,
				Distance: peer.Distance,
			}
			t.calculateNetworkParam(data, peer.NameTo, np)
			networkParams = append(networkParams, np)
		}
		gr := &proto.FsNodeUpdate{
			Name:              data.Name,
			InShadow:          data.InShadow,
			PosStr:            fmt.Sprintf("lat=%.2f,lng=%.2f", data.Lat, data.Lng),
			X:                 data.X,
			Y:                 data.Y,
			Z:                 data.Z,
			Lat:               data.Lat,
			Lng:               data.Lng,
			Alt:               data.Alt,
			NetworkParams:     networkParams,
			UpdatedUnixMillis: nowMillis,
		}
		func() {
			t.nodesLock.Lock()
			defer t.nodesLock.Unlock()
			if state, ok := t.nodes[data.Name]; ok {
				state.PosX = data.X
				state.PosY = data.Y
				state.PosZ = data.Z
				state.Lat = data.Lat
				state.Lng = data.Lng
				state.Alt = data.Alt
				state.InShadow = data.InShadow
			}
		}()

		err := t.facade.Publish(t.mainCtx, fmt.Sprintf("updates/%s", data.Name), 0, true, gr)
		jeh.Append(err)
	}
	return jeh.AsError()
}

// Reference parameters for distance-dependent link metrics.
const (
	bandwidthMaxBps      = 100 * 1024 * 1024 // 100 Mbit/s at or below dRefKm
	bandwidthMinBps      = 100 * 1024        // 100 kbit/s floor
	bandwidthRefDistance = 1000.0            // km — distance at which we still get full bandwidth
	transmitterDelayMs   = 1.0               // fixed transmitter delay in ms
	speedOfLightKmPerMs  = 300.0             // km per millisecond
	packageLossBase      = 0.001             // 0.1% baseline loss
	packageLossSlope     = 0.001             // per (d/d_ref)^2 unit
	packageLossMax       = 0.5               // 50% cap; above that link is effectively dead
)

func (t *AppType) calculateNetworkParam(fsNodeMain *geocalc.FsNodeInfo, dstName string, dst *proto.FsNodeUpdateNetworkParamEntry) {
	_ = fsNodeMain
	dst.Subject = dstName
	dst.PackageDelay = transmitterDelayMs + dst.Distance/speedOfLightKmPerMs
	// Loss grows quadratically with distance to mimic worse SNR over longer hops.
	distRatio := float64(dst.Distance) / float64(bandwidthRefDistance)
	loss := packageLossBase + packageLossSlope*distRatio*distRatio
	if loss > packageLossMax {
		loss = packageLossMax
	}
	dst.PackageLoss = float32(loss)
	// Bandwidth ~ (d_ref / d)^2 to mimic free-space path loss, capped at bandwidthMaxBps for d <= d_ref.
	bwRatio := float64(bandwidthRefDistance) / math.Max(float64(dst.Distance), float64(bandwidthRefDistance))
	bw := float64(bandwidthMaxBps) * bwRatio * bwRatio
	if bw < bandwidthMinBps {
		bw = bandwidthMinBps
	}
	dst.Bandwidth = float32(bw)
}

func (t *AppType) updateK8sResource() error {
	if t.experimentTime != nil {
		exp := &yassv1.Experiment{}
		err := t.k8sClient.Get(t.mainCtx, client.ObjectKey{
			Namespace: t.namespace,
			Name:      t.ExperimentDefData.Name,
		}, exp)
		if err != nil {
			return err
		}
		exp.Status.ExperimentTime = v1.Time{Time: *t.experimentTime}
		return t.k8sClient.Status().Update(t.mainCtx, exp)
	}
	return errors.New("experimentTime is not set")
}
