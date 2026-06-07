package internal

import (
	yassv1 "github.com/duobitx/yass-simulator/yass-operator/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

// APIGroup / APIVersion identify the aggregated runtime API that fronts an
// experiment's runtime functions. It is a DIFFERENT group than the CRD
// (`int.esa.yass`): the aggregation layer cannot serve the same group/version
// as an existing CRD.
const (
	APIGroup   = "runtime.esa.yass"
	APIVersion = "v1"
	resource   = "experiments"
	kind       = "Experiment"
	singular   = "experiment"
)

// GroupVersion is the served group/version.
var GroupVersion = schema.GroupVersion{Group: APIGroup, Version: APIVersion}

var (
	// Scheme is the apiserver scheme. It registers the Experiment object (reused
	// from the CRD package) under THIS group/version, so the read-through GET of
	// the base resource returns the full Experiment object.
	Scheme = runtime.NewScheme()
	// Codecs is the codec factory backing request/response (de)serialization.
	Codecs = serializer.NewCodecFactory(Scheme)
)

func init() {
	Scheme.AddKnownTypes(GroupVersion, &yassv1.Experiment{}, &yassv1.ExperimentList{})
	metav1.AddToGroupVersion(Scheme, GroupVersion)
}

func groupResource() schema.GroupResource {
	return GroupVersion.WithResource(resource).GroupResource()
}
