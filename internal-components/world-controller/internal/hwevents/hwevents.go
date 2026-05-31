// Package hwevents schedules and executes hardware fault injection
// events on this FsNode. See yass-docs/hardware-events-spec.md for the
// full model and §9 for the per-fault runtime mechanism.
package hwevents

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"log/slog"
	"math/rand"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/duobitx/yass-simulator/internal-components/world-controller/internal/networking"
	yassv1 "github.com/duobitx/yass-simulator/yass-operator/api/v1"
	com "github.com/m-szalik/com-facade"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Volume paths inside the world-controller container (Bidirectional
// propagation pushes mount events from here to engine+agent).
const (
	pathTransfer  = "/var/yass/transfer"
	pathEngineTmp = "/var/yass/engine-tmp"
	pathAgentTmp  = "/var/yass/agent-tmp"
)

// fuseErrorFsBinary is the path inside the internal-components image
// where the FUSE binary is copied (see internal-components/Dockerfile).
const fuseErrorFsBinary = "/fuse-errorfs"

// Manager schedules and executes hardware events for one FsNode.
type Manager struct {
	fsNode       string
	namespace    string
	experiment   string
	events       []yassv1.HardwareEvent
	facade       com.Facade
	k8sClient    client.Client
	networking   *networking.Handler
	getPeerIPs   func() []string
	killTargets  func() []int // returns PIDs of engine + agent processes
	publishOffln func() error // signal "offline" on Destroy

	mu        sync.Mutex
	expStart  time.Time // wall-clock at t=0
	active    map[yassv1.HardwareEventType]*activeFault
	destroyed bool

	// Per-event scheduler state, indexed by event slot.
	schedState []*eventSchedState
}

type activeFault struct {
	name     string
	typ      yassv1.HardwareEventType
	endsAt   time.Time // wall-clock; zero for Destroy
	params       *yassv1.HardwareEventParams
	override     int64 // remembered externalCap (bps) for clean teardown
	reductionPct int32 // remembered bandwidth reduction % for clean teardown
}

type eventSchedState struct {
	fired       int
	nextWallFire time.Time
	rng         *rand.Rand
	done        bool
}

// Config carries the wiring the Manager needs from main.
type Config struct {
	FsNode       string
	Namespace    string
	Experiment   string
	Events       []yassv1.HardwareEvent
	Facade       com.Facade
	K8sClient    client.Client
	Networking   *networking.Handler
	GetPeerIPs   func() []string
	KillTargets  func() []int
	PublishOffln func() error
}

func New(cfg Config) *Manager {
	m := &Manager{
		fsNode:       cfg.FsNode,
		namespace:    cfg.Namespace,
		experiment:   cfg.Experiment,
		events:       cfg.Events,
		facade:       cfg.Facade,
		k8sClient:    cfg.K8sClient,
		networking:   cfg.Networking,
		getPeerIPs:   cfg.GetPeerIPs,
		killTargets:  cfg.KillTargets,
		publishOffln: cfg.PublishOffln,
		active:       map[yassv1.HardwareEventType]*activeFault{},
		schedState:   make([]*eventSchedState, len(cfg.Events)),
	}
	for i, e := range cfg.Events {
		seed := int64(0)
		if e.Schedule != nil && e.Schedule.Seed != 0 {
			seed = e.Schedule.Seed
		} else {
			h := fnv.New64a()
			_, _ = h.Write([]byte(cfg.FsNode + "/" + e.Name))
			seed = int64(h.Sum64())
		}
		m.schedState[i] = &eventSchedState{rng: rand.New(rand.NewSource(seed))}
	}
	return m
}

// Start kicks off the scheduler loop. It returns once ctx is cancelled.
// expStart is the wall-clock time of the experiment's t=0.
func (m *Manager) Start(ctx context.Context, expStart time.Time) {
	m.mu.Lock()
	m.expStart = expStart
	m.mu.Unlock()
	if len(m.events) == 0 {
		slog.Info("hwevents: no events scheduled", "fsNode", m.fsNode)
		return
	}
	slog.Info("hwevents: scheduler started", "fsNode", m.fsNode, "events", len(m.events), "expStart", expStart)
	tick := time.NewTicker(1 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			m.clearAll(context.Background(), "experiment_ended")
			return
		case now := <-tick.C:
			m.runTick(ctx, now)
		}
	}
}

func (m *Manager) runTick(ctx context.Context, now time.Time) {
	m.mu.Lock()
	if m.destroyed {
		m.mu.Unlock()
		return
	}
	// Pull events to fire / clear under the lock, then release before
	// running effects so per-effect work doesn't block the scheduler.
	toFire := []int{}
	toClear := []yassv1.HardwareEventType{}
	for typ, a := range m.active {
		if !a.endsAt.IsZero() && now.After(a.endsAt) {
			toClear = append(toClear, typ)
		}
	}
	for i, e := range m.events {
		st := m.schedState[i]
		if st.done {
			continue
		}
		fireAt, ok := nextFireWall(m.expStart, e, st)
		if !ok {
			st.done = true
			continue
		}
		if now.Before(fireAt) {
			continue
		}
		toFire = append(toFire, i)
	}
	m.mu.Unlock()

	for _, typ := range toClear {
		m.clearOne(ctx, typ, "scheduled")
	}
	for _, i := range toFire {
		m.fire(ctx, i, now)
	}
}

// nextFireWall returns the wall-clock time at which event index i should
// next fire. ok=false means the event is exhausted (one-shot already
// fired, or recurring exceeded MaxOccurrences).
func nextFireWall(expStart time.Time, e yassv1.HardwareEvent, st *eventSchedState) (time.Time, bool) {
	startOff, err := time.ParseDuration(e.StartOffset)
	if err != nil {
		return time.Time{}, false
	}
	first := expStart.Add(startOff)
	if e.Schedule == nil {
		// One-shot (or Destroy).
		if st.fired > 0 {
			return time.Time{}, false
		}
		return first, true
	}
	// Recurring.
	if e.Schedule.MaxOccurrences > 0 && st.fired >= int(e.Schedule.MaxOccurrences) {
		return time.Time{}, false
	}
	if st.fired == 0 {
		return first, true
	}
	if !st.nextWallFire.IsZero() {
		return st.nextWallFire, true
	}
	// Compute next interval with jitter.
	mean, err := time.ParseDuration(e.Schedule.IntervalMean)
	if err != nil {
		return time.Time{}, false
	}
	jp := float64(e.Schedule.IntervalJitterPercent) / 100.0
	delta := jitter(st.rng, mean, jp)
	st.nextWallFire = first.Add(delta) // overwritten on each fire
	return st.nextWallFire, true
}

func jitter(rng *rand.Rand, mean time.Duration, jitterFrac float64) time.Duration {
	if jitterFrac <= 0 {
		return mean
	}
	f := 1 + (rng.Float64()*2-1)*jitterFrac
	if f < 0 {
		f = 0
	}
	return time.Duration(float64(mean) * f)
}

func (m *Manager) fire(ctx context.Context, idx int, now time.Time) {
	m.mu.Lock()
	e := m.events[idx]
	st := m.schedState[idx]

	if cur, ok := m.active[e.Type]; ok {
		// Overlap rule (§1.3) — drop the new occurrence.
		st.fired++
		// Plan next occurrence if recurring.
		m.advanceNext(idx, now)
		m.mu.Unlock()
		m.publishEvent(ctx, e, "dropped_overlap", "overlap_with_"+cur.name, now, time.Time{})
		return
	}

	var endsAt time.Time
	switch {
	case e.Type == yassv1.HardwareEventDestroy:
		endsAt = time.Time{}
	case e.Schedule != nil:
		mean, _ := time.ParseDuration(e.Schedule.DurationMean)
		dur := jitter(st.rng, mean, float64(e.Schedule.DurationJitterPercent)/100.0)
		endsAt = now.Add(dur)
	default:
		dur, _ := time.ParseDuration(e.Duration)
		endsAt = now.Add(dur)
	}

	af := &activeFault{
		name:   eventName(e, idx),
		typ:    e.Type,
		endsAt: endsAt,
		params: e.Params,
	}
	if e.Type == yassv1.HardwareEventNetworkBandwidthReduced && e.Params != nil && e.Params.NetworkBandwidth != nil {
		af.override = e.Params.NetworkBandwidth.CapBitsPerSec
		af.reductionPct = e.Params.NetworkBandwidth.ReductionPercent
	}
	m.active[e.Type] = af
	st.fired++
	m.advanceNext(idx, now)
	m.mu.Unlock()

	if err := m.activate(ctx, af); err != nil {
		slog.Error("hwevents: activate failed", "type", e.Type, "name", af.name, "error", err)
	}
	m.publishEvent(ctx, e, "active", "scheduled", now, endsAt)
}

func (m *Manager) advanceNext(idx int, base time.Time) {
	e := m.events[idx]
	st := m.schedState[idx]
	if e.Schedule == nil {
		return
	}
	if e.Schedule.MaxOccurrences > 0 && st.fired >= int(e.Schedule.MaxOccurrences) {
		st.done = true
		return
	}
	mean, err := time.ParseDuration(e.Schedule.IntervalMean)
	if err != nil {
		st.done = true
		return
	}
	dur := jitter(st.rng, mean, float64(e.Schedule.IntervalJitterPercent)/100.0)
	st.nextWallFire = base.Add(dur)
}

func (m *Manager) clearOne(ctx context.Context, typ yassv1.HardwareEventType, reason string) {
	m.mu.Lock()
	af, ok := m.active[typ]
	if !ok {
		m.mu.Unlock()
		return
	}
	delete(m.active, typ)
	m.mu.Unlock()
	if err := m.deactivate(ctx, af); err != nil {
		slog.Error("hwevents: deactivate failed", "type", typ, "error", err)
	}
	m.publishEvent(ctx, yassv1.HardwareEvent{Type: typ, Name: af.name}, "cleared", reason, time.Now(), time.Time{})
}

func (m *Manager) clearAll(ctx context.Context, reason string) {
	m.mu.Lock()
	types := make([]yassv1.HardwareEventType, 0, len(m.active))
	for t := range m.active {
		types = append(types, t)
	}
	m.mu.Unlock()
	for _, t := range types {
		m.clearOne(ctx, t, reason)
	}
}

// ---- per-type effects ----

func (m *Manager) activate(ctx context.Context, a *activeFault) error {
	switch a.typ {
	case yassv1.HardwareEventNetworkBandwidthReduced:
		// CapBitsPerSec is an absolute floor; ReductionPercent is a
		// multiplicative reduction of each peer's orbital rate — the overlay
		// applies whichever is set (the CRD enforces exactly one).
		return m.networking.ApplyFaultOverlay(a.override, a.reductionPct, m.isBlackHole())
	case yassv1.HardwareEventNetworkFailure:
		return m.networking.ApplyFaultOverlay(m.externalCap(), m.reductionPercent(), true)
	case yassv1.HardwareEventDiskFull:
		return m.remountReadOnly()
	case yassv1.HardwareEventDiskFailure:
		return m.mountErrorFS()
	case yassv1.HardwareEventDestroy:
		return m.destroy(ctx)
	}
	return fmt.Errorf("unknown event type %s", a.typ)
}

func (m *Manager) deactivate(ctx context.Context, a *activeFault) error {
	switch a.typ {
	case yassv1.HardwareEventNetworkBandwidthReduced:
		return m.networking.ApplyFaultOverlay(0, 0, m.isBlackHole())
	case yassv1.HardwareEventNetworkFailure:
		return m.networking.ApplyFaultOverlay(m.externalCap(), m.reductionPercent(), false)
	case yassv1.HardwareEventDiskFull:
		return m.remountReadWrite()
	case yassv1.HardwareEventDiskFailure:
		return m.unmountErrorFS()
	case yassv1.HardwareEventDestroy:
		// terminal — never cleared
		return nil
	}
	return nil
}

// isBlackHole / externalCap walk m.active under lock to compose
// overlays when multiple network-class events overlap (in practice
// excluded by the overlap rule, but defensive).
func (m *Manager) isBlackHole() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.active[yassv1.HardwareEventNetworkFailure]
	return ok
}

func (m *Manager) externalCap() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	if a, ok := m.active[yassv1.HardwareEventNetworkBandwidthReduced]; ok {
		return a.override
	}
	return 0
}
func (m *Manager) reductionPercent() int32 {
	m.mu.Lock()
	defer m.mu.Unlock()
	if a, ok := m.active[yassv1.HardwareEventNetworkBandwidthReduced]; ok {
		return a.reductionPct
	}
	return 0
}

// remountReadOnly flips /tmp and /mnt/transfer to read-only in the
// engine and agent mount namespaces. We enter each target's mnt ns via
// nsenter because remount-flag changes do not propagate through
// Bidirectional mount-propagation (kernel only propagates new mounts).
func (m *Manager) remountReadOnly() error { return m.remountTargets("ro") }
func (m *Manager) remountReadWrite() error { return m.remountTargets("rw") }

func (m *Manager) remountTargets(mode string) error {
	var failures []string
	for _, pid := range m.uniqueMntNsPIDs() {
		for _, p := range []string{"/tmp", "/mnt/transfer"} {
			cmd := exec.Command("nsenter", "-t", strconv.Itoa(pid), "-m", "--",
				"mount", "-o", "remount,"+mode+",bind", p, p)
			if out, err := cmd.CombinedOutput(); err != nil {
				slog.Warn("hwevents: nsenter mount failed", "pid", pid, "path", p, "mode", mode, "error", err, "out", string(out))
				failures = append(failures, fmt.Sprintf("pid %d %s: %v", pid, p, err))
			}
		}
	}
	if len(failures) > 0 {
		// Propagate so a fault that failed to remount is recorded as failed
		// rather than reported "active" while having done nothing.
		return fmt.Errorf("remount %s failed: %s", mode, strings.Join(failures, "; "))
	}
	return nil
}

// uniqueMntNsPIDs picks one PID per distinct mount namespace among the
// kill-target containers (engine + agent). nsenter only needs one PID
// per namespace to flip everyone inside it.
func (m *Manager) uniqueMntNsPIDs() []int {
	if m.killTargets == nil {
		return nil
	}
	pids := m.killTargets()
	seen := map[string]int{}
	for _, p := range pids {
		ns, err := exec.Command("readlink", fmt.Sprintf("/proc/%d/ns/mnt", p)).Output()
		if err != nil {
			continue
		}
		key := strings.TrimSpace(string(ns))
		if _, ok := seen[key]; !ok {
			seen[key] = p
		}
	}
	out := make([]int, 0, len(seen))
	for _, p := range seen {
		out = append(out, p)
	}
	return out
}

// mountErrorFS starts a fuse-errorfs server per target path. The server
// runs in foreground (fs.Serve never returns), so we shell-background
// it with nohup — `fusermount -u` at clear time triggers fs.Serve to
// return and the daemon to exit. Output is captured to /tmp for debug.
func (m *Manager) mountErrorFS() error {
	for _, path := range []string{pathTransfer, pathEngineTmp, pathAgentTmp} {
		logFile := fmt.Sprintf("/tmp/fuse-errorfs-%s.log", strings.ReplaceAll(strings.TrimPrefix(path, "/"), "/", "_"))
		shellCmd := fmt.Sprintf("nohup %s %s >%s 2>&1 &", fuseErrorFsBinary, path, logFile)
		out, err := exec.Command("sh", "-c", shellCmd).CombinedOutput()
		if err != nil {
			return fmt.Errorf("fuse-errorfs %s: %w: %s", path, err, string(out))
		}
		// Brief wait for the kernel mountpoint to materialise.
		time.Sleep(300 * time.Millisecond)
	}
	return nil
}

func (m *Manager) unmountErrorFS() error {
	return runAll(
		[]string{"fusermount", "-u", pathTransfer},
		[]string{"fusermount", "-u", pathEngineTmp},
		[]string{"fusermount", "-u", pathAgentTmp},
	)
}

func (m *Manager) destroy(ctx context.Context) error {
	pids := []int{}
	if m.killTargets != nil {
		pids = m.killTargets()
	}
	for _, pid := range pids {
		if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
			slog.Warn("hwevents: kill failed", "pid", pid, "error", err)
		} else {
			slog.Info("hwevents: SIGKILL sent", "pid", pid)
		}
	}
	m.mu.Lock()
	m.destroyed = true
	m.mu.Unlock()
	if m.publishOffln != nil {
		if err := m.publishOffln(); err != nil {
			slog.Warn("hwevents: publishOffline failed", "error", err)
		}
	}
	return nil
}

// ---- event publishing ----

func (m *Manager) publishEvent(ctx context.Context, e yassv1.HardwareEvent, state, reason string, wallNow, expiresAt time.Time) {
	payload := map[string]any{
		"type":        string(e.Type),
		"name":        eventNameWithFallback(e),
		"state":       state,
		"reason":      reason,
		"startOffset": e.StartOffset,
		"wallTime":    wallNow.UTC().Format(time.RFC3339Nano),
	}
	if !expiresAt.IsZero() {
		payload["expiresAt"] = expiresAt.UTC().Format(time.RFC3339Nano)
	}
	if e.Params != nil {
		payload["params"] = e.Params
	}
	body, err := json.Marshal(payload)
	if err != nil {
		slog.Error("hwevents: marshal", "error", err)
		return
	}
	topic := fmt.Sprintf("hardware-events/%s", m.fsNode)
	if err := m.facade.Publish(ctx, topic, 0, false, body); err != nil {
		slog.Warn("hwevents: mqtt publish failed", "topic", topic, "error", err)
	}
	m.emitK8sEvent(ctx, e, state, reason, expiresAt)
}

func (m *Manager) emitK8sEvent(ctx context.Context, e yassv1.HardwareEvent, state, reason string, expiresAt time.Time) {
	if m.k8sClient == nil {
		return
	}
	severity := corev1.EventTypeNormal
	if state == "dropped_overlap" || e.Type == yassv1.HardwareEventDestroy {
		severity = corev1.EventTypeWarning
	}
	msg := fmt.Sprintf("[%s] %s %s (%s)", eventNameWithFallback(e), e.Type, state, reason)
	if !expiresAt.IsZero() {
		msg += fmt.Sprintf(" expires=%s", expiresAt.UTC().Format(time.RFC3339))
	}
	// Look up FsNode for the involvedObject reference.
	fsn := &yassv1.FsNode{}
	if err := m.k8sClient.Get(ctx, client.ObjectKey{Namespace: m.namespace, Name: m.fsNode}, fsn); err != nil {
		slog.Warn("hwevents: cannot get FsNode for event ref", "error", err)
		return
	}
	ev := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s.hw.%s", m.fsNode, uuid.NewUUID()),
			Namespace: m.namespace,
		},
		InvolvedObject: corev1.ObjectReference{
			Kind:       "FsNode",
			Namespace:  fsn.Namespace,
			Name:       fsn.Name,
			UID:        fsn.UID,
			APIVersion: yassv1.GroupVersion.String(),
		},
		Reason:         "Hardware" + string(e.Type) + capitalise(state),
		Message:        msg,
		Type:           severity,
		FirstTimestamp: metav1.NewTime(time.Now()),
		LastTimestamp:  metav1.NewTime(time.Now()),
		Source:         corev1.EventSource{Component: "world-controller"},
	}
	if err := m.k8sClient.Create(ctx, ev); err != nil {
		slog.Warn("hwevents: cannot create k8s event", "error", err)
	}
}

// ---- helpers ----

func runAll(cmds ...[]string) error {
	for _, c := range cmds {
		out, err := exec.Command(c[0], c[1:]...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("%s: %w: %s", strings.Join(c, " "), err, string(out))
		}
	}
	return nil
}

func eventName(e yassv1.HardwareEvent, idx int) string {
	if e.Name != "" {
		return e.Name
	}
	return fmt.Sprintf("%s-%d", strings.ToLower(string(e.Type)), idx)
}

func eventNameWithFallback(e yassv1.HardwareEvent) string {
	if e.Name != "" {
		return e.Name
	}
	return string(e.Type)
}

func capitalise(s string) string {
	if s == "" {
		return s
	}
	parts := strings.Split(s, "_")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, "")
}
