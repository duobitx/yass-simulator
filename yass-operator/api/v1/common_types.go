package v1

// Orbit describes the orbital position of a satellite using a Two-Line Element set
// (TLE) — the de-facto standard format published by NORAD/Celestrak and consumed by
// the SGP4 propagator the simulator uses to derive an instantaneous (lat, lng, alt)
// for every FsNode of type `satellite` at simulated time `t`.
//
// Use Orbit for satellites; use EarthPosition for ground stations. Exactly one of
// the two must be set on an EmbeddedPosition (enforced by CEL validation).
//
// Example (ONEWEB-0008):
//
//	orbit:
//	  tle:
//	    - "1 44059U 19010C   25347.49126494  .00000026  00000+0  35053-4 0  9990"
//	    - "2 44059  87.9045 265.7535 0001501  76.4950 283.6348 13.16596955327295"
type Orbit struct {
	// TLE is the canonical two-line element set. Both lines are required and must
	// be passed verbatim — the simulator does not normalise whitespace or checksums.
	// The optional "Line 0" object name is NOT part of this field; put it in
	// `metadata.name` of the enclosing FsNode/LayoutSatSpec if needed.
	// +kubebuilder:validation:MinItems=2
	// +kubebuilder:validation:MaxItems=2
	TLE []string `json:"tle"`
}

// EarthPosition pins a node to a fixed point on the surface of the Earth in
// geodetic (WGS-84) coordinates. Use it for ground stations and other stationary
// objects. For satellites use Orbit instead.
//
// Example (ESTRACK Kiruna):
//
//	earthPosition:
//	  lat: 67.857
//	  lng: 20.964
//	  heightOverSeaLevel: 0
type EarthPosition struct {
	// Lat is the geodetic latitude in decimal degrees. Positive = north, negative = south.
	Lat float32 `json:"lat"`
	// Lng is the geodetic longitude in decimal degrees. Positive = east, negative = west.
	Lng float32 `json:"lng"`
	// HeightOverSeaLevel is the altitude above mean sea level, in metres.
	// Optional — defaults to 0 (sea level). Used by the visibility/line-of-sight
	// computation when checking whether a satellite has a usable contact window.
	// +kubebuilder:validation:Optional
	HeightOverSeaLevel float32 `json:"heightOverSeaLevel,omitempty"`
}

// Rotation is the body-fixed orientation of the object in radians, expressed as
// Tait–Bryan (yaw/pitch/roll) angles. It is currently informational only and
// reserved for future hardware events that need a pointing model (antenna
// alignment, solar-panel sun angle, camera footprint on Earth, ...).
//
// All three fields default to 0 and any subset may be specified.
type Rotation struct {
	// Yaw — rotation around the body's vertical axis, in radians. Default 0.
	// +kubebuilder:validation:Optional
	Yaw float32 `json:"yaw,omitempty"`
	// Roll — rotation around the body's longitudinal axis, in radians. Default 0.
	// +kubebuilder:validation:Optional
	Roll float32 `json:"roll,omitempty"`
	// Pitch — rotation around the body's lateral axis, in radians. Default 0.
	// +kubebuilder:validation:Optional
	Pitch float32 `json:"pitch,omitempty"`
}

// EmbeddedPosition is a reusable struct embedded into any resource that needs to
// place a node in the simulated world (FsNode, LayoutSatSpec). It encodes the
// XOR invariant "either orbit, or earthPosition — but not both" as a CEL rule on
// the server side, so misconfiguration fails at `kubectl apply`-time rather than
// inside the controller.
//
// Rotation is optional and orthogonal — it may be set in addition to either of
// the position kinds.
//
// +kubebuilder:validation:XValidation:rule="(has(self.orbit) && !has(self.earthPosition)) || (!has(self.orbit) && has(self.earthPosition))",message="Exactly one of spec.orbit or spec.earthPosition must be set"
type EmbeddedPosition struct {
	// EarthPosition pins the node to a fixed lat/lng/alt point on the Earth.
	// Use this for ground stations. Mutually exclusive with Orbit.
	// +kubebuilder:validation:Optional
	EarthPosition *EarthPosition `json:"earthPosition,omitempty"`

	// Orbit places the node on an SGP4-propagated orbit defined by a TLE.
	// Use this for satellites. Mutually exclusive with EarthPosition.
	// +kubebuilder:validation:Optional
	Orbit *Orbit `json:"orbit,omitempty"`

	// Rotation is the body-fixed pointing/attitude of the object. Optional.
	// +kubebuilder:validation:Optional
	Rotation *Rotation `json:"rotation,omitempty"`
}

// EmbeddedHardware attaches a hardware profile to a node, either by reference to
// a cluster-scoped HardwareDefinition (typical, encouraged for reuse across
// experiments) or inline as a HardwareSpec (handy for one-off tweaks).
//
// At least one of the two must be set. When both are set, the inline HardwareSpec
// wins — this lets a Layout entry start from a shared profile and override a
// single field without duplicating the whole spec.
//
// +kubebuilder:validation:XValidation:rule="has(self.hardwareSpecRef) || has(self.hardwareSpec)",message="At least one of spec.hardwareSpecRef or spec.hardwareSpec must be set"
type EmbeddedHardware struct {
	// HardwareSpec is an inline hardware profile. When present it takes precedence
	// over HardwareSpecRef (no field-level merge — the inline spec is used as-is).
	// +kubebuilder:validation:Optional
	HardwareSpec *HardwareSpec `json:"hardwareSpec,omitempty"`

	// HardwareSpecRef is the name of a cluster-scoped HardwareDefinition to use as
	// the hardware profile for this node (e.g. `sentinel-2`, `ground-station-hwdef`).
	// Ignored when HardwareSpec is set inline.
	// +kubebuilder:validation:Optional
	HardwareSpecRef string `json:"hardwareSpecRef,omitempty"`
}

// SimpleContainerConfigFiles mounts an existing ConfigMap as files inside a
// SimpleContainer. The ConfigMap must already exist in the same namespace as the
// resulting Pod. Each ConfigMap key becomes a file at `<mountPath>/<key>`.
type SimpleContainerConfigFiles struct {
	// ConfigMapRef is the name of the ConfigMap to mount.
	ConfigMapRef string `json:"configMapRef"`
	// MountPath is the absolute path inside the container where the ConfigMap's
	// keys will be exposed as files.
	MountPath string `json:"mountPath"`
}

// SimpleContainer is a stripped-down Pod-container description used by YASS for
// the user-supplied workloads (the agent on each FsNode). It intentionally omits
// the long tail of `corev1.Container` knobs (probes, lifecycle, ports, ...) that
// would not be meaningful for a simulated satellite payload — the operator wires
// the rest itself.
//
// Compare with `FsNodeSpec.EngineContainers` / `ExperimentSpec.EngineContainers`,
// which take the full `corev1.Container` because file-system engines (TUS, EDFS,
// ...) often need raw access to ports, volumes and resource requests.
type SimpleContainer struct {
	// Image is the container image reference (e.g. `ghcr.io/duobitx/yass-agent-periodic`).
	Image string `json:"image"`

	// Envs is an additional set of environment variables injected into the container.
	// The operator merges these with the FsNode's `spec.properties` and a handful
	// of well-known YASS variables (node name, MQTT broker address, ...).
	// +kubebuilder:validation:Optional
	Envs map[string]string `json:"envsMap,omitempty"`

	// ConfigurationFilesFromConfigMap optionally mounts a ConfigMap as files inside
	// the container — useful for shipping per-experiment agent configuration without
	// rebuilding the image.
	// +kubebuilder:validation:Optional
	ConfigurationFilesFromConfigMap *SimpleContainerConfigFiles `json:"configurationFilesFromConfigMap"`
}
