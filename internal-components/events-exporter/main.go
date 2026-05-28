// events-exporter queries Loki for one experiment's events and writes them
// to an .ods spreadsheet. One sheet per event `kind` (crud, online_state,
// power, lifecycle, hardware). Inside a sheet rows are sorted oldest-first.
//
// Trigger model: invoked as a one-shot binary (CLI or Kubernetes Job). All
// configuration is via flags + env defaults so it can be embedded in a
// container with a single argv.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/duobitx/yass-simulator/internal-components/events-exporter/internal/lokiquery"
	"github.com/duobitx/yass-simulator/internal-components/events-exporter/internal/ods"
	"github.com/m-szalik/goutils"
)

func main() {
	loki := flag.String("loki", goutils.Env("LOKI_URL", "http://loki.yass-system.svc.cluster.local:3100"), "Loki base URL")
	tenant := flag.String("tenant", goutils.Env("LOKI_TENANT", ""), "Loki X-Scope-OrgID (optional)")
	experiment := flag.String("experiment", goutils.Env("EXPERIMENT_NAME", ""), "experiment label to filter")
	engine := flag.String("engine", goutils.Env("ENGINE", ""), "engine label to filter (optional)")
	runID := flag.String("run-id", goutils.Env("RUN_ID", ""), "run_id label to filter (optional)")
	since := flag.Duration("since", 24*time.Hour, "look-back window from now")
	startStr := flag.String("start", "", "absolute RFC3339 start time (overrides --since)")
	endStr := flag.String("end", "", "absolute RFC3339 end time (defaults to now)")
	out := flag.String("out", "", "output file path (default /var/yass-observability/exports/<experiment>-<run_id>.<ext>)")
	format := flag.String("format", "ods", "output format: ods (default, single spreadsheet) or csv (tar.gz with one CSV per event kind)")
	flag.Parse()
	switch *format {
	case "ods", "csv":
		// ok
	default:
		fmt.Fprintf(os.Stderr, "bad --format %q: want ods|csv\n", *format)
		os.Exit(2)
	}

	if *experiment == "" {
		fmt.Fprintln(os.Stderr, "missing --experiment / EXPERIMENT_NAME")
		os.Exit(2)
	}

	end := time.Now()
	if *endStr != "" {
		t, err := time.Parse(time.RFC3339, *endStr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "bad --end: %v\n", err)
			os.Exit(2)
		}
		end = t
	}
	start := end.Add(-*since)
	if *startStr != "" {
		t, err := time.Parse(time.RFC3339, *startStr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "bad --start: %v\n", err)
			os.Exit(2)
		}
		start = t
	}

	outPath := *out
	if outPath == "" {
		stamp := time.Now().UTC().Format("20060102T150405Z")
		name := *experiment
		if *runID != "" {
			name = name + "-" + *runID
		}
		ext := ".ods"
		if *format == "csv" {
			ext = ".tar.gz"
		}
		name = name + "-" + stamp + ext
		outPath = filepath.Join("/var/yass-observability/exports", name)
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir: %v\n", err)
		os.Exit(3)
	}

	selector := buildSelector(*experiment, *engine, *runID)
	slog.Info("query", "loki", *loki, "selector", selector, "start", start, "end", end)

	client := lokiquery.New(*loki, *tenant)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	entries, err := client.QueryRange(ctx, selector, start, end)
	if err != nil {
		fmt.Fprintf(os.Stderr, "loki query: %v\n", err)
		os.Exit(4)
	}
	slog.Info("fetched", "entries", len(entries))

	sheets := groupIntoSheets(entries)

	f, err := os.Create(outPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create %s: %v\n", outPath, err)
		os.Exit(5)
	}
	switch *format {
	case "ods":
		if err := ods.Write(f, sheets); err != nil {
			_ = f.Close()
			fmt.Fprintf(os.Stderr, "write ods: %v\n", err)
			os.Exit(6)
		}
	case "csv":
		if err := writeCSVTarGz(f, sheets); err != nil {
			_ = f.Close()
			fmt.Fprintf(os.Stderr, "write csv: %v\n", err)
			os.Exit(6)
		}
	}
	if err := f.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "close: %v\n", err)
		os.Exit(7)
	}
	slog.Info("written", "path", outPath, "sheets", len(sheets), "format", *format)
}

func buildSelector(experiment, engine, runID string) string {
	parts := []string{fmt.Sprintf(`experiment=%q`, experiment)}
	if engine != "" {
		parts = append(parts, fmt.Sprintf(`engine=%q`, engine))
	}
	if runID != "" {
		parts = append(parts, fmt.Sprintf(`run_id=%q`, runID))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

// groupIntoSheets turns a chronological stream of Loki entries into one Sheet
// per event `kind`. Each sheet has a stable column layout that's the union of
// keys seen across that sheet's rows.
func groupIntoSheets(entries []lokiquery.Entry) []ods.Sheet {
	byKind := map[string][]lokiquery.Entry{}
	for _, e := range entries {
		k := e.Labels["kind"]
		if k == "" {
			k = "_unknown"
		}
		byKind[k] = append(byKind[k], e)
	}

	var kinds []string
	for k := range byKind {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)

	var sheets []ods.Sheet
	for _, k := range kinds {
		sheets = append(sheets, buildSheet(k, byKind[k]))
	}
	return sheets
}

func buildSheet(kind string, entries []lokiquery.Entry) ods.Sheet {
	// Standard left-most columns; the rest are payload keys discovered below.
	// experimentTime is the canonical timestamp (Loki sample time also tracks
	// experiment time, but we surface the body field explicitly for clarity).
	fixed := []string{"experimentTime", "wallTime", "fsNode", "type", "engine", "run_id"}
	payloadKeys := map[string]struct{}{}

	// Pre-parse payloads so we know all keys before laying out the header.
	parsed := make([]map[string]any, len(entries))
	for i, e := range entries {
		var m map[string]any
		_ = json.Unmarshal([]byte(e.Line), &m)
		parsed[i] = m
		for k := range m {
			if isFixed(fixed, k) {
				continue
			}
			payloadKeys[k] = struct{}{}
		}
	}
	var extra []string
	for k := range payloadKeys {
		extra = append(extra, k)
	}
	sort.Strings(extra)
	header := append(append([]string{}, fixed...), extra...)

	rows := make([][]ods.Cell, 0, len(entries))
	for i, e := range entries {
		expTime := parseTime(parsed[i], "experimentTime")
		wallTime := parseTime(parsed[i], "wallTime")
		row := make([]ods.Cell, 0, len(header))
		row = append(row,
			timeOrFallback(expTime, e.Time),
			timeOrFallback(wallTime, e.Time),
			ods.StringCell(e.Labels["fsNode"]),
			ods.StringCell(e.Labels["type"]),
			ods.StringCell(e.Labels["engine"]),
			ods.StringCell(e.Labels["run_id"]),
		)
		for _, k := range extra {
			row = append(row, payloadCell(parsed[i], k))
		}
		rows = append(rows, row)
	}
	return ods.Sheet{Name: kind, Header: header, Rows: rows}
}

func parseTime(m map[string]any, key string) *time.Time {
	v, ok := m[key].(string)
	if !ok || v == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339Nano, v)
	if err != nil {
		return nil
	}
	return &t
}

func timeOrFallback(primary *time.Time, fallback time.Time) ods.Cell {
	if primary != nil {
		return ods.TimeCell(*primary)
	}
	return ods.TimeCell(fallback)
}

func isFixed(fixed []string, k string) bool {
	for _, f := range fixed {
		if f == k {
			return true
		}
	}
	return false
}

func payloadCell(payload map[string]any, key string) ods.Cell {
	v, ok := payload[key]
	if !ok || v == nil {
		return ods.Cell{}
	}
	switch x := v.(type) {
	case string:
		return ods.StringCell(x)
	case float64:
		return ods.NumberCell(x)
	case bool:
		if x {
			return ods.StringCell("true")
		}
		return ods.StringCell("false")
	default:
		b, _ := json.Marshal(v)
		return ods.StringCell(string(b))
	}
}
