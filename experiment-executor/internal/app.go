package internal

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/duobitx/yass-internal-components/experiment-executor/internal/geocalc"
	"github.com/duobitx/yass-internal-components/experiment-executor/internal/model"
	"github.com/duobitx/yass-internal-components/go-common/cmodel"
	"github.com/duobitx/yass-internal-components/go-common/com"
	proto "github.com/duobitx/yass-internal-components/go-common/proto/go"
	yassv1 "github.com/duobitx/yass-operator/api/v1"
	"github.com/m-szalik/goutils"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	experimentEndTopic = "experiment/end-request"
)

type AppType struct {
	mainCtx           context.Context
	facade            com.Facade
	ExperimentDefData *cmodel.ExperimentDefinition
	k8sClient         client.Client
	nodes             map[string]*model.FsNodeState
	nodesLock         sync.Mutex
	namespace         string
}

func (t *AppType) handleOnlineUpdate(_ context.Context, data []byte) error {
	msg := &proto.FsNodeOnlineState{}
	err := com.MsgUnmarshall(data, msg)
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
	cfg, err := ctrl.GetConfig()
	if err != nil {
		return nil, err
	}
	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("creating k8s client: %w", err)
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
	experimentCtx, cancel := context.WithCancelCause(ctxParent)
	// do not call cancel by default, we want that to continue.
	err := t.facade.Publish(experimentCtx, experimentEndTopic, 0, true, "")
	if err != nil {
		return errors.Wrapf(err, "cannot publish to %s", experimentEndTopic)
	}
	err = t.facade.Subscribe(experimentEndTopic, func(sCtx context.Context, topic string, retained bool, data []byte) {
		if len(data) > 0 {
			req := &proto.AgentExperimentEndRequest{}
			err := com.MsgUnmarshall(data, req)
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
				if !experimentEndAt.IsZero() && !experimentEndAt.After(lastTime) {
					if err := t.sendTimeUpdate(lastTime, false); err != nil {
						slog.Default().Error("cannot send time update", "error", err)
					}
					slog.Default().Info("experiment time ended", "shouldEndAt", experimentEndAt, "now", lastTime)
					err := t.experimentCompletedUpdateExperimentResource("timeout")
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
	if t.ExperimentDefData.MaxDuration != nil {
		experimentEndAt = startAt.Add(*t.ExperimentDefData.MaxDuration)
	}
	slog.Default().Info("starting experiment", "startTime", startAt, "maxDuration", t.ExperimentDefData.MaxDuration)

	return nil
}

func (t *AppType) sendTimeUpdate(now time.Time, ongoing bool) error {
	obj := &proto.TimeUpdate{
		Ongoing: ongoing,
		Now:     now.UnixMilli(),
	}
	return t.facade.Publish(t.mainCtx, "updates/_time_", 0, true, obj)
}

func (t *AppType) experimentCompletedUpdateExperimentResource(completionReason string) error {
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
		slog.Default().Info(fmt.Sprintf("Experiment Completed, reason: %s", completionReason))
		exp.Status.ExperimentState = yassv1.ExperimentStateCompleted
		// TODO event
		return t.k8sClient.Status().Update(t.mainCtx, exp)
	}
	return nil
}

func (t *AppType) handleGeoUpdate(_ context.Context, upd *geocalc.GeoCalcUpdate) error {
	nowMillis := upd.CurrentTime.UnixMilli()
	jeh := goutils.JoinErrorHelper{}
	for _, data := range upd.FsNodeInfos {
		networkParams := make([]*proto.FsNodeUpdateNetworkParamEntry, len(data.ReachableFsNodes))
		for i := 0; i < len(networkParams); i++ {
			np := &proto.FsNodeUpdateNetworkParamEntry{}
			ipFsState, ok := t.nodes[data.ReachableFsNodes[i].To]
			if !ok {
				// FIXME return fmt.Errorf("cannot resolve IP for fsNode %s, no fsStateEntry", data.ReachableFsNodes[i].To)
			} else {
				np.Ip = ipFsState.IP
			}
			np.Distance = data.ReachableFsNodes[i].Distance
			t.calculateNetworkParam(data, data.ReachableFsNodes[i].To, np)
			networkParams[i] = np
		}
		gr := &proto.FsNodeUpdate{
			Name:              data.Name,
			InShadow:          false, // TODO later v2
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
			}
		}()

		err := t.facade.Publish(t.mainCtx, fmt.Sprintf("updates/%s", data.Name), 0, true, gr)
		jeh.Append(err)
	}
	return jeh.AsError()
}

func (t *AppType) calculateNetworkParam(fsNodeMain *geocalc.FsNodeInfo, dstName string, dst *proto.FsNodeUpdateNetworkParamEntry) {
	// TODO
	_ = fsNodeMain
	dst.Subject = dstName
	dst.PackageDelay = 0.001 /* 1ms for transmitter */ + dst.Distance/300_000.00
	dst.PackageLoss = 0.1 // 10% fixed as for now FIXME calculate
}
