package main

import (
	"strings"
	"testing"
	"time"

	"github.com/duobitx/yass-simulator/internal-components/events-exporter/internal/lokiquery"
)

// TestBlockRecvExportedAsCSV proves the generic exporter turns block_recv Loki
// entries into a block_recv sheet whose columns include the propagation-graph
// fields, with no kind-specific code.
func TestBlockRecvExportedAsCSV(t *testing.T) {
	entries := []lokiquery.Entry{{
		Time:   time.Unix(1700000000, 0).UTC(),
		Labels: map[string]string{"kind": "block_recv", "fsNode": "relay-01", "type": "RECV", "engine": "edfs", "run_id": "r1"},
		Line:   `{"from_fsNode":"oneweb-0008","to_fsNode":"relay-01","from_peer":"12D3KooWX","file":"oneweb-0008_uc1_0","size":262158,"experimentTime":"2026-05-17T00:02:54Z","wallTime":"2026-06-04T10:00:00Z"}`,
	}}

	sheets := groupIntoSheets(entries)
	if len(sheets) != 1 || sheets[0].Name != "block_recv" {
		t.Fatalf("expected one block_recv sheet, got %+v", sheets)
	}
	csv, err := encodeSheetCSV(sheets[0])
	if err != nil {
		t.Fatal(err)
	}
	out := string(csv)
	for _, want := range []string{"file", "from_fsNode", "to_fsNode", "size", "oneweb-0008", "relay-01", "oneweb-0008_uc1_0", "262158"} {
		if !strings.Contains(out, want) {
			t.Errorf("block_recv.csv missing %q\n--- csv ---\n%s", want, out)
		}
	}
	t.Logf("block_recv.csv:\n%s", out)
}
