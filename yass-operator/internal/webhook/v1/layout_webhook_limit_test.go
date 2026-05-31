package v1

import (
	"context"
	"testing"

	yassv1 "github.com/duobitx/yass-simulator/yass-operator/api/v1"
)

func groundStationSpec(n int) []yassv1.LayoutSatSpec {
	spec := make([]yassv1.LayoutSatSpec, n)
	for i := range spec {
		spec[i].NodeType = yassv1.FsNodeTypeGroundStation
	}
	return spec
}

func TestLayoutMaxFsNodes(t *testing.T) {
	v := LayoutWebhook{}
	ctx := context.Background()

	if _, err := v.ValidateCreate(ctx, &yassv1.Layout{Spec: groundStationSpec(MaxFsNodes + 1)}); err == nil {
		t.Fatalf("expected an error for %d fsNodes (over the %d limit)", MaxFsNodes+1, MaxFsNodes)
	}
	if _, err := v.ValidateCreate(ctx, &yassv1.Layout{Spec: groundStationSpec(MaxFsNodes)}); err != nil {
		t.Fatalf("unexpected error at the %d fsNode limit: %v", MaxFsNodes, err)
	}
}
