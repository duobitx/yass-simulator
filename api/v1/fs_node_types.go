/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// FsNodeSpec defines the desired state of FsNode
type FsNodeSpec struct {
	EmbeddedHardware `json:",inline"`
	EmbeddedPosition `json:",inline"`
	// What agent to be installed.
	Agent SimpleContainer `json:"agent,omitempty"`
	// What file system engine to be installed
	// +kubebuilder:validation:MinItems=1
	EngineContainers []corev1.Container `json:"engineContainers,omitempty"`
	// +kubebuilder:validation:Optional
	EngineVolumes []corev1.Volume `json:"engineVolumes,omitempty"`
}

// FsNodeStatus defines the observed state of FsNode.
type FsNodeStatus struct {
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions         []*metav1.Condition `json:"conditions,omitempty"`
	Ready              bool                `json:"ready"`
	PosStr             string              `json:"posStr"`
	LowPower           bool                `json:"lowPower"`
	BatteryChargeLevel string              `json:"batteryChargeLevel"`
	BatteryChargeRate  string              `json:"batteryChargeRate"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=fsn
// +kubebuilder:resource:shortName=fsns
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.ready`
// +kubebuilder:printcolumn:name="BatLev",type=string,JSONPath=`.status.batteryChargeLevel`
// +kubebuilder:printcolumn:name="BatCharge",type=string,JSONPath=`.status.batteryChargeRate`
// +kubebuilder:printcolumn:name="LowPower",type=boolean,JSONPath=`.status.lowPower`
// +kubebuilder:printcolumn:name="PosOverEarth",type=boolean,JSONPath=`.status.posStr`
// FsNode is the Schema for the FsNode API
type FsNode struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of FsNode
	// +required
	Spec FsNodeSpec `json:"spec"`

	// status defines the observed state of FsNode
	// +optional
	Status FsNodeStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true
// FsNodeList contains a list of FsNode
type FsNodeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FsNode `json:"items"`
}

func (s *SimpleContainer) AsMap() (map[string]string, error) {
	ret := map[string]string{}
	if s.Envs != nil {
		for k, v := range s.Envs {
			ret[k] = v
		}
	}
	return ret, nil
}

func init() {
	SchemeBuilder.Register(&FsNode{}, &FsNodeList{})
}
