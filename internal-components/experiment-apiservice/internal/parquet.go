package internal

import (
	"io"

	"github.com/parquet-go/parquet-go"
)

// columnIndex maps a flat schema's leaf column names to their column index
// (parquet-go orders Group fields, so the index is not the insertion order).
func columnIndex(s *parquet.Schema) map[string]int {
	m := make(map[string]int, len(s.Columns()))
	for i, path := range s.Columns() {
		m[path[len(path)-1]] = i
	}
	return m
}

// writeStringParquet writes an all-string table (required columns). rows are in
// `columns` order. Mirrors the events sheet layout of events-exporter.
func writeStringParquet(out io.Writer, columns []string, rows [][]string) error {
	group := parquet.Group{}
	for _, c := range columns {
		group[c] = parquet.String()
	}
	schema := parquet.NewSchema("event", group)
	idx := columnIndex(schema)
	w := parquet.NewWriter(out, schema, parquet.Compression(&parquet.Zstd))

	const batchSize = 512
	batch := make([]parquet.Row, 0, batchSize)
	for _, r := range rows {
		row := make(parquet.Row, len(columns))
		for i, c := range columns {
			ci := idx[c]
			row[ci] = parquet.ByteArrayValue([]byte(r[i])).Level(0, 0, ci)
		}
		batch = append(batch, row)
		if len(batch) == batchSize {
			if _, err := w.WriteRows(batch); err != nil {
				return err
			}
			batch = batch[:0]
		}
	}
	if len(batch) > 0 {
		if _, err := w.WriteRows(batch); err != nil {
			return err
		}
	}
	return w.Close()
}

// wideMetricRow is one Prometheus series rendered as a wide row: string label
// values plus the float values indexed by ISO-timestamp column (absent samples
// are written as parquet null).
type wideMetricRow struct {
	labels map[string]string
	values map[string]float64
}

// writeMetricWideParquet writes the wide metric layout (string label columns +
// optional-float ISO-timestamp columns), matching yass-report's metric parquet.
func writeMetricWideParquet(out io.Writer, labelCols, tsCols []string, rows []wideMetricRow) error {
	group := parquet.Group{}
	for _, c := range labelCols {
		group[c] = parquet.String()
	}
	for _, c := range tsCols {
		group[c] = parquet.Optional(parquet.Leaf(parquet.DoubleType))
	}
	schema := parquet.NewSchema("metric", group)
	idx := columnIndex(schema)
	w := parquet.NewWriter(out, schema, parquet.Compression(&parquet.Zstd))

	for _, r := range rows {
		row := make(parquet.Row, len(labelCols)+len(tsCols))
		for _, c := range labelCols {
			ci := idx[c]
			row[ci] = parquet.ByteArrayValue([]byte(r.labels[c])).Level(0, 0, ci)
		}
		for _, c := range tsCols {
			ci := idx[c]
			if v, ok := r.values[c]; ok {
				row[ci] = parquet.DoubleValue(v).Level(0, 1, ci)
			} else {
				row[ci] = parquet.NullValue().Level(0, 0, ci)
			}
		}
		if _, err := w.WriteRows([]parquet.Row{row}); err != nil {
			return err
		}
	}
	return w.Close()
}
