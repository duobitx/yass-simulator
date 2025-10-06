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
	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// SatSpec defines the desired state of Sat
type SatSpec struct {
	HardwareSpec          *HardwareSpec         `json:"hardwareSpec,omitempty"`
	HardwareDefinitionRef string                `json:"hardwareDefinitionRef,omitempty"`
	Orbit                 Orbit                 `json:"orbit"`
	Rotation              Rotation              `json:"rotation,omitempty"`
	Engine                SimpleSatContainerDef `json:"engine,omitempty"`
	Agent                 SimpleSatContainerDef `json:"agent,omitempty"`
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
// +kubebuilder:printcolumn:name="LowPower",type=bool,JSONPath=`.spec.lowPower`
// +kubebuilder:printcolumn:name="PosOverEarth",type=bool,JSONPath=`.spec.posStr`
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

type Orbit struct {
	Radius      float32 `json:"radius"`
	Inclination float32 `json:"inclination"`
	Period      float32 `json:"period"`
	Raan        float32 `json:"raan"`
	ArgPerigee  float32 `json:"argPerigee"`
	TrueAnomaly float32 `json:"trueAnomaly"`
}

// Rotation - to be inlined
type Rotation struct {
	Yaw   float32 `json:"yaw,omitempty"`
	Roll  float32 `json:"roll,omitempty"`
	Pitch float32 `json:"pitch,omitempty"`
}

type SimpleSatContainerDef struct {
	Image      string  `json:"image"`
	Parameters v1.JSON `json:"parameters,omitempty"`
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
