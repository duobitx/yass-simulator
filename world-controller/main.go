package main

import (
	"context"
	"fmt"
	"log/slog"
	"os/signal"
	"sync"
	"syscall"

	"github.com/ESA-PhiLab/yass-internal-components/go-common/com"
	"github.com/ESA-PhiLab/yass-internal-components/go-common/proto"
	"github.com/ESA-PhiLab/yass-internal-components/go-common/startup"
	"github.com/ESA-PhiLab/yass-internal-components/world-controller/consts"
	"github.com/ESA-PhiLab/yass-internal-components/world-controller/internal"
	"github.com/ESA-PhiLab/yass-internal-components/world-controller/internal/model"
	yassv1 "github.com/ESA-PhiLab/yass-operator/api/v1"
	"github.com/m-szalik/goutils"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/json"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type appType struct {
	mainCtx      context.Context
	facade       com.Facade
	k8sClient    client.Client
	fsNodeObjKey client.ObjectKey
	podIP        string
	experiment   string
	nodes        map[string]model.SharedNodeInfo
	nodesLock    sync.Mutex
}

func (a *appType) handleUpdate(ctx context.Context, data []byte) error {
	slog.Info("incoming data", "data", data)
	fsNode := &yassv1.FsNode{}
	err := a.k8sClient.Get(ctx, a.fsNodeObjKey, fsNode)
	if err != nil {
		return err
	}
	fsNode.Status.PosStr = string(data)
	return a.k8sClient.Status().Update(ctx, fsNode)
}

func (a *appType) publishOnlineState(online bool) error {
	onlineUpdateTopic := fmt.Sprintf("online-states/%s", a.fsNodeObjKey.Name)
	podIP := goutils.BoolToStr(online, a.podIP, "")
	slog.Default().Info("Publishing my online state", "IP", podIP, "online", online)
	return a.facade.Publish(a.mainCtx, onlineUpdateTopic, 0, true, proto.FsNodeOnlineState{
		FsNodeId: &proto.FsNodeId{
			Name:       a.fsNodeObjKey.Name,
			Experiment: a.experiment,
		},
		Ip:     podIP,
		Online: online,
	})
}

func (a *appType) updateListOfExperimentNodes(_ context.Context, data []byte) error {
	msg := &proto.FsNodeOnlineState{}
	err := com.MsgUnmarshall(data, msg)
	if err != nil {
		return err
	}
	if msg.FsNodeId.Experiment != a.experiment {
		return nil // ignore as this is not our experiment
	}
	a.nodesLock.Lock()
	defer a.nodesLock.Unlock()
	prevEntry, ok := a.nodes[msg.FsNodeId.Name]
	newEntry := model.SharedNodeInfo{
		IP:         msg.Ip,
		Experiment: msg.FsNodeId.Experiment,
		Online:     msg.Online,
	}
	if !ok || !newEntry.Eq(prevEntry) {
		a.nodes[msg.FsNodeId.Name] = newEntry
		content, err := json.Marshal(a.nodes)
		if err != nil {
			return err
		}
		return internal.SaveInShared("nodes.json", content)
	}
	return nil
}

func main() {
	resourceName := goutils.EnvRequired[string]("RESOURCE_NAME")
	resourceNamespace := goutils.EnvRequired[string]("NAMESPACE")
	slog.Info("World Controller", "namespace", resourceNamespace, "name", resourceName)
	ctx, cancel := signal.NotifyContext(context.WithValue(context.Background(), consts.CtxKeyFsName, resourceName), syscall.SIGTERM)
	defer cancel()
	facade := com.NewFacade(ctx, fmt.Sprintf("%s-%s", resourceName, consts.AppName))
	app := &appType{
		mainCtx:   ctx,
		facade:    facade,
		k8sClient: nil,
		fsNodeObjKey: client.ObjectKey{
			Namespace: resourceNamespace,
			Name:      resourceName,
		},
		podIP:      goutils.EnvRequired[string]("POD_IP"),
		experiment: goutils.Env("YASS_EXPERIMENT", ""),
		nodes:      map[string]model.SharedNodeInfo{},
	}

	err := facade.Connect()
	goutils.ExitOnError(err, 2)

	scheme := runtime.NewScheme()
	err = clientgoscheme.AddToScheme(scheme)
	goutils.ExitOnError(err, 3)
	err = yassv1.AddToScheme(scheme)
	goutils.ExitOnError(err, 3)
	cfg := ctrl.GetConfigOrDie()
	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		panic(fmt.Errorf("creating k8s client: %w", err))
	}
	app.k8sClient = k8sClient

	subscribeUpdateTopic := fmt.Sprintf("/updates/%s", app.fsNodeObjKey.Name)
	slog.Info("Subscribe", "topic", subscribeUpdateTopic)
	err = facade.Subscribe(subscribeUpdateTopic, func(sCtx context.Context, topic string, retained bool, data []byte) {
		err := app.handleUpdate(sCtx, data)
		if err != nil {
			slog.Error("error handling incoming update data", "data", string(data), "topic", topic, "error", err)
		}
	})
	goutils.ExitOnError(err, 4)

	subscribeAllUpdatesTopic := "online-states/#"
	slog.Info("Subscribe", "topic", subscribeAllUpdatesTopic)
	err = facade.Subscribe(subscribeAllUpdatesTopic, func(sCtx context.Context, topic string, retained bool, data []byte) {
		err := app.updateListOfExperimentNodes(sCtx, data)
		if err != nil {
			slog.Error("error handling incoming online updates data", "data", string(data), "topic", topic, "error", err)
		}
	})
	goutils.ExitOnError(err, 4)

	err = app.publishOnlineState(true)
	goutils.ExitOnError(err, 5)
	defer func() {
		err := app.publishOnlineState(false)
		if err != nil {
			slog.Error("cannot publish offline status", "error", err)
		}
	}()

	err = startup.FileProbe(ctx, consts.AppName)
	goutils.ExitOnError(err, 6)
	slog.Info("StartupCompleted")
	<-ctx.Done()
	slog.Info("Terminated")
}
