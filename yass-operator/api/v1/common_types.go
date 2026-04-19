package v1

type Orbit struct {
	// TLE lines
	// +kubebuilder:validation:MinItems=2
	// +kubebuilder:validation:MaxItems=2
	TLE []string `json:"tle"`
}

// EarthPosition Position of the object defined as geo coordinates.
type EarthPosition struct {
	// Latitude in numeric format.
	Lat float32 `json:"lat"`
	// Longitude in numeric format.
	Lng float32 `json:"lng"`
	// +kubebuilder:validation:Optional
	// Height over sea level in meters.
	HeightOverSeaLevel float32 `json:"heightOverSeaLevel,omitempty"`
}

// Rotation - rotation of the object.
type Rotation struct {
	// +kubebuilder:validation:Optional
	Yaw float32 `json:"yaw,omitempty"`
	// +kubebuilder:validation:Optional
	Roll float32 `json:"roll,omitempty"`
	// +kubebuilder:validation:Optional
	Pitch float32 `json:"pitch,omitempty"`
}

// EmbeddedPosition to be embedded in other API resources.
// +kubebuilder:validation:XValidation:rule="(has(self.orbit) && !has(self.earthPosition)) || (!has(self.orbit) && has(self.earthPosition))",message="Exactly one of spec.orbit or spec.earthPosition must be set"
type EmbeddedPosition struct {
	// +kubebuilder:validation:Optional
	// A position of an object on the Earth.
	EarthPosition *EarthPosition `json:"earthPosition,omitempty"`

	// +kubebuilder:validation:Optional
	// A position of an object as TLE.
	Orbit *Orbit `json:"orbit,omitempty"`

	// +kubebuilder:validation:Optional
	// Rotation of an object
	Rotation *Rotation `json:"rotation,omitempty"`
}

// +kubebuilder:validation:XValidation:rule="has(self.hardwareSpecRef) || has(self.hardwareSpec)",message="At least one of spec.hardwareSpecRef or spec.hardwareSpec must be set"
type EmbeddedHardware struct {
	// +kubebuilder:validation:Optional
	// Satellite hardware spec.
	HardwareSpec *HardwareSpec `json:"hardwareSpec,omitempty"`

	// +kubebuilder:validation:Optional
	// Satellite Hardware specification. Field hardwareSpec has priority over hardwareSpecRef.
	HardwareSpecRef string `json:"hardwareSpecRef,omitempty"`
}

type SimpleContainerConfigFiles struct {
	ConfigMapRef string `json:"configMapRef"`
	MountPath    string `json:"mountPath"`
}
type SimpleContainer struct {
	// Container image
	Image string `json:"image"`

	// Envs environment variables
	// +kubebuilder:validation:Optional
	Envs map[string]string `json:"envsMap,omitempty"`

	// Configuration files can be mounted from ConfigMap.
	// +kubebuilder:validation:Optional
	ConfigurationFilesFromConfigMap *SimpleContainerConfigFiles `json:"configurationFilesFromConfigMap"`
}
