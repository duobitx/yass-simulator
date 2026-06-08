package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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
	experimentEndTopic       = "experiment/end-request"
	experimentLifecycleTopic = "experiment-lifecycle"
	crudEventsTopic          = "crud-events"
)

// crudEventMsg is the subset of the fs-engine-wrapper NotifyEvent (capitalised
// Go keys) the executor needs to judge completion off the crud-events stream.
type crudEventMsg struct {
	Name       string `json:"Name"`
	FsNodeName string `json:"FsNodeName"`
	Type       string `json:"Type"`
}

type AppType struct {
	mainCtx             context.Context
	facade              com.Facade
	ExperimentDefData   *cmodel.ExperimentDefinition
	k8sClient           client.Client
	nodes               map[string]*model.FsNodeState
	nodeTypes           map[string]yassv1.FsNodeType
	nodesLock           sync.Mutex
	namespace           string
	experimentStartedAt atomic.Pointer[time.Time]
	experimentTime      atomic.Pointer[time.Time]
	starting            atomic.Bool

	// Server-side completion. The executor decides Success from the authoritative
	// `crud-events` stream (the same one metrics-bridge consumes), instead of
	// relying only on a ground-station agent's best-effort `end-request` broadcast.
	// terminators are the broadcast ground stations (SUCCESS_BROADCAST=true in the
	// ExperimentDefinition); the rule mirrors the agent's SUCCESS_AFTER_FILES.
	terminators    map[string]completionRule
	produced       map[string]struct{}
	receivedByNode map[string]map[string]struct{}
	completionMu   sync.Mutex
	completed      atomic.Bool
}

// completionRule is the per-terminator success condition lifted from the GS
// behaviour's SUCCESS_AFTER_FILES env: a fixed count N, or `all` (the node must
// hold every produced file).
type completionRule struct {
	countAll bool
	n        int
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
		nodeTypes:         map[string]yassv1.FsNodeType{},
		nodesLock:         sync.Mutex{},
		namespace:         goutils.Env("NAMESPACE", ""),
		produced:          map[string]struct{}{},
		receivedByNode:    map[string]map[string]struct{}{},
	}

	if err := app.refreshNodeTypes(ctx); err != nil {
		slog.Default().Warn("cannot load FsNode types from k8s; GS-GS link override disabled until types are known", "error", err)
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
	if !t.starting.CompareAndSwap(false, true) {
		slog.Default().Info("Start() called but experiment already started; treating as no-op")
		return nil
	}
	experimentCtx, cancel := context.WithCancelCause(ctxParent)
	// do not call cancel by default, we want that to continue.
	err := t.facade.Publish(experimentCtx, experimentEndTopic, 0, true, "")
	if err != nil {
		cancel(err)
		t.starting.Store(false)
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
			var endState yassv1.ExperimentState
			switch req.Status {
			case proto.Status_EXPERIMENT_END_REQUEST_FAILURE:
				endErr = NewExperimentEndErrorWithComment(ExperimentEndDueToScenarioFailure, req.Comment)
				endState = yassv1.ExperimentStateFailure
			case proto.Status_EXPERIMENT_END_REQUEST_SUCCESS:
				endErr = NewExperimentEndErrorWithComment(ExperimentEndDueToScenarioSuccess, req.Comment)
				endState = yassv1.ExperimentStateSuccess
			default:
				endErr = NewExperimentEndErrorWithCause(ExperimentEndDueToUnexpectedError, fmt.Errorf("unsupported value fro req.Status - %d", req.Status))
			}
			// Persist the terminal state BEFORE cancelling: cancellation stops the
			// geocalc loop (freezing simulation time, so the maxDuration timeout can
			// never fire again), so this is the only chance to record Success/Failure.
			if endState != "" {
				if err := t.setExperimentTerminalState(endState); err != nil {
					slog.Default().Error("cannot write experiment terminal state", "state", endState, "error", err)
				}
			}
			if endErr != nil {
				cancel(endErr)
			}
		}
	})
	if err != nil {
		t.starting.Store(false)
		return errors.Wrapf(err, "cannot subscribe to %s", experimentEndTopic)
	}

	// Server-side completion: end the run as Success from the authoritative
	// crud-events stream, independent of the GS agent's best-effort end-request
	// broadcast. Only active when the def marks broadcast terminators; otherwise
	// this is a no-op and the run ends on maxDuration / the agent broadcast.
	t.terminators = t.loadCompletionTerminators(experimentCtx)
	if len(t.terminators) > 0 {
		err = t.facade.Subscribe(crudEventsTopic, func(sCtx context.Context, topic string, retained bool, data []byte) {
			if t.completed.Load() {
				return
			}
			e := &crudEventMsg{}
			if err := json.Unmarshal(data, e); err != nil {
				slog.Default().Warn("cannot unmarshal content from topic", "topic", crudEventsTopic, "error", err)
				return
			}
			done, comment := t.recordCrudForCompletion(e)
			if !done || !t.completed.CompareAndSwap(false, true) {
				return
			}
			slog.Default().Info("server-side completion reached", "comment", comment)
			// Persist Success BEFORE cancelling: cancellation stops the geocalc
			// loop, so this is the only chance to record the terminal state.
			if err := t.setExperimentTerminalState(yassv1.ExperimentStateSuccess); err != nil {
				slog.Default().Error("cannot write experiment terminal state", "state", yassv1.ExperimentStateSuccess, "error", err)
			}
			cancel(NewExperimentEndErrorWithComment(ExperimentEndDueToScenarioSuccess, comment))
		})
		if err != nil {
			t.starting.Store(false)
			return errors.Wrapf(err, "cannot subscribe to %s", crudEventsTopic)
		}
		slog.Default().Info("server-side completion enabled", "terminators", len(t.terminators))
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
				lt := lastTime
				t.experimentTime.Store(&lt)
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
				reason := "context-cancelled"
				comment := ""
				if cause := context.Cause(experimentCtx); cause != nil {
					var endErr *ExperimentEndError
					if errors.As(cause, &endErr) {
						reason = endErr.String()
						comment = endErr.comment
					} else {
						comment = cause.Error()
					}
				}
				t.publishLifecycle("ended", reason, comment)
				return
			}
		}
	}()
	startAt := time.Now()
	if t.ExperimentDefData.StartTime != nil {
		startAt = *t.ExperimentDefData.StartTime
	}
	t.experimentStartedAt.Store(&startAt)
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
	t.experimentStartedAt.Store(&startAt)
	t.publishLifecycle("started", "", "")
	return nil
}

// publishLifecycle emits a single event onto the experiment-lifecycle MQTT
// topic. The metrics-bridge subscribes and pushes it to Loki — this is how
// experiment start/end transitions show up in the events table and the .ods
// export.
//
// Both clocks are sent: `expTime` is the simulated/experiment clock and is
// the canonical timestamp (the bridge uses it as the Loki sample time);
// `when` is wall-clock and is only a fallback if expTime is zero (e.g.
// "started" fires before geocalc has produced its first tick).
//
// reason / comment are free-form; for end-events reason is one of
// "scenario-success", "scenario-failure", "scenario-timeout",
// "unexpected-error".
func (t *AppType) publishLifecycle(state, reason, comment string) {
	body := map[string]any{
		"state": state,
		"when":  time.Now().UTC(),
	}
	if et := t.experimentTime.Load(); et != nil {
		body["expTime"] = et.UTC()
	} else if sa := t.experimentStartedAt.Load(); sa != nil {
		body["expTime"] = sa.UTC()
	}
	if reason != "" {
		body["reason"] = reason
	}
	if comment != "" {
		body["comment"] = comment
	}
	if err := t.facade.Publish(t.mainCtx, experimentLifecycleTopic, 0, false, body); err != nil {
		slog.Default().Warn("cannot publish lifecycle event", "state", state, "error", err)
	}
}

func (t *AppType) sendTimeUpdate(now time.Time, ongoing bool) error {
	obj := &proto.TimeUpdate{
		Ongoing: ongoing,
		Now:     now.UnixMilli(),
	}
	return t.facade.Publish(t.mainCtx, "updates/_time_", 0, true, obj)
}

func (t *AppType) experimentTimedOutUpdateExperimentResource() error {
	return t.setExperimentTerminalState(yassv1.ExperimentStateTimedOut)
}

// setExperimentTerminalState writes a terminal experimentState onto the Experiment
// resource when it is still Ongoing. The timeout path uses it for TimedOut; the
// agent end-signal handler uses it for Success/Failure. Without this, an agent
// SUCCESS/FAILURE message only cancels the executor's loop (freezing simulation
// time) and never writes a terminal state, leaving the Experiment stuck Ongoing
// forever — the operator's fallback only transitions once EVERY FsNode is terminal,
// which never happens when a ground station or relay stays Running.
func (t *AppType) setExperimentTerminalState(state yassv1.ExperimentState) error {
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
		slog.Default().Info("experiment reached terminal state", "state", state)
		exp.Status.ExperimentState = state
		return t.k8sClient.Status().Update(t.mainCtx, exp)
	}
	return nil
}

func (t *AppType) handleGeoUpdate(_ context.Context, upd *geocalc.GlobalGeoCalcUpdate) error {
	nowMillis := upd.CurrentTime.UnixMilli()
	jeh := goutils.JoinErrorHelper{}
	t.augmentGroundStationLinks(upd.FsNodeInfos)
	for _, data := range upd.FsNodeInfos {
		networkParams := make([]*proto.FsNodeUpdateNetworkParamEntry, 0, len(data.ReachableFsNodes))
		for _, peer := range data.ReachableFsNodes {
			var ip string
			var ok bool
			func() {
				t.nodesLock.Lock()
				defer t.nodesLock.Unlock()
				if st, found := t.nodes[peer.NameTo]; found {
					ip = st.IP
					ok = true
				}
			}()
			if !ok {
				// Peer hasn't published its online-state on MQTT yet; skip until it does.
				slog.Default().Warn("cannot resolve IP for fsNode", "fsNode", peer.NameTo, "processingFsNode", data.Name)
				continue
			}
			np := &proto.FsNodeUpdateNetworkParamEntry{
				Ip:       ip,
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

		// Sibling publish for observability — `los/<src>` carries just the
		// peer roster (names + their network params) so metrics-bridge can
		// derive `yass_los_active{src, peer}` without parsing the full
		// FsNodeUpdate. See yass-docs/observability-v2-spec.md §G3 / §A.3.
		losPeers := make([]map[string]any, 0, len(data.ReachableFsNodes))
		for _, peer := range data.ReachableFsNodes {
			losPeers = append(losPeers, map[string]any{
				"name":     peer.NameTo,
				"distance": peer.Distance,
			})
		}
		losMsg := map[string]any{
			"fsNode":            data.Name,
			"updatedUnixMillis": nowMillis,
			"peers":             losPeers,
		}
		err = t.facade.Publish(t.mainCtx, fmt.Sprintf("los/%s", data.Name), 0, true, losMsg)
		jeh.Append(err)
	}
	return jeh.AsError()
}

// Reference parameters for distance-dependent link metrics.
const (
	bandwidthMaxBps      = 100 * 1024 * 1024       // 100 Mbit/s at or below dRefKm
	bandwidthMinBps      = 100 * 1024              // 100 kbit/s floor
	bandwidthRefDistance = 1000.0                  // km — distance at which we still get full bandwidth
	lossRefDistance      = 1000.0                  // km — reference distance for the quadratic loss model
	transmitterDelayMs   = 1.0                     // fixed transmitter delay in ms
	speedOfLightKmPerMs  = 299.792458              // km per millisecond
	packageLossBase      = 0.001                   // 0.1% baseline loss
	packageLossSlope     = 0.001                   // per (d/d_ref)^2 unit
	packageLossMax       = 0.5                     // 50% cap; above that link is effectively dead
	gsBandwidthBps       = 10 * 1000 * 1000 * 1000 // 10 Gbit/s — terrestrial GS-GS link
)

func (t *AppType) calculateNetworkParam(fsNodeMain *geocalc.FsNodeInfo, dstName string, dst *proto.FsNodeUpdateNetworkParamEntry) {
	dst.Subject = dstName
	dst.PackageDelay = transmitterDelayMs + dst.Distance/speedOfLightKmPerMs
	if t.isGroundStation(fsNodeMain.Name) && t.isGroundStation(dstName) {
		dst.PackageLoss = 0
		dst.Bandwidth = float32(gsBandwidthBps)
		return
	}
	// Loss grows quadratically with distance to mimic worse SNR over longer hops.
	distRatio := float64(dst.Distance) / float64(lossRefDistance)
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

func (t *AppType) refreshNodeTypes(ctx context.Context) error {
	list := &yassv1.FsNodeList{}
	if err := t.k8sClient.List(ctx, list, client.InNamespace(t.namespace)); err != nil {
		return errors.Wrapf(err, "cannot list FsNodes in namespace %q", t.namespace)
	}
	types := make(map[string]yassv1.FsNodeType, len(list.Items))
	for i := range list.Items {
		types[list.Items[i].Name] = list.Items[i].Spec.NodeType
	}
	t.nodesLock.Lock()
	t.nodeTypes = types
	t.nodesLock.Unlock()
	slog.Default().Info("Loaded FsNode types", "count", len(types))
	return nil
}

func (t *AppType) isGroundStation(name string) bool {
	t.nodesLock.Lock()
	nt, ok := t.nodeTypes[name]
	t.nodesLock.Unlock()
	return ok && nt == yassv1.FsNodeTypeGroundStation
}

// loadCompletionTerminators reads the experiment's ExperimentDefinition and
// returns one completionRule per ground station that the generators marked as a
// run terminator (`SUCCESS_BROADCAST=true`), keyed by FsNode name. The rule is
// lifted from the same `SUCCESS_AFTER_FILES` env the receive-only agent uses, so
// server-side completion is byte-for-byte the condition the agent would apply —
// just authoritative. Returns nil (server-side completion disabled) when the def
// is unreadable or marks no broadcast terminators.
func (t *AppType) loadCompletionTerminators(ctx context.Context) map[string]completionRule {
	def := &yassv1.ExperimentDefinition{}
	key := client.ObjectKey{Namespace: t.namespace, Name: t.ExperimentDefData.Name}
	if err := t.k8sClient.Get(ctx, key, def); err != nil {
		slog.Default().Warn("cannot load ExperimentDefinition; server-side completion disabled", "name", key, "error", err)
		return nil
	}
	rules := map[string]completionRule{}
	for i := range def.Spec.Behaviours {
		b := &def.Spec.Behaviours[i]
		if !envTrue(b.Agent.Envs["SUCCESS_BROADCAST"]) {
			continue
		}
		raw := strings.TrimSpace(strings.ToLower(b.Agent.Envs["SUCCESS_AFTER_FILES"]))
		if raw == "all" {
			rules[b.FsNodeName] = completionRule{countAll: true}
			continue
		}
		n := 1
		if v, err := strconv.Atoi(raw); err == nil && v > 1 {
			n = v
		}
		rules[b.FsNodeName] = completionRule{n: n}
	}
	if len(rules) == 0 {
		return nil
	}
	return rules
}

// recordCrudForCompletion folds one crud-event into the completion tracker and
// reports whether a terminator's condition is now met. PUT grows the produced
// set; RECEIVED at a terminator grows that node's received set. A fixed-count
// rule fires once the node holds N files; an `all` rule fires once the node
// holds every produced file.
func (t *AppType) recordCrudForCompletion(e *crudEventMsg) (bool, string) {
	if e.Name == "" {
		return false, ""
	}
	t.completionMu.Lock()
	defer t.completionMu.Unlock()
	switch e.Type {
	case "PUT":
		t.produced[e.Name] = struct{}{}
	case "RECEIVED":
		rule, isTerminator := t.terminators[e.FsNodeName]
		if !isTerminator {
			return false, ""
		}
		recv := t.receivedByNode[e.FsNodeName]
		if recv == nil {
			recv = map[string]struct{}{}
			t.receivedByNode[e.FsNodeName] = recv
		}
		recv[e.Name] = struct{}{}
		if rule.countAll {
			if len(t.produced) > 0 && subset(t.produced, recv) {
				return true, fmt.Sprintf("%s holds all %d produced file(s)", e.FsNodeName, len(t.produced))
			}
			return false, ""
		}
		if len(recv) >= rule.n {
			return true, fmt.Sprintf("%s received %d/%d file(s)", e.FsNodeName, len(recv), rule.n)
		}
	}
	return false, ""
}

func envTrue(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on", "y", "t":
		return true
	}
	return false
}

// subset reports whether every key of want is present in have.
func subset(want, have map[string]struct{}) bool {
	for k := range want {
		if _, ok := have[k]; !ok {
			return false
		}
	}
	return true
}

// Terrestrial GS-GS links bypass line-of-sight filtering done by geo_calc;
// distance falls back to a great-circle arc so delay stays realistic.
func (t *AppType) augmentGroundStationLinks(nodes []*geocalc.FsNodeInfo) {
	gsList := make([]*geocalc.FsNodeInfo, 0, len(nodes))
	for _, n := range nodes {
		if t.isGroundStation(n.Name) {
			gsList = append(gsList, n)
		}
	}
	if len(gsList) < 2 {
		return
	}
	for _, src := range gsList {
		existing := make(map[string]bool, len(src.ReachableFsNodes))
		for _, p := range src.ReachableFsNodes {
			existing[p.NameTo] = true
		}
		for _, dst := range gsList {
			if dst.Name == src.Name || existing[dst.Name] {
				continue
			}
			src.ReachableFsNodes = append(src.ReachableFsNodes, geocalc.DistanceInfo{
				NameTo:   dst.Name,
				Distance: greatCircleDistanceKm(src.Lat, src.Lng, dst.Lat, dst.Lng),
			})
		}
	}
}

func greatCircleDistanceKm(lat1, lng1, lat2, lng2 float32) float32 {
	// Match the geo-calculator's Earth radius (libsgp4 WGS72, radiusearthkm)
	// so terrestrial GS-GS distances are on the same Earth as the LOS-derived
	// satellite distances.
	const earthRadiusKm = 6378.137
	const degToRad = math.Pi / 180.0
	la1 := float64(lat1) * degToRad
	la2 := float64(lat2) * degToRad
	dLat := la2 - la1
	dLng := (float64(lng2) - float64(lng1)) * degToRad
	sinDLat := math.Sin(dLat / 2)
	sinDLng := math.Sin(dLng / 2)
	a := sinDLat*sinDLat + math.Cos(la1)*math.Cos(la2)*sinDLng*sinDLng
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return float32(earthRadiusKm * c)
}

func (t *AppType) updateK8sResource() error {
	et := t.experimentTime.Load()
	if et == nil {
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
	exp.Status.ExperimentTime = v1.Time{Time: *et}
	return t.k8sClient.Status().Update(t.mainCtx, exp)
}
