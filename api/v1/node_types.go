package v1

import (
	corev1 "k8s.io/api/core/v1"
)

type OrbitDef struct {
	Radius      float32 `json:"radius"`
	Inclination float32 `json:"inclination"`
	Period      float32 `json:"period"`
	Raan        float32 `json:"raan"`
	ArgPerigee  float32 `json:"arg_perigee"`
	TrueAnomaly float32 `json:"true_anomaly"`
}

type EngineDef struct {
	Image         string `json:"image"`
	Configuration string `json:"configuration,omitempty"`
}

type Node struct {
	Name           string                 `json:"name"`
	Orbit          OrbitDef               `json:"orbit"`
	RotationSpeeds []float32              `json:"rotation_speeds"`
	Engine         EngineDef              `json:"engine,omitempty"`
	// NodeTemplateRef is a reference to another Kubernetes resource that defines the template for this node.
	// Use Kind/APIVersion/Name/Namespace to point at any namespaced resource (CRD or built-in).
	NodeTemplateRef corev1.ObjectReference `json:"nodeTemplateRef"`
}
