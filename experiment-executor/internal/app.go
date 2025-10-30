package internal

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/ESA-PhiLab/yass-internal-components/experiment-executor/internal/eclock"
	"github.com/ESA-PhiLab/yass-internal-components/experiment-executor/internal/geocalc"
	"github.com/ESA-PhiLab/yass-internal-components/experiment-executor/internal/model"
	"github.com/ESA-PhiLab/yass-internal-components/go-common/cmodel"
	"github.com/ESA-PhiLab/yass-internal-components/go-common/com"
	"github.com/ESA-PhiLab/yass-internal-components/go-common/proto"
	yassv1 "github.com/ESA-PhiLab/yass-operator/api/v1"
	"github.com/m-szalik/goutils"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type AppType struct {
	mainCtx           context.Context
	facade            com.Facade
	ExperimentDefData *cmodel.ExperimentDefinition
	k8sClient         client.Client
	nodes             map[string]*model.FsNodeState
	nodesLock         sync.Mutex
	clock             eclock.EClock
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

	err = facade.Subscribe("/online-states/#", func(sCtx context.Context, topic string, retained bool, data []byte) {
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
	if t.clock != nil {
		return errors.New("experiment already started")
	}
	ctx, cancel := context.WithCancelCause(ctxParent)
	var experimentEndAt time.Time
	dataCh, errCh := geocalc.RunGeoCalc(ctx, 2*time.Second)
	timeSourceCh := make(chan time.Time)
	go func() {
		var lastTime time.Time
		defer close(timeSourceCh)
		for {
			select {
			case err := <-errCh:
				if err != nil {
					slog.Default().Error("geocalc error", "error", err)
				}
			case upd := <-dataCh:
				timeSourceCh <- upd.CurrentTime
				lastTime = upd.CurrentTime
				if !experimentEndAt.IsZero() && experimentEndAt.After(upd.CurrentTime) {
					if err := t.sendTimeUpdate(upd.CurrentTime, false); err != nil {
						slog.Default().Error("cannot send time update", "error", err)
					}
					cancel(errors.New("experiment time ended"))
				} else {
					if err := t.sendTimeUpdate(upd.CurrentTime, true); err != nil {
						slog.Default().Error("cannot send time update", "error", err)
					}
					if err := t.handleGeoUpdate(ctx, upd); err != nil {
						slog.Default().Error("cannot send geo update", "error", err)
					}
				}
				err := t.experimentCompletedUpdateExperimentResource()
				if err != nil {
					slog.Default().Error("error updating experiment.status resource", "error", err)
				}
			case <-ctx.Done():
				if err := t.sendTimeUpdate(lastTime, false); err != nil {
					slog.Default().Error("cannot send time update after ctx canceled", "error", err)
				}
				return
			}
		}
	}()
	experimentClock, err := eclock.NewExperimentClock(ctx, timeSourceCh, t.ExperimentDefData.MaxDuration)
	if err != nil {
		return errors.Wrapf(err, "NewExperimentClock")
	}
	startAt := experimentClock.Now()
	if t.ExperimentDefData.MaxDuration != nil {
		experimentEndAt = startAt.Add(*t.ExperimentDefData.MaxDuration)
	}
	slog.Default().Info("starting experiment", "startTime", startAt, "maxDuration", t.ExperimentDefData.MaxDuration)
	t.clock = experimentClock
	return nil
}

func (t *AppType) sendGeoUpdate(fsnName string, gr *model.GeoResult) error {
	pos := fmt.Sprintf("x=%.2f,y=%.2f,z=%.2f", gr.X, gr.Y, gr.Z)
	obj := &proto.FsNodeUpdate{
		Id:                fsnName,
		InShadow:          false,
		UpdatedUnixMillis: time.Now().UnixMilli(),
		PosStr:            &pos,
	}
	return t.facade.Publish(t.mainCtx, fmt.Sprintf("updates/%s", fsnName), 0, true, obj)
}

func (t *AppType) sendTimeUpdate(now time.Time, ongoing bool) error {
	obj := &proto.TimeUpdate{
		Ongoing: ongoing,
		Now:     now.UnixMilli(),
	}
	return t.facade.Publish(t.mainCtx, "updates/_time_", 0, true, obj)
}

func (t *AppType) experimentCompletedUpdateExperimentResource() error {
	if t.namespace == "" {
		slog.Default().Warn("namespace not set")
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
		exp.Status.ExperimentState = yassv1.ExperimentStateCompleted
		return t.k8sClient.Status().Update(t.mainCtx, exp)
	}
	return nil
}

func (t *AppType) handleGeoUpdate(_ context.Context, upd *geocalc.GeoCalcUpdate) error {
	for _, data := range upd.FsNodeInfos {
		networkParams := make([]*model.GeoResultNetworkParamEntry, len(data.ReachableFsNodes))
		for i := 0; i < len(networkParams); i++ {
			np := &model.GeoResultNetworkParamEntry{}
			ipFsState, ok := t.nodes[data.ReachableFsNodes[i].To]
			if !ok {
				return fmt.Errorf("cannot resolve IP for fsNode %s, no fsStateEntry", data.ReachableFsNodes[i].To)
			}
			np.IP = ipFsState.IP
			np.Distance = data.ReachableFsNodes[i].Distance
			err := t.calculateNetworkParam(data, data.ReachableFsNodes[i].To, np)
			if err != nil {
				return errors.Wrapf(err, "cannot calculate network param between %+v for %+v", data, np)
			}
			networkParams[i] = np
		}
		gr := &model.GeoResult{
			X:             data.X,
			Y:             data.Y,
			Z:             data.Z,
			Alt:           data.Alt,
			NetworkParams: networkParams,
		}
		err := t.sendGeoUpdate(data.Name, gr)
		if err != nil {
			return errors.Wrapf(err, "cannot send geoUpdate to %s", data.Name)
		}
	}
	return nil
}

func (t *AppType) calculateNetworkParam(fsNodeMain *geocalc.FsNodeInfo, dstName string, dst *model.GeoResultNetworkParamEntry) error {
	// TODO
	_ = fsNodeMain
	_ = dstName
	dst.Delay = 0.01 * dst.Distance
	dst.PackageLoss = 0.01
	return nil
}
