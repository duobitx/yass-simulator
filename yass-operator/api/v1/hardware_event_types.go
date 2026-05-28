package v1

// HardwareEventType discriminates the kind of fault injected into the
// simulated platform. Per-type semantics are documented in
// yass-docs/hardware-events-spec.md §1 and §9.
//
// +kubebuilder:validation:Enum=NetworkBandwidthReduced;NetworkFailure;DiskFull;DiskFailure;Destroy
type HardwareEventType string

const (
	HardwareEventNetworkBandwidthReduced HardwareEventType = "NetworkBandwidthReduced"
	HardwareEventNetworkFailure          HardwareEventType = "NetworkFailure"
	HardwareEventDiskFull                HardwareEventType = "DiskFull"
	HardwareEventDiskFailure             HardwareEventType = "DiskFailure"
	HardwareEventDestroy                 HardwareEventType = "Destroy"
)

// HardwareEvent declares a scheduled hardware fault injected by the
// world-controller into the simulated platform of an FsNode. Faults
// affect ONLY the agent and the fs-engine container — the
// world-controller and any system containers continue to run so the
// failure can be observed and (where applicable) cleared.
//
// Timing:
//   - StartOffset is relative to the experiment's `t=0`. Go-duration
//     format (`10s`, `5m`, `2h30m`).
//   - Either Duration (one-shot) or Schedule (recurring) is set, but
//     never both. For Destroy neither is allowed — Destroy is
//     instantaneous and terminal.
//
// Overlap rule: at most one event of the same Type may be active on a
// node at once. A second occurrence overlapping an already-active one
// of the same type is dropped and reported as `dropped_overlap` on
// `hardware-events/<fsNode>`.
//
// +kubebuilder:validation:XValidation:rule="self.type == 'Destroy' ? (!has(self.duration) && !has(self.schedule)) : ((has(self.duration) && !has(self.schedule)) || (!has(self.duration) && has(self.schedule)))",message="One-shot events require duration; recurring events require schedule; Destroy allows neither."
// +kubebuilder:validation:XValidation:rule="self.type == 'NetworkBandwidthReduced' ? (has(self.params) && has(self.params.networkBandwidth)) : true",message="NetworkBandwidthReduced requires params.networkBandwidth."
type HardwareEvent struct {
	// Name is a stable identifier within the event list. Used as the
	// `name` label on emitted MQTT/K8s events so dashboards can
	// correlate activate/clear pairs. Optional; operator synthesises
	// `<type>-<index>` if missing.
	// +kubebuilder:validation:Optional
	Name string `json:"name,omitempty"`

	// Type selects which fault to inject. See HardwareEventType.
	Type HardwareEventType `json:"type"`

	// StartOffset is the Go-duration offset from experiment start at
	// which the first occurrence fires. For a Schedule this is the
	// earliest time the first occurrence may happen; subsequent
	// occurrences are spaced by Schedule.IntervalMean ± jitter.
	StartOffset string `json:"startOffset"`

	// Duration is the wall-clock length of a single one-shot event.
	// Go-duration format. Mutually exclusive with Schedule; ignored
	// (and rejected) for Destroy.
	// +kubebuilder:validation:Optional
	Duration string `json:"duration,omitempty"`

	// Schedule turns the event into a recurring fault — see
	// HardwareEventSchedule. Mutually exclusive with Duration.
	// +kubebuilder:validation:Optional
	Schedule *HardwareEventSchedule `json:"schedule,omitempty"`

	// Params carries type-specific tuning knobs.
	// +kubebuilder:validation:Optional
	Params *HardwareEventParams `json:"params,omitempty"`
}

// HardwareEventSchedule describes a recurring fault: every
// IntervalMean ± jitter the event fires, each occurrence lasts
// DurationMean ± jitter.
type HardwareEventSchedule struct {
	// IntervalMean is the average wall-clock time between the *start*
	// of two consecutive occurrences. Go-duration format.
	IntervalMean string `json:"intervalMean"`

	// IntervalJitterPercent makes the actual interval uniformly random
	// in `[mean*(1-p/100), mean*(1+p/100)]`. 0 = deterministic.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	IntervalJitterPercent int32 `json:"intervalJitterPercent,omitempty"`

	// DurationMean is the average length of each occurrence.
	DurationMean string `json:"durationMean"`

	// DurationJitterPercent — same semantics as IntervalJitterPercent
	// but applied to occurrence duration.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	DurationJitterPercent int32 `json:"durationJitterPercent,omitempty"`

	// MaxOccurrences caps the total number of times this event fires.
	// 0 (default) = unlimited until experiment end.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=0
	MaxOccurrences int32 `json:"maxOccurrences,omitempty"`

	// Seed lets a recurring event be reproduced bit-for-bit across
	// re-runs. If unset, the world-controller seeds from
	// `fsNode + event-name` so different runs of the same experiment
	// share a schedule.
	// +kubebuilder:validation:Optional
	Seed int64 `json:"seed,omitempty"`
}

// HardwareEventParams holds type-specific knobs. Only the subfield
// matching HardwareEvent.Type is consulted.
type HardwareEventParams struct {
	// NetworkBandwidth is required for NetworkBandwidthReduced.
	// +kubebuilder:validation:Optional
	NetworkBandwidth *NetworkBandwidthParams `json:"networkBandwidth,omitempty"`
}

// NetworkBandwidthParams caps the effective throughput while the event
// is active. Exactly one of CapBitsPerSec or ReductionPercent must be
// set.
//
// +kubebuilder:validation:XValidation:rule="(has(self.capBitsPerSec) && !has(self.reductionPercent)) || (!has(self.capBitsPerSec) && has(self.reductionPercent))",message="Exactly one of capBitsPerSec or reductionPercent must be set."
type NetworkBandwidthParams struct {
	// CapBitsPerSec is the hard throughput cap in bits per second.
	// Effective rate = min(orbital, this).
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=1
	CapBitsPerSec int64 `json:"capBitsPerSec,omitempty"`

	// ReductionPercent reduces the current effective bandwidth by this
	// fraction. 25 = "drop to 75% of the natural cap".
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=99
	ReductionPercent int32 `json:"reductionPercent,omitempty"`
}
