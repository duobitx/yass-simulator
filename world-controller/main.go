package main

import (
	"context"
	"fmt"
	"log/slog"
	"os/signal"
	"sync"
	"syscall"
	"time"

	proto "github.com/duobitx/yass-internal-components/go-common/proto/go"
	"github.com/duobitx/yass-internal-components/go-common/startup"
	"github.com/duobitx/yass-internal-components/world-controller/consts"
	"github.com/duobitx/yass-internal-components/world-controller/internal"
	"github.com/duobitx/yass-internal-components/world-controller/internal/hw"
	"github.com/duobitx/yass-internal-components/world-controller/internal/model"
	"github.com/duobitx/yass-internal-components/world-controller/internal/networking"
	yassv1 "github.com/duobitx/yass-operator/api/v1"
	com "github.com/m-szalik/com-facade"
	"github.com/m-szalik/com-facade/mqtt"
	"github.com/m-szalik/goutils"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/apimachinery/pkg/util/rand"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type updates struct {
	posStr     string
	batteryStr string
}
type appType struct {
	mainCtx           context.Context
	facade            com.Facade
	k8sClient         client.Client
	fsNodeObjKey      client.ObjectKey
	podIP             string
	experiment        string
	nodes             map[string]model.SharedNodeInfo
	nodesLock         sync.Mutex
	networkingHandler *networking.Handler
	hw                *hw.NodeHwState
	updates           *updates
}

func (a *appType) handleUpdate(_ context.Context, data []byte) error {
	slog.Info("incoming data", "data", data)
	dataObj := &proto.FsNodeUpdate{}
	err := json.Unmarshal(data, &dataObj)
	if err != nil {
		return err
	}
	a.hw.InShadow = dataObj.InShadow
	a.updates.posStr = dataObj.PosStr
	jeh := goutils.JoinErrorHelper{}
	networkParams := goutils.SliceMap[*proto.FsNodeUpdateNetworkParamEntry, networking.NetworkParam](
		dataObj.NetworkParams,
		func(entry *proto.FsNodeUpdateNetworkParamEntry) networking.NetworkParam {
			return networking.NetworkParamFromFsNodeUpdateNetworkParamEntry(entry)
		},
	)
	if err = a.networkingHandler.Update(networkParams); err != nil {
		slog.Warn("Error updating networking rules", "params", networkParams, "error", err)
		jeh.Append(err)
	}
	return jeh.AsError()
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
	err := json.Unmarshal(data, msg)
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
	goutils.ExitOnErrorf(startup.InitSlog(), 1, "cannot initialize slog")
	resourceName := goutils.EnvRequired[string]("RESOURCE_NAME")
	resourceNamespace := goutils.EnvRequired[string]("NAMESPACE")
	slog.Info("World Controller", "namespace", resourceNamespace, "name", resourceName)
	ctxWithName := context.WithValue(context.Background(), consts.CtxKeyFsName, resourceName)
	ctx, cancel := signal.NotifyContext(ctxWithName, syscall.SIGTERM, syscall.SIGINT)
	defer cancel()
	facade := mqtt.NewFacade(ctx, fmt.Sprintf("%s-%s-%d", resourceName, consts.AppName, rand.Intn(100)),
		mqtt.WithHostPort("tcp://"+goutils.Env("MESSAGING_BROKER_HOST_PORT", "messaging:1883")))
	disableNetworking := goutils.Env("DISABLE_NETWORKING_MANIPULATION", false)
	slog.Info("Networking manipulation", "disabled", disableNetworking)
	networkingHandler, err := networking.NewNetworkHandler(disableNetworking)
	goutils.ExitOnErrorf(err, 1, "cannot create Handler")
	hwSpec, err := hw.Read()
	goutils.ExitOnErrorf(err, 2, "cannot read hw spec")

	app := &appType{
		mainCtx:   ctx,
		facade:    facade,
		k8sClient: nil,
		fsNodeObjKey: client.ObjectKey{
			Namespace: resourceNamespace,
			Name:      resourceName,
		},
		podIP:             goutils.EnvRequired[string]("POD_IP"),
		experiment:        goutils.Env("YASS_EXPERIMENT", ""),
		nodes:             map[string]model.SharedNodeInfo{},
		networkingHandler: networkingHandler,
		hw:                hw.NewNodeHwState(hwSpec),
		updates: &updates{
			posStr:     "unspecified",
			batteryStr: "unspecified",
		},
	}

	err = facade.Connect()
	goutils.ExitOnError(err, 2)

	scheme := runtime.NewScheme()
	err = clientgoscheme.AddToScheme(scheme)
	goutils.ExitOnError(err, 3)
	err = yassv1.AddToScheme(scheme)
	goutils.ExitOnError(err, 4)

	var k8sClient client.Client
	if goutils.Env("MOCK_K8S", false) {
		slog.Info("Using fake k8s client")
		k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()
	} else {
		cfg := ctrl.GetConfigOrDie()
		k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
		if err != nil {
			panic(fmt.Errorf("creating k8s client: %w", err))
		}
	}
	app.k8sClient = k8sClient

	subscribeUpdateTopic := fmt.Sprintf("updates/%s", app.fsNodeObjKey.Name)
	slog.Info("Subscribe", "topic", subscribeUpdateTopic)
	err = facade.Subscribe(subscribeUpdateTopic, func(sCtx context.Context, topic string, retained bool, data []byte) {
		err := app.handleUpdate(sCtx, data)
		if err != nil {
			slog.Error("error handling incoming update data", "data", string(data), "topic", topic, "error", err)
		}
	})
	goutils.ExitOnError(err, 5)

	subscribeAllUpdatesTopic := "online-states/#"
	slog.Info("Subscribe", "topic", subscribeAllUpdatesTopic)
	err = facade.Subscribe(subscribeAllUpdatesTopic, func(sCtx context.Context, topic string, retained bool, data []byte) {
		err := app.updateListOfExperimentNodes(sCtx, data)
		if err != nil {
			slog.Error("error handling incoming online updates data", "data", string(data), "topic", topic, "error", err)
		}
	})
	goutils.ExitOnError(err, 6)

	err = app.publishOnlineState(true)
	goutils.ExitOnError(err, 7)
	defer func() {
		err := app.publishOnlineState(false)
		if err != nil {
			slog.Error("cannot publish offline status", "error", err)
		}
	}()

	internal.BackgroundPeriodicTask(ctx, 1*time.Second, func() {
		stats, err := networkingHandler.GetTrafficStats()
		if err != nil {
			slog.Error("cannot get networks stats", "error", err)
			return
		}
		buff, err := json.Marshal(stats)
		if err != nil {
			slog.Error("cannot get marshal stats", "error", err)
			return
		}
		topic := fmt.Sprintf("total-network-stats/%s", app.fsNodeObjKey.Name)
		err = facade.Publish(ctx, topic, 0, false, buff)
		if err != nil {
			slog.Error("cannot publish to topic", "error", err, "topic", topic)
			return
		}
	})

	internal.BackgroundPeriodicTask(ctx, 10*time.Second, func() {
		networkStats, err := networkingHandler.GetTrafficStats()
		if err != nil {
			slog.Error("cannot get network stats", "error", err)
		}
		data, statusStr, err := app.hw.Update(networkStats)
		if err != nil {
			slog.Error("cannot update energy stats", "error", err)
			return
		}
		app.updates.batteryStr = statusStr
		topic := fmt.Sprintf("energy/%s", app.fsNodeObjKey.Name)
		err = facade.Publish(ctx, topic, 0, false, data)
		if err != nil {
			slog.Error("cannot publish to topic", "error", err, "topic", topic)
			return
		}
	})

	internal.BackgroundPeriodicTask(ctx, 5*time.Second, func() {
		node := yassv1.FsNode{}
		err = app.k8sClient.Get(ctx, app.fsNodeObjKey, &node)
		if err != nil {
			slog.Error("cannot get node object", "error", err)
			return
		}
		node.Status.EnergyConsumptionStr = app.updates.batteryStr
		node.Status.PosStr = app.updates.posStr
		err = app.k8sClient.Status().Update(ctx, &node)
		if err != nil {
			slog.Error("cannot update node status", "error", err)
		}
	})

	err = startup.FileProbe(ctx)
	goutils.ExitOnError(err, 8)
	startup.HttpProbe(ctx, 8801)
	slog.Info("StartupCompleted")
	<-ctx.Done()
	time.Sleep(1 * time.Second)
	slog.Info("Terminated")
}
