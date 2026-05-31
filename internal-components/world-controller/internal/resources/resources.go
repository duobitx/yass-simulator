// Package resources gathers per-FsNode soft signals (power, per-volume disk
// usage, per-engine-container CPU/RAM) and produces proto.FsNodeResources
// snapshots for publication on the `<fsNode>/resources` MQTT topic.
//
// Per-container CPU/RAM requires the FsNode pod to run with
// shareProcessNamespace: true so /proc lists processes from sibling
// containers; without it, engine_containers will simply come back empty.
// Per-volume disk usage requires the inspected volumes to be mounted into
// the world-controller container; if a mount is missing the volume is
// silently omitted from the snapshot.
package resources

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	proto "github.com/duobitx/yass-simulator/internal-components/go-common/proto/go"
	"github.com/duobitx/yass-simulator/internal-components/world-controller/internal/hw"
	yassv1 "github.com/duobitx/yass-simulator/yass-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// labelFsNode mirrors yass-operator/internal/controller.LabelFsNode. The
// operator's `internal/` package cannot be imported from another module, so
// the constant is duplicated here.
const labelFsNode = "yass-fs-node"

// VolumeSpec describes a volume that should be reported in the resources
// snapshot. MountPath is where the volume is mounted inside the
// world-controller container (not where it is mounted in engine/agent).
type VolumeSpec struct {
	Name        string
	MountPath   string
	HardLimited bool
}

// DefaultVolumes is the canonical set of volumes reported on the resources
// topic. The mount paths must match the operator's PodSpec for the
// world-controller container — see yass-operator/internal/controller/fs_node.
var DefaultVolumes = []VolumeSpec{
	{Name: "transfer", MountPath: "/var/yass/transfer", HardLimited: false},
	{Name: "engine-tmp", MountPath: "/var/yass/engine-tmp", HardLimited: true},
	{Name: "agent-tmp", MountPath: "/var/yass/agent-tmp", HardLimited: false},
}

const clockTicksPerSecond = 100 // Linux default; CONFIG_HZ=100 is the kernel norm under k8s

type Publisher struct {
	fsNodeName string
	namespace  string
	k8sClient  client.Client
	hwState    *hw.NodeHwState
	hwSpec     *yassv1.HardwareSpec
	volumes    []VolumeSpec

	mu        sync.Mutex
	prevTicks map[int]uint64
}

func NewPublisher(fsNodeName, namespace string, k8sClient client.Client, hwState *hw.NodeHwState, hwSpec *yassv1.HardwareSpec) *Publisher {
	return &Publisher{
		fsNodeName: fsNodeName,
		namespace:  namespace,
		k8sClient:  k8sClient,
		hwState:    hwState,
		hwSpec:     hwSpec,
		volumes:    DefaultVolumes,
		prevTicks:  map[int]uint64{},
	}
}

// Snapshot builds a FsNodeResources message. periodSeconds is the elapsed
// time since the previous Snapshot call, used to convert CPU ticks into
// millicores.
func (p *Publisher) Snapshot(ctx context.Context, periodSeconds float64) *proto.FsNodeResources {
	batteryWh, capacityWh, inShadow, lowPower := p.hwState.Power()
	mode := proto.PowerState_NORMAL
	if lowPower {
		mode = proto.PowerState_LOW_POWER
	}
	return &proto.FsNodeResources{
		FsNodeName:        p.fsNodeName,
		UpdatedUnixMillis: time.Now().UnixMilli(),
		Power: &proto.PowerState{
			Mode:              mode,
			BatteryWh:         batteryWh,
			BatteryCapacityWh: capacityWh,
			InShadow:          inShadow,
		},
		Volumes:          p.collectVolumes(),
		EngineContainers: p.collectContainers(ctx, periodSeconds),
	}
}

func (p *Publisher) collectVolumes() []*proto.VolumeUsage {
	out := make([]*proto.VolumeUsage, 0, len(p.volumes))
	for _, v := range p.volumes {
		used, total, ok := statfs(v.MountPath)
		if !ok {
			continue
		}
		capacity := total
		if v.HardLimited && p.hwSpec != nil && p.hwSpec.DiskSpace != nil {
			capacity = uint64(p.hwSpec.DiskSpace.Value())
		}
		out = append(out, &proto.VolumeUsage{
			Name:          v.Name,
			MountPath:     v.MountPath,
			UsedBytes:     used,
			CapacityBytes: capacity,
			HardLimited:   v.HardLimited,
		})
	}
	return out
}

// statfs returns bytes-used and total bytes on the filesystem backing
// mountPath. Returns ok=false if the path is not accessible.
func statfs(mountPath string) (used, total uint64, ok bool) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(mountPath, &stat); err != nil {
		return 0, 0, false
	}
	total = stat.Blocks * uint64(stat.Bsize)
	avail := stat.Bavail * uint64(stat.Bsize)
	if total < avail {
		return 0, total, true
	}
	return total - avail, total, true
}

type containerAcc struct {
	tickDelta uint64
	rss       uint64
}

func (p *Publisher) collectContainers(ctx context.Context, periodSeconds float64) []*proto.ContainerCompute {
	pod, err := p.findOwnPod(ctx)
	if err != nil {
		slog.Debug("resources: cannot find own pod, skipping per-container metrics", "error", err)
		return nil
	}
	fsNode := &yassv1.FsNode{}
	if err := p.k8sClient.Get(ctx, client.ObjectKey{Namespace: p.namespace, Name: p.fsNodeName}, fsNode); err != nil {
		slog.Debug("resources: cannot get FsNode, skipping per-container metrics", "error", err)
		return nil
	}
	engineNames := map[string]struct{}{}
	for _, c := range fsNode.Spec.EngineContainers {
		engineNames[c.Name] = struct{}{}
	}
	if len(engineNames) == 0 {
		return nil
	}
	limits := map[string]struct {
		cpuMilli float32
		memBytes uint64
	}{}
	for _, c := range pod.Spec.Containers {
		if _, ok := engineNames[c.Name]; !ok {
			continue
		}
		lim := struct {
			cpuMilli float32
			memBytes uint64
		}{}
		if q, ok := c.Resources.Limits[corev1.ResourceCPU]; ok {
			lim.cpuMilli = float32(q.MilliValue())
		}
		if q, ok := c.Resources.Limits[corev1.ResourceMemory]; ok {
			lim.memBytes = uint64(q.Value())
		}
		limits[c.Name] = lim
	}
	idToName := map[string]string{}
	for _, cs := range pod.Status.ContainerStatuses {
		if _, ok := engineNames[cs.Name]; !ok {
			continue
		}
		if cid := stripContainerIDPrefix(cs.ContainerID); cid != "" {
			idToName[cid] = cs.Name
		}
	}
	if len(idToName) == 0 {
		return nil
	}

	perContainer := map[string]*containerAcc{}
	for name := range engineNames {
		perContainer[name] = &containerAcc{}
	}

	procEntries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	seen := map[int]struct{}{}
	for _, entry := range procEntries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 0 {
			continue
		}
		cgroupRaw, err := os.ReadFile(fmt.Sprintf("/proc/%d/cgroup", pid))
		if err != nil {
			continue
		}
		cid := containerIDRe.FindString(string(cgroupRaw))
		if cid == "" {
			continue
		}
		name, ok := idToName[cid]
		if !ok {
			continue
		}
		ticks, rss, err := readProcStats(pid)
		if err != nil {
			continue
		}
		prev, hasPrev := p.prevTicks[pid]
		p.prevTicks[pid] = ticks
		seen[pid] = struct{}{}
		acc := perContainer[name]
		acc.rss += rss
		if hasPrev && ticks >= prev {
			acc.tickDelta += ticks - prev
		}
	}
	for pid := range p.prevTicks {
		if _, ok := seen[pid]; !ok {
			delete(p.prevTicks, pid)
		}
	}

	out := make([]*proto.ContainerCompute, 0, len(perContainer))
	for name, acc := range perContainer {
		cpuMillicores := float32(0)
		if periodSeconds > 0 {
			cpuMillicores = float32((float64(acc.tickDelta) / float64(clockTicksPerSecond)) / periodSeconds * 1000.0)
		}
		lim := limits[name]
		out = append(out, &proto.ContainerCompute{
			ContainerName:      name,
			CpuMillicores:      cpuMillicores,
			MemoryBytes:        acc.rss,
			CpuMillicoresLimit: lim.cpuMilli,
			MemoryBytesLimit:   lim.memBytes,
		})
	}
	return out
}

func (p *Publisher) findOwnPod(ctx context.Context) (*corev1.Pod, error) {
	podList := &corev1.PodList{}
	err := p.k8sClient.List(ctx, podList,
		client.InNamespace(p.namespace),
		client.MatchingLabels{labelFsNode: p.fsNodeName},
	)
	if err != nil {
		return nil, err
	}
	if len(podList.Items) == 0 {
		return nil, fmt.Errorf("no pod with label %s=%s", labelFsNode, p.fsNodeName)
	}
	return &podList.Items[0], nil
}

// KillTargetContainerNames returns the agent + engine container names declared
// by the fs-node controller on this pod's annotations (yass-containers/agent
// and yass-containers/engine). The world-controller is deliberately not listed,
// so it survives a Destroy to publish the offline state. Returns nil if the
// annotations are absent (e.g. a pod created by an older operator) so the
// caller can fall back to its own list.
func (p *Publisher) KillTargetContainerNames(ctx context.Context) []string {
	pod, err := p.findOwnPod(ctx)
	if err != nil {
		slog.Warn("resources: cannot find own pod for kill targets", "error", err)
		return nil
	}
	var names []string
	for _, key := range []string{yassv1.AnnotationAgentContainers, yassv1.AnnotationEngineContainers} {
		for _, n := range strings.Split(pod.Annotations[key], ",") {
			if n = strings.TrimSpace(n); n != "" {
				names = append(names, n)
			}
		}
	}
	return names
}

func stripContainerIDPrefix(id string) string {
	if i := strings.Index(id, "://"); i >= 0 {
		return id[i+3:]
	}
	return id
}

var containerIDRe = regexp.MustCompile(`[0-9a-f]{64}`)

// PIDsByContainerNames returns the PIDs (in the world-controller's PID
// namespace) of every process belonging to any of the named containers
// in the FsNode's own pod. Requires shareProcessNamespace: true.
//
// Used by the hardware-event injector to deliver SIGKILL/SIGURG to
// agent + engine processes from the world-controller (see
// yass-docs/hardware-events-spec.md §9.5).
func (p *Publisher) PIDsByContainerNames(ctx context.Context, names []string) []int {
	pod, err := p.findOwnPod(ctx)
	if err != nil {
		slog.Warn("resources: cannot find own pod", "error", err)
		return nil
	}
	wanted := map[string]struct{}{}
	for _, n := range names {
		wanted[n] = struct{}{}
	}
	idToName := map[string]string{}
	for _, cs := range pod.Status.ContainerStatuses {
		if _, ok := wanted[cs.Name]; !ok {
			continue
		}
		if cid := stripContainerIDPrefix(cs.ContainerID); cid != "" {
			idToName[cid] = cs.Name
		}
	}
	if len(idToName) == 0 {
		return nil
	}
	procEntries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	out := make([]int, 0, 16)
	for _, entry := range procEntries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 0 {
			continue
		}
		cgroupRaw, err := os.ReadFile(fmt.Sprintf("/proc/%d/cgroup", pid))
		if err != nil {
			continue
		}
		cid := containerIDRe.FindString(string(cgroupRaw))
		if cid == "" {
			continue
		}
		if _, ok := idToName[cid]; !ok {
			continue
		}
		out = append(out, pid)
	}
	return out
}

func readProcStats(pid int) (ticks, rss uint64, err error) {
	raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0, 0, err
	}
	s := string(raw)
	last := strings.LastIndex(s, ")")
	if last < 0 || last+2 >= len(s) {
		return 0, 0, fmt.Errorf("invalid stat for pid %d", pid)
	}
	fields := strings.Fields(s[last+1:])
	// After comm: state, ppid, pgrp, session, tty_nr, tpgid, flags,
	// minflt, cminflt, majflt, cmajflt, utime(11), stime(12)
	if len(fields) < 13 {
		return 0, 0, fmt.Errorf("not enough stat fields for pid %d", pid)
	}
	utime, _ := strconv.ParseUint(fields[11], 10, 64)
	stime, _ := strconv.ParseUint(fields[12], 10, 64)
	ticks = utime + stime

	statusRaw, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return ticks, 0, nil
	}
	for _, line := range strings.Split(string(statusRaw), "\n") {
		if strings.HasPrefix(line, "VmRSS:") {
			ff := strings.Fields(line)
			if len(ff) >= 2 {
				kb, _ := strconv.ParseUint(ff[1], 10, 64)
				return ticks, kb * 1024, nil
			}
		}
	}
	return ticks, 0, nil
}
