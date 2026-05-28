package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/duobitx/yass-simulator/internal-components/events-exporter/internal/ods"
)

// writeCSVTarGz emits each Sheet as a separate <name>.csv inside a
// gzip-compressed tarball. Same column layout as the ODS variant.
// See yass-docs/observability-v2-spec.md §G5.
func writeCSVTarGz(w io.Writer, sheets []ods.Sheet) error {
	gz := gzip.NewWriter(w)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()

	for _, sh := range sheets {
		body, err := encodeSheetCSV(sh)
		if err != nil {
			return fmt.Errorf("encode %s: %w", sh.Name, err)
		}
		hdr := &tar.Header{
			Name:    sh.Name + ".csv",
			Mode:    0o644,
			Size:    int64(len(body)),
			ModTime: time.Now(),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if _, err := tw.Write(body); err != nil {
			return err
		}
	}
	return nil
}

func encodeSheetCSV(sh ods.Sheet) ([]byte, error) {
	var buf bytesWriter
	w := csv.NewWriter(&buf)
	if err := w.Write(sh.Header); err != nil {
		return nil, err
	}
	for _, row := range sh.Rows {
		rec := make([]string, len(row))
		for i, c := range row {
			rec[i] = cellToString(c)
		}
		if err := w.Write(rec); err != nil {
			return nil, err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return buf.b, nil
}

// cellToString renders an ods.Cell back to a flat string. Number cells
// keep their numeric form (Excel/pandas will auto-detect); time cells
// become RFC3339Nano; string cells pass through.
func cellToString(c ods.Cell) string {
	if c.Time != nil {
		return c.Time.UTC().Format(time.RFC3339Nano)
	}
	if c.Number != nil {
		return strconv.FormatFloat(*c.Number, 'f', -1, 64)
	}
	return c.String
}

type bytesWriter struct{ b []byte }

func (w *bytesWriter) Write(p []byte) (int, error) {
	w.b = append(w.b, p...)
	return len(p), nil
}
