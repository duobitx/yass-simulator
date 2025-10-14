package internal

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"sync"
	"time"

	"github.com/ESA-PhiLab/yass-internal-components/experiment-executor/internal/eclock"
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

func (t *AppType) Start() error {
	if t.clock != nil {
		return errors.New("experiment already started")
	}
	whenStart := time.Now()
	if t.ExperimentDefData.StartTime != nil {
		whenStart = *t.ExperimentDefData.StartTime
	}
	slog.Default().Info("starting experiment", "startTime", whenStart, "maxDuration", t.ExperimentDefData.MaxDuration)
	t.clock = eclock.NewExperimentRealClock(t.mainCtx, whenStart, t.ExperimentDefData.MaxDuration)
	go func() {
		for {
			select {
			case <-t.clock.Done():
				err := t.sendTimeUpdate(t.clock.Now(), false)
				if err != nil {
					slog.Default().Error("error sending final time update", "error", err)
				}
				err = t.experimentCompletedUpdateExperimentResource()
				if err != nil {
					slog.Default().Error("error updating experiment.status resource", "error", err)
				}
				return
			case now := <-t.clock.Tick():
				err := t.sendUpdates(now)
				if err != nil {
					slog.Default().Error("error sending mock update", "error", err)
				}
			}
		}
	}()
	return nil
}

func (t *AppType) sendUpdates(now time.Time) error {
	t.nodesLock.Lock()
	defer t.nodesLock.Unlock()
	joinErr := &goutils.JoinErrorHelper{}
	wg := sync.WaitGroup{}
	for _, fsNode := range t.ExperimentDefData.FsNodes {
		name := fsNode.Name
		geoResult, err := calculatePosition(&fsNode, now) // no multithread is supported
		if err != nil {
			joinErr.Append(errors.Wrapf(err, "cannot calculate position for %s", name))
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			err2 := t.sendGeoUpdate(name, geoResult)
			if err2 != nil {
				joinErr.Append(errors.Wrapf(err2, "cannot send geoUpdate to %s", name))
			}
		}()
		wg.Wait()
		err = t.sendTimeUpdate(now, true)
		joinErr.Append(err)
	}
	return joinErr.AsError()
}

func calculatePosition(fsn *cmodel.ExperimentFsNode, t time.Time) (*model.GeoResult, error) {
	// return nil, errors.New("calculatePosition:: implement me")
	// TODO
	return &model.GeoResult{
		X: rand.Float32() * 10.0,
		Y: rand.Float32() * 10.0,
		Z: rand.Float32() * 10.0,
	}, nil
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
