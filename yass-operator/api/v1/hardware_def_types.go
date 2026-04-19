package v1

import (
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func init() {
	SchemeBuilder.Register(&HardwareDefinition{}, &HardwareDefinitionList{})
}

// HardwareDefinition definition of the hardware installed on the satellite or ground station.
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="CPU",type=string,JSONPath=`.spec.cpu`
// +kubebuilder:printcolumn:name="Memory",type=string,JSONPath=`.spec.memory`
// +kubebuilder:printcolumn:name="DiskSpace",type=string,JSONPath=`.spec.diskSpace`
type HardwareDefinition struct {
	metav1.TypeMeta `json:",inline"`

	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +kubebuilder:validation:Optional
	// Description of the hardware specification.
	Description string `json:"description,omitempty"`

	// +kubebuilder:validation:Optional
	Spec *HardwareSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true
type HardwareDefinitionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HardwareDefinition `json:"items"`
}

type HardwareSpec struct {
	// Number of available CPUs.
	CPU *resource.Quantity `json:"cpu"`

	// Number of available RAM.
	Memory *resource.Quantity `json:"memory"`

	// Available disk space. If not defined then unlimited.
	// +kubebuilder:validation:Optional
	DiskSpace *resource.Quantity `json:"diskSpace"`

	BatteryCapacityWh float32 `json:"batteryCapacityWh"`
	// +kubebuilder:validation:Optional
	BatteryChargeW float32 `json:"batteryChargeW"`
	// +kubebuilder:validation:Optional
	EnergyConsumption HardwareSpecEnergyConsumption `json:"energyConsumption"`
	// +kubebuilder:validation:Optional
	LowPowerThresholdWh float32 `json:"lowPowerThresholdWh"`
}

type HardwareSpecEnergyConsumption struct {
	NormalPowerBaseW float32 `json:"normalPowerBaseW"`
	// +kubebuilder:validation:Optional
	LowPowerBaseW float32 `json:"lowPowerBaseW"`
	// +kubebuilder:validation:Optional
	PerkByteTXWh float32 `json:"perkByteTXWh"`
	// +kubebuilder:validation:Optional
	PerkByteDiskWR float32 `json:"perkByteDiskWR"`
	// +kubebuilder:validation:Optional
	PerkByteDiskRD float32 `json:"perkByteDiskRD"`
}
