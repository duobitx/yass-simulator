package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func init() {
	SchemeBuilder.Register(&ExperimentDefinition{}, &ExperimentDefinitionList{})
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Model",type=string,JSONPath=`.spec.model`
// +kubebuilder:printcolumn:name="MaxDuration",type=string,JSONPath=`.spec.maxDuration`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ExperimentDefinition defines behavior and events for an experiment.
type ExperimentDefinition struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Description of the experiment
	Description string                   `json:"description,omitempty"`
	Spec        ExperimentDefinitionSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true
type ExperimentDefinitionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ExperimentDefinition `json:"items"`
}

// ExperimentDefinitionSpec defines the desired state of an ExperimentDefinition
type ExperimentDefinitionSpec struct {

	// +kubebuilder:validation:Optional
	// MaxDuration of the experiment - duration format
	MaxDuration string `json:"maxDuration,omitempty"`

	// +listType=map
	// +listMapKey=fsNode
	Behaviours []Behaviour `json:"behaviours,omitempty"`

	HardwareEvents []HardwareEvent `json:"HardwareEvents,omitempty"`

	// +kubebuilder:validation:Optional
	MetricsConfig *MetricsConfig `json:"metricsConfig,omitempty"`
}

// MetricsConfig configures the metrics-bridge that yass-operator deploys
// next to mosquitto. It is consumed only by the bridge process; if absent,
// the bridge falls back to default behaviour (any GroundStation that
// receives the file counts as a target, deadline = 1h).
type MetricsConfig struct {
	// +kubebuilder:validation:Optional
	// DeliveryDeadline (duration format, e.g. "2h"). Files un-delivered
	// after this become yass_file_lost_total.
	DeliveryDeadline string `json:"deliveryDeadline,omitempty"`

	// +kubebuilder:validation:Optional
	// TargetGroundStations maps each producing fsNode (typically a
	// satellite) to its dedicated destination ground station. Used to
	// label yass_file_delivery_seconds with is_target_gs=true/false.
	TargetGroundStations map[string]string `json:"targetGroundStations,omitempty"`
}

type Behaviour struct {
	// Name of the satellite / ground station to be configured.
	FsNodeName string `json:"fsNode"`
	// Agent on the satellite
	Agent SimpleContainer `json:"agent"`

	// +kubebuilder:validation:Optional
	// What hardware events to expect during the experiment.
	HardwareEvents []HardwareEvent `json:"hardwareEvents"`
}
type HardwareEvent struct {
	// TODO
}
