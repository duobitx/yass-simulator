package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	proto "github.com/duobitx/yass-simulator/internal-components/go-common/proto/go"
	"github.com/duobitx/yass-simulator/internal-components/go-common/startup"
	"github.com/duobitx/yass-simulator/internal-components/world-controller/consts"
	"github.com/duobitx/yass-simulator/internal-components/world-controller/internal"
	"github.com/duobitx/yass-simulator/internal-components/world-controller/internal/hw"
	"github.com/duobitx/yass-simulator/internal-components/world-controller/internal/hwevents"
	"github.com/duobitx/yass-simulator/internal-components/world-controller/internal/model"
	"github.com/duobitx/yass-simulator/internal-components/world-controller/internal/networking"
	"github.com/duobitx/yass-simulator/internal-components/world-controller/internal/resources"
	yassv1 "github.com/duobitx/yass-simulator/yass-operator/api/v1"
	com "github.com/m-szalik/com-facade"
	"github.com/m-szalik/com-facade/mqtt"
	"github.com/m-szalik/goutils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/apimachinery/pkg/util/rand"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type updates struct {
	mu         sync.Mutex
	posStr     string
	batteryStr string
}

func (u *updates) setPos(s string) {
	u.mu.Lock()
	u.posStr = s
	u.mu.Unlock()
}

func (u *updates) setBattery(s string) {
	u.mu.Lock()
	u.batteryStr = s
	u.mu.Unlock()
}

func (u *updates) snapshot() (pos, battery string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.posStr, u.batteryStr
}
type appType struct {
	mainCtx           context.Context
	facade            com.Facade
	k8sClient         client.Client
	fsNodeObjKey      client.ObjectKey
	podIP             string
	experiment        string
	nodeType          string
	nodes             map[string]model.SharedNodeInfo
	nodesLock         sync.Mutex
	networkingHandler *networking.Handler
	hw                *hw.NodeHwState
	updates           *updates
	// started is closed on the first position update from the experiment-executor,
	// which only arrives once the experiment has actually started (post /start).
	// Hardware-event injection is gated on it so faults never fire during startup.
	started     chan struct{}
	startedOnce sync.Once

	// destroyed is set by the hwevents Manager when a Destroy event fires. While
	// set, agentVerdict reports a terminal non-errored phase, because Destroy is
	// the one fault where the agent's exit code / sentinel must be ignored.
	destroyed atomic.Bool
}

// signalStarted marks the experiment as started (first sim update received).
func (a *appType) signalStarted() {
	a.startedOnce.Do(func() { close(a.started) })
}

func (a *appType) handleUpdate(_ context.Context, data []byte) error {
	slog.Info("incoming data", "data", data)
	dataObj := &proto.FsNodeUpdate{}
	err := json.Unmarshal(data, &dataObj)
	if err != nil {
		return err
	}
	a.hw.SetInShadow(dataObj.InShadow)
	a.updates.setPos(dataObj.PosStr)
	a.signalStarted() // first sim update ⇒ experiment has started ⇒ faults may begin
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
		Ip:       podIP,
		Online:   online,
		NodeType: a.nodeType,
	})
}

func (a *appType) fillPeerFsNode(stats []*proto.TrafficStats) {
	if len(stats) == 0 {
		return
	}
	a.nodesLock.Lock()
	defer a.nodesLock.Unlock()
	for _, s := range stats {
		if s == nil || s.Ip == "" {
			continue
		}
		for name, info := range a.nodes {
			if info.IP == s.Ip {
				s.PeerFsNode = name
				break
			}
		}
	}
}

func (a *appType) updateListOfExperimentNodes(_ context.Context, data []byte) error {
	msg := &proto.FsNodeOnlineState{}
	err := json.Unmarshal(data, msg)
	if err != nil {
		return err
	}
	if msg.FsNodeId == nil {
		return nil // malformed/foreign message without an FsNode id
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

const agentContainerName = "agent"

// agentVerdict decides the FsNode terminal phase from the agent's exit signals,
// honouring this priority (success-criteria spec):
//  1. agent.exit.ok present        → MissionCompleted (agent may keep running)
//  2. agent.exit.failure present   → MissionFail      (agent may keep running)
//  3. no sentinel, agent exited 0  → MissionCompleted
//  4. no sentinel, agent exited ≠0 → Errored
//  5. no sentinel, agent running   → no verdict yet (decided=false)
//
// The sentinel always wins over the exit code (an .ok agent that later exits
// non-zero is still a success). text is the optional reason for the event.
func (a *appType) agentVerdict(ctx context.Context, dir string) (phase yassv1.FsNodePhase, text string, decided bool) {
	// Destroy is the one fault where the agent's exit code / sentinel is
	// irrelevant: a SIGKILLed agent must NOT mark the node (and thus the whole
	// experiment) Errored. Report a terminal, non-errored phase instead.
	if a.destroyed.Load() {
		return yassv1.FsNodePhaseMissionCompleted, "node destroyed by hardware event", true
	}
	if b, err := os.ReadFile(dir + "/agent.exit.ok"); err == nil {
		return yassv1.FsNodePhaseMissionCompleted, strings.TrimSpace(string(b)), true
	}
	for _, n := range []string{"/agent.exit.failure", "/agent.exit.fail"} { // .fail kept for back-compat
		if b, err := os.ReadFile(dir + n); err == nil {
			return yassv1.FsNodePhaseMissionFail, strings.TrimSpace(string(b)), true
		}
	}
	if code, terminated := a.agentExitCode(ctx); terminated {
		if code == 0 {
			return yassv1.FsNodePhaseMissionCompleted, "agent exited 0 without a sentinel", true
		}
		return yassv1.FsNodePhaseErrored, fmt.Sprintf("agent exited %d without a sentinel", code), true
	}
	return "", "", false
}

// agentExitCode returns the agent container's terminated exit code from the own
// Pod (named after the FsNode), and whether the agent container has terminated.
func (a *appType) agentExitCode(ctx context.Context) (int32, bool) {
	var pod corev1.Pod
	if err := a.k8sClient.Get(ctx, a.fsNodeObjKey, &pod); err != nil {
		return 0, false
	}
	for i := range pod.Status.ContainerStatuses {
		cs := &pod.Status.ContainerStatuses[i]
		if cs.Name == agentContainerName && cs.State.Terminated != nil {
			return cs.State.Terminated.ExitCode, true
		}
	}
	return 0, false
}

// applyAgentPhase sets the FsNode terminal phase and emits a Kubernetes event
// on the FsNode (with the optional reason text). Returns an error if the phase
// could not be persisted (the caller retries on the next tick).
func (a *appType) applyAgentPhase(ctx context.Context, phase yassv1.FsNodePhase, text string) error {
	reason, etype := "AgentCompleted", corev1.EventTypeNormal
	switch phase {
	case yassv1.FsNodePhaseMissionFail:
		reason, etype = "AgentFailed", corev1.EventTypeWarning
	case yassv1.FsNodePhaseErrored:
		reason, etype = "AgentErrored", corev1.EventTypeWarning
	}
	var fsNode yassv1.FsNode
	// Re-Get + Update under optimistic-concurrency retry: at n100/n200 scale the
	// operator reconciles the same FsNode concurrently, so a single Status Update
	// frequently hits a resourceVersion conflict — without retry the terminal
	// phase is silently lost and the experiment never completes.
	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := a.k8sClient.Get(ctx, a.fsNodeObjKey, &fsNode); err != nil {
			return err
		}
		fsNode.Status.Phase = phase
		return a.k8sClient.Status().Update(ctx, &fsNode)
	}); err != nil {
		slog.Error("cannot update FsNode terminal phase", "error", err, "phase", string(phase))
		return err
	}
	slog.Info("FsNode terminal phase set", "phase", string(phase), "detail", text)
	msg := string(phase)
	if text != "" {
		msg = fmt.Sprintf("%s: %s", phase, text)
	}
	now := metav1.Now()
	ev := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{GenerateName: fsNode.Name + ".", Namespace: fsNode.Namespace},
		InvolvedObject: corev1.ObjectReference{
			APIVersion: yassv1.GroupVersion.String(), Kind: "FsNode",
			Namespace: fsNode.Namespace, Name: fsNode.Name, UID: fsNode.UID,
		},
		Reason:         reason,
		Message:        msg,
		Type:           etype,
		Source:         corev1.EventSource{Component: "world-controller"},
		FirstTimestamp: now,
		LastTimestamp:  now,
		Count:          1,
	}
	if err := a.k8sClient.Create(ctx, ev); err != nil {
		slog.Error("cannot create FsNode event", "error", err)
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
		started: make(chan struct{}),
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

	// Learn our own node type (satellite / groundStation) so it can be
	// stamped onto online-state messages; metrics-bridge uses it to tell SAT
	// from GS (e.g. to count ground deliveries).
	{
		var self yassv1.FsNode
		if err := app.k8sClient.Get(ctx, app.fsNodeObjKey, &self); err != nil {
			slog.Warn("cannot read own FsNode for node type", "error", err)
		} else {
			app.nodeType = string(self.Spec.NodeType)
		}
	}

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
		app.fillPeerFsNode(stats)
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

	internal.BackgroundPeriodicTask(ctx, 1*time.Second, func() {
		ifaceStats, err := networkingHandler.GetInterfaceTotals()
		if err != nil {
			slog.Error("cannot get interface totals", "error", err)
			return
		}
		if ifaceStats == nil {
			return
		}
		buff, err := json.Marshal(ifaceStats)
		if err != nil {
			slog.Error("cannot marshal interface totals", "error", err)
			return
		}
		topic := fmt.Sprintf("interface-stats/%s", app.fsNodeObjKey.Name)
		if err := facade.Publish(ctx, topic, 0, false, buff); err != nil {
			slog.Error("cannot publish to topic", "error", err, "topic", topic)
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
		app.updates.setBattery(statusStr)
		// Deprecated topic — superseded by `<fsNode>/resources`. See ../TOPICS.md.
		topic := fmt.Sprintf("energy/%s", app.fsNodeObjKey.Name)
		err = facade.Publish(ctx, topic, 0, false, data)
		if err != nil {
			slog.Error("cannot publish to topic", "error", err, "topic", topic)
			return
		}
	})

	resPublisher := resources.NewPublisher(app.fsNodeObjKey.Name, app.fsNodeObjKey.Namespace, app.k8sClient, app.hw, hwSpec)
	const resourcesPeriod = 1 * time.Second
	internal.BackgroundPeriodicTask(ctx, resourcesPeriod, func() {
		snap := resPublisher.Snapshot(ctx, resourcesPeriod.Seconds())
		topic := fmt.Sprintf("%s/resources", app.fsNodeObjKey.Name)
		if err := facade.Publish(ctx, topic, 0, false, snap); err != nil {
			slog.Error("cannot publish to topic", "error", err, "topic", topic)
		}
	})

	// Reflect the agent's outcome into the FsNode status + a Kubernetes event.
	// Language-agnostic contract (success-criteria spec): the agent writes
	// agent.exit.ok (success) or agent.exit.failure (deliberate failure) into
	// AGENT_EXIT_DIR — it may keep running afterwards. Absent any sentinel, the
	// verdict falls back to the agent container's exit code (0 → success, ≠0 →
	// Errored). See agentVerdict / applyAgentPhase.
	{
		sentinelDir := goutils.Env("AGENT_EXIT_DIR", "/tmp")
		applied := false
		internal.BackgroundPeriodicTask(ctx, 2*time.Second, func() {
			if applied {
				return
			}
			if phase, text, decided := app.agentVerdict(ctx, sentinelDir); decided {
				if err := app.applyAgentPhase(ctx, phase, text); err == nil {
					applied = true // stop only once the phase actually persisted
				}
			}
		})
	}

	// Hardware-event injector. Reads scheduled faults from the FsNode CR
	// (populated by the experiment-controller from Behaviour.hardwareEvents)
	// and executes them per yass-docs/hardware-events-spec.md.
	go func() {
		fsn := yassv1.FsNode{}
		if err := app.k8sClient.Get(ctx, app.fsNodeObjKey, &fsn); err != nil {
			slog.Error("hwevents: cannot read own FsNode, no faults will be scheduled", "error", err)
			return
		}
		if len(fsn.Spec.HardwareEvents) == 0 {
			return
		}
		mgr := hwevents.New(hwevents.Config{
			FsNode:     app.fsNodeObjKey.Name,
			Namespace:  app.fsNodeObjKey.Namespace,
			Experiment: app.experiment,
			Events:     fsn.Spec.HardwareEvents,
			Facade:     facade,
			K8sClient:  app.k8sClient,
			Networking: networkingHandler,
			KillTargets: func() []int {
				names := resPublisher.KillTargetContainerNames(ctx)
				if len(names) == 0 {
					// Older operator without the yass-containers/* annotations.
					names = killTargetContainerNames(&fsn)
				}
				return resPublisher.PIDsByContainerNames(ctx, names)
			},
			PublishOffln: func() error { return app.publishOnlineState(false) },
			OnDestroy:    func() { app.destroyed.Store(true) },
		})
		// Gate fault injection on the experiment actually starting (first sim
		// update). Otherwise DiskFailure/DiskFull etc. could fire during pod
		// startup, crash the agent/engine and wrongly mark the experiment Errored.
		select {
		case <-app.started:
		case <-ctx.Done():
			return
		}
		mgr.Start(ctx, time.Now())
	}()

	internal.BackgroundPeriodicTask(ctx, 5*time.Second, func() {
		node := yassv1.FsNode{}
		err = app.k8sClient.Get(ctx, app.fsNodeObjKey, &node)
		if err != nil {
			slog.Error("cannot get node object", "error", err)
			return
		}
		pos, battery := app.updates.snapshot()
		node.Status.EnergyConsumptionStr = battery
		node.Status.PosStr = pos
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
	// Release any DiskFailure fuse-errorfs mounts so a deleted pod doesn't hang
	// Terminating with a live FUSE mount kubelet can't clean up (NOTES.md §3).
	hwevents.UnmountAllErrorFS()
	time.Sleep(1 * time.Second)
	slog.Info("Terminated")
}

// killTargetContainerNames is the fallback kill-target list for pods that lack
// the yass-containers/* annotations (created by an older operator): every
// engine container plus the literal "agent". The world-controller and any
// system containers are excluded. The preferred source is the pod annotations
// — see resources.Publisher.KillTargetContainerNames.
func killTargetContainerNames(fsn *yassv1.FsNode) []string {
	out := []string{"agent"}
	for _, c := range fsn.Spec.EngineContainers {
		out = append(out, c.Name)
	}
	return out
}
