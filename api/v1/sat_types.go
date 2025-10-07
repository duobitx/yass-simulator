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
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SatSpec defines the desired state of Sat
// +kubebuilder:validation:XValidation:rule="(has(self.hardwareSpecRef) && !has(self.hardwareSpec)) || (!has(self.hardwareSpecRef) && has(self.hardwareSpec))",message="Exactly one of spec.hardwareSpecRef or spec.hardwareSpec must be set"
type SatSpec struct {
	// +kubebuilder:validation:Optional
	// Reference to a hardware installed on the satellite.
	HardwareSpecRef string `json:"hardwareSpecRef,omitempty"`

	// +kubebuilder:validation:Optional
	// Satellite Hardware specification. This has priority over hardwareSpecRef.
	HardwareSpec *HardwareSpec `json:"hardwareSpec,omitempty"`

	EmbeddedPosition EmbeddedPosition `json:",inline"`

	// What file system engine to be installed
	Engine SimpleSatContainerDef `json:"engine,omitempty"`

	// What agent to be installed.
	Agent SimpleSatContainerDef `json:"agent,omitempty"`
}

// SatStatus defines the observed state of Sat.
type SatStatus struct {
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
	PosStr             string             `json:"posStr"`
	LowPower           bool               `json:"lowPower"`
	BatteryChargeLevel string             `json:"batteryChargeLevel"`
	BatteryChargeRate  string             `json:"batteryChargeRate"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="BatLev",type=string,JSONPath=`.spec.batteryChargeLevel`
// +kubebuilder:printcolumn:name="BatCharge",type=string,JSONPath=`.spec.batteryChargeRate`
// +kubebuilder:printcolumn:name="LowPower",type=boolean,JSONPath=`.spec.lowPower`
// +kubebuilder:printcolumn:name="PosOverEarth",type=boolean,JSONPath=`.spec.posStr`
// Sat is the Schema for the SATs API
type Sat struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of Sat
	// +required
	Spec SatSpec `json:"spec"`

	// status defines the observed state of Sat
	// +optional
	Status SatStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// SatList contains a list of Sat
type SatList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Sat `json:"items"`
}

func (s *SimpleSatContainerDef) ParametersAsMap(objName string) (map[string]string, error) {
	params := map[string]string{}
	if s.Parameters.Raw != nil {
		err := json.Unmarshal(s.Parameters.Raw, &params)
		if err != nil {
			return params, errors.Wrap(err, fmt.Sprintf("cannot convert params of '%s'", objName))
		}
	}
	return params, nil
}

func init() {
	SchemeBuilder.Register(&Sat{}, &SatList{})
}
