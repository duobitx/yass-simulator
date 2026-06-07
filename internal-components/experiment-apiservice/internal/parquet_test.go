package internal

import (
	"bytes"
	"io"
	"testing"

	"github.com/parquet-go/parquet-go"
)

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func colNames(s *parquet.Schema) []string {
	out := make([]string, 0, len(s.Columns()))
	for _, p := range s.Columns() {
		out = append(out, p[len(p)-1])
	}
	return out
}

// readParquet opens a parquet blob and returns its schema and all rows.
func readParquet(t *testing.T, b []byte) (*parquet.Schema, []parquet.Row) {
	t.Helper()
	f, err := parquet.OpenFile(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	var rows []parquet.Row
	buf := make([]parquet.Row, 64)
	rr := f.RowGroups()[0].Rows()
	defer func() { _ = rr.Close() }()
	for {
		n, err := rr.ReadRows(buf)
		rows = append(rows, buf[:n]...)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read rows: %v", err)
		}
	}
	return f.Schema(), rows
}

func TestWriteStringParquetRoundTrip(t *testing.T) {
	cols := []string{"experimentTime", "wallTime", "fsNode", "type", "engine", "run_id", "action", "fileName"}
	rows := [][]string{
		{"t0", "w0", "sat-1", "satellite", "edfs", "run1", "PUT", "a.bin"},
		{"t1", "w1", "gs-2", "groundStation", "edfs", "run1", "RECEIVED", "a.bin"},
	}
	var buf bytes.Buffer
	if err := writeStringParquet(&buf, cols, rows); err != nil {
		t.Fatalf("write: %v", err)
	}
	schema, prows := readParquet(t, buf.Bytes())
	if len(prows) != 2 {
		t.Fatalf("rows=%d want 2", len(prows))
	}
	got := colNames(schema)
	for _, c := range cols {
		if !contains(got, c) {
			t.Fatalf("missing column %q in %v", c, got)
		}
	}
	idx := columnIndex(schema)
	if v := string(prows[0][idx["action"]].ByteArray()); v != "PUT" {
		t.Fatalf("row0.action=%q want PUT", v)
	}
	if v := string(prows[1][idx["fsNode"]].ByteArray()); v != "gs-2" {
		t.Fatalf("row1.fsNode=%q want gs-2", v)
	}
}

func TestWriteMetricWideRoundTrip(t *testing.T) {
	labelCols := []string{"fsNode", "__name__"}
	ts0, ts1 := "2024-01-01T00:00:00Z", "2024-01-01T00:00:15Z"
	tsCols := []string{ts0, ts1}
	rows := []wideMetricRow{
		{labels: map[string]string{"fsNode": "sat-1", "__name__": "m"}, values: map[string]float64{ts0: 1.5}},
		{labels: map[string]string{"fsNode": "gs-2", "__name__": "m"}, values: map[string]float64{ts1: 2.0}},
	}
	var buf bytes.Buffer
	if err := writeMetricWideParquet(&buf, labelCols, tsCols, rows); err != nil {
		t.Fatalf("write: %v", err)
	}
	schema, prows := readParquet(t, buf.Bytes())
	if len(prows) != 2 {
		t.Fatalf("rows=%d want 2", len(prows))
	}
	idx := columnIndex(schema)
	if v := prows[0][idx[ts0]].Double(); v != 1.5 {
		t.Fatalf("row0[%s]=%v want 1.5", ts0, v)
	}
	if !prows[0][idx[ts1]].IsNull() {
		t.Fatalf("row0[%s] want null", ts1)
	}
	if v := prows[1][idx[ts1]].Double(); v != 2.0 {
		t.Fatalf("row1[%s]=%v want 2.0", ts1, v)
	}
}
