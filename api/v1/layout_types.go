package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func init() {
	SchemeBuilder.Register(&Layout{}, &LayoutList{})
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Start",type=boolean,JSONPath=`.spec.start`
// Layout describe how satellites and groud stations are located in space or/and on the Earth.
type Layout struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Description of the layout
	// +kubebuilder:validation:Optional
	Description string `json:"description,omitempty"`

	// +listType=map
	// +listMapKey=fsNode
	Spec []LayoutSatSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true
type LayoutList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Layout `json:"items"`
}

type LayoutSatSpec struct {
	FsNodeName string `json:"fsNode"`

	EmbeddedHardware `json:",inline"`

	EmbeddedPosition `json:",inline"`
}
