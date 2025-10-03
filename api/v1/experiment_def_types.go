package v1

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func init() {
	SchemeBuilder.Register(&ExperimentDef{}, &ExperimentDefList{})
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Model",type=string,JSONPath=`.spec.model`

// ExperimentDef is the Schema for the Experiment Definition
type ExperimentDef struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Description       string            `json:"description,omitempty"`
	Spec              ExperimentDefSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true
type ExperimentDefList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ExperimentDef `json:"items"`
}

// ExperimentDefSpec defines the desired state of an ExperimentDef
type ExperimentDefSpec struct {
	MaxDuration *time.Duration `json:"max_duration,omitempty"`
	//max_duration: 2h # Optional
	//nodes_behavior:
	//- node: sentinel-node-1
	//agent-image: ghcr.io/satlab/tasks-based-simulation-agent:latest
	//agent_parameters:   # map[string]any
	//tasks:  # image can support it or not, parameters specific to the image
	//- name: data-collection
	//start_time: 0s
	//duration: 30m
	//parallel_tasks:
	//- type: make_photo
	//parameters:
	//data_size: 2MB
	//frequency: 2
	//- node: sentinel-node-1
	//agent-image: ghcr.io/satlab/position-based-simulation-agent:latest
	//agent_parameters:   # map[string]any
	//rules:  # image can support it or not, parameters specific to the image
	//- position: # any position definition that is supported by the image
	//action:
	//- type: make_photo
	//parameters:
	//data_size: 2MB
	//frequency: 2
	//hardware_events:
	//- name: "network_interface_failure"
	//type: "network_interface_down"
	//start_time: 15m
	//duration: 1m
	//parameters:
	//interface_name: "Ka-Band Interface 0"
	//- node: "custom_agent_image_node"
	//agent-image: ghcr.io/satlab/custom-agent:latest
	//evaluation:
	//image: ghcr.io/satlab/experiment-evaluator:latest
	//configuration: "evaluator-configuration"  # ConfigMap

}
