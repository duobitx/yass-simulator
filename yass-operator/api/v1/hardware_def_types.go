package v1

import (
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func init() {
	SchemeBuilder.Register(&HardwareDefinition{}, &HardwareDefinitionList{})
}

// HardwareDefinition is a cluster-scoped, reusable hardware profile that models
// the platform a satellite or ground station is built on: CPU, memory, persistent
// storage, battery capacity and a coarse energy-consumption model.
//
// In the typical workflow a HardwareDefinition is referenced by name from a
// Layout entry (`hardwareSpecRef: sentinel-2`) so the same profile can back many
// FsNodes across many experiments. For one-off tweaks the same Spec may be
// inlined directly on the FsNode/Layout entry via [EmbeddedHardware].
//
// The values land in two distinct enforcement paths:
//   - CPU, memory and disk become real Kubernetes resource requests/limits on
//     the FsNode's Pod, so they participate in normal scheduling and eviction.
//   - Battery and energy values are *simulated* — the world-controller maintains
//     a battery model from these inputs and publishes the current state on the
//     MQTT topic `<fsNode>/resources`.
//
// Example:
//
//	apiVersion: int.esa.yass/v1
//	kind: HardwareDefinition
//	metadata:
//	  name: sentinel-2
//	description: "Sentinel-2 (MSI) — Optical MSI, 13 bands ..."
//	spec:
//	  cpu: "1500m"
//	  memory: "2Gi"
//	  diskSpace: "150Gi"
//	  batteryCapacityWh: 15000
//	  batteryChargeW: 1700
//	  lowPowerThresholdWh: 1500
//	  energyConsumption:
//	    normalPowerBaseW: 1500
//	    lowPowerBaseW: 900
//	    perkByteTXWh: 0.00001
//	    perkByteDiskWR: 0.00002
//	    perkByteDiskRD: 0.00001
//
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="CPU",type=string,JSONPath=`.spec.cpu`
// +kubebuilder:printcolumn:name="Memory",type=string,JSONPath=`.spec.memory`
// +kubebuilder:printcolumn:name="DiskSpace",type=string,JSONPath=`.spec.diskSpace`
type HardwareDefinition struct {
	metav1.TypeMeta `json:",inline"`

	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Description is a free-form human-readable summary of the modelled platform
	// (mission name, on-board storage, downlink class, launch year, ...). Not
	// consumed by the simulator — purely for cluster operators reading
	// `kubectl get hardwaredefinition -o yaml`.
	// +kubebuilder:validation:Optional
	Description string `json:"description,omitempty"`

	// Spec is the actual hardware profile. Optional only to permit a
	// description-only stub during authoring; an FsNode that selects this profile
	// will fail validation if Spec is missing.
	// +kubebuilder:validation:Optional
	Spec *HardwareSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true
type HardwareDefinitionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HardwareDefinition `json:"items"`
}

// HardwareSpec is the body of a [HardwareDefinition] and may also be inlined on
// an [EmbeddedHardware]. It mixes *hard* Kubernetes-enforced limits (CPU, memory,
// DiskSpace) with *soft* simulated limits (the battery / energy block).
type HardwareSpec struct {
	// CPU available to the node. Used as the request and limit on the FsNode Pod
	// (Kubernetes `resource.Quantity`, e.g. `1500m` or `2`). The KinD CPU
	// over-subscription trick documented in NOTES.md applies here.
	CPU *resource.Quantity `json:"cpu"`

	// Memory available to the node. Used as the request and limit on the FsNode Pod.
	Memory *resource.Quantity `json:"memory"`

	// DiskSpace is the hard `emptyDir.sizeLimit` for the engine container's scratch
	// volume (`/tmp`). Exceeding it causes kubelet to evict the pod (no graceful
	// ENOSPC).
	//
	// It does NOT limit `/mnt/transfer` or the agent's `/tmp` — those are reported
	// as soft signals on the `<fsNode>/resources` MQTT topic and are expected to
	// be respected by the engine itself.
	//
	// If not defined the volume is unlimited.
	// +kubebuilder:validation:Optional
	DiskSpace *resource.Quantity `json:"diskSpace"`

	// BatteryCapacityWh is the total usable battery capacity in watt-hours.
	// The simulated state-of-charge starts at this value and is decremented by
	// the EnergyConsumption model below.
	BatteryCapacityWh float32 `json:"batteryCapacityWh"`

	// BatteryChargeW is the average charge power in watts (e.g. from solar panels
	// during illuminated orbit). For ground stations set it large enough to keep
	// the battery effectively full. Optional, defaults to 0 (no recharge).
	// +kubebuilder:validation:Optional
	BatteryChargeW float32 `json:"batteryChargeW"`

	// EnergyConsumption describes the per-component power draw used by the
	// simulated battery model. Optional — fields default to 0, which is suitable
	// for ground stations or for tests that do not exercise power management.
	// +kubebuilder:validation:Optional
	EnergyConsumption HardwareSpecEnergyConsumption `json:"energyConsumption"`

	// LowPowerThresholdWh sets the state-of-charge at which the node transitions
	// to low-power mode. In low-power mode the world-controller switches the base
	// draw to `EnergyConsumption.LowPowerBaseW` and the agent is expected to back
	// off any non-essential activity (the current power mode is published on the
	// `<fsNode>/resources` topic).
	// Optional — defaults to 0 (never enter low-power mode).
	// +kubebuilder:validation:Optional
	LowPowerThresholdWh float32 `json:"lowPowerThresholdWh"`
}

// HardwareSpecEnergyConsumption is the simulator's energy model. The world-controller
// integrates these rates against simulated activity (transmissions, disk I/O,
// elapsed time) to drive the battery state forward.
//
// All `Perk...` rates are per kilobyte; multiply by 1000 to get per-byte values.
type HardwareSpecEnergyConsumption struct {
	// NormalPowerBaseW is the baseline draw (watts) when the node is in normal
	// power mode — covers always-on subsystems regardless of activity.
	NormalPowerBaseW float32 `json:"normalPowerBaseW"`

	// LowPowerBaseW is the baseline draw (watts) when the node is in low-power mode
	// (state-of-charge dropped below LowPowerThresholdWh). Typically lower than
	// NormalPowerBaseW. Optional — defaults to 0.
	// +kubebuilder:validation:Optional
	LowPowerBaseW float32 `json:"lowPowerBaseW"`

	// PerkByteTXWh is the energy cost per kilobyte transmitted, in watt-hours.
	// Charged for every byte the engine sends out of the node.
	// +kubebuilder:validation:Optional
	PerkByteTXWh float32 `json:"perkByteTXWh"`

	// PerkByteDiskWR is the energy cost per kilobyte written to disk, in watt-hours.
	// +kubebuilder:validation:Optional
	PerkByteDiskWR float32 `json:"perkByteDiskWR"`

	// PerkByteDiskRD is the energy cost per kilobyte read from disk, in watt-hours.
	// +kubebuilder:validation:Optional
	PerkByteDiskRD float32 `json:"perkByteDiskRD"`
}
