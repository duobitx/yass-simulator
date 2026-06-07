package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"
)

const lokiPageLimit = 5000

// lokiClient is a minimal Loki query_range client with forward pagination,
// ported from events-exporter's lokiquery.
type lokiClient struct {
	url    string
	tenant string
	http   *http.Client
}

func newLokiClient(lokiURL, tenant string) *lokiClient {
	if lokiURL == "" {
		return nil
	}
	return &lokiClient{url: lokiURL, tenant: tenant, http: &http.Client{Timeout: 30 * time.Second}}
}

type lokiEntry struct {
	t      time.Time
	labels map[string]string
	line   string
}

func (c *lokiClient) queryRange(ctx context.Context, selector string, start, end time.Time) ([]lokiEntry, error) {
	var out []lokiEntry
	cursor := start
	var seenAtCursor map[string]struct{}
	for {
		page, err := c.queryPage(ctx, selector, cursor, end)
		if err != nil {
			return nil, err
		}
		if len(page) == 0 {
			break
		}
		newCount := 0
		for _, e := range page {
			if seenAtCursor != nil && e.t.Equal(cursor) {
				if _, dup := seenAtCursor[e.line]; dup {
					continue
				}
			}
			out = append(out, e)
			newCount++
		}
		if len(page) < lokiPageLimit {
			break
		}
		last := page[len(page)-1].t
		if last.Equal(cursor) {
			cursor = last.Add(time.Nanosecond)
			seenAtCursor = nil
		} else {
			cursor = last
			seenAtCursor = make(map[string]struct{})
			for i := len(page) - 1; i >= 0 && page[i].t.Equal(last); i-- {
				seenAtCursor[page[i].line] = struct{}{}
			}
		}
		if newCount == 0 && last.Equal(cursor) {
			break
		}
		if !cursor.Before(end) {
			break
		}
	}
	return out, nil
}

func (c *lokiClient) queryPage(ctx context.Context, selector string, start, end time.Time) ([]lokiEntry, error) {
	q := url.Values{}
	q.Set("query", selector)
	q.Set("start", strconv.FormatInt(start.UnixNano(), 10))
	q.Set("end", strconv.FormatInt(end.UnixNano(), 10))
	q.Set("limit", strconv.Itoa(lokiPageLimit))
	q.Set("direction", "forward")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url+"/loki/api/v1/query_range?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	if c.tenant != "" {
		req.Header.Set("X-Scope-OrgID", c.tenant)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("loki query: status %d", resp.StatusCode)
	}
	var body struct {
		Data struct {
			Result []struct {
				Stream map[string]string `json:"stream"`
				Values [][2]string       `json:"values"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	var out []lokiEntry
	for _, stream := range body.Data.Result {
		for _, kv := range stream.Values {
			ns, err := strconv.ParseInt(kv[0], 10, 64)
			if err != nil {
				continue
			}
			out = append(out, lokiEntry{t: time.Unix(0, ns), labels: stream.Stream, line: kv[1]})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].t.Before(out[j].t) })
	return out, nil
}

// lokiEventTable queries one event kind and renders it as the events-exporter
// sheet: fixed [experimentTime, wallTime, fsNode, type, engine, run_id] columns
// (labels + the two payload timestamps) followed by the sorted union of the
// remaining JSON payload keys. All values are strings.
func (c *lokiClient) lokiEventTable(ctx context.Context, experiment, runID, kind string, start, end time.Time) (cols []string, rows [][]string, err error) {
	selector := fmt.Sprintf(`{experiment=%q, run_id=%q, kind=%q}`, experiment, runID, kind)
	entries, err := c.queryRange(ctx, selector, start, end)
	if err != nil {
		return nil, nil, err
	}
	if len(entries) == 0 {
		return nil, nil, nil
	}

	fixed := []string{"experimentTime", "wallTime", "fsNode", "type", "engine", "run_id"}
	fixedSet := map[string]bool{"experimentTime": true, "wallTime": true, "fsNode": true, "type": true, "engine": true, "run_id": true}

	parsed := make([]map[string]any, len(entries))
	extraSet := map[string]struct{}{}
	for i, e := range entries {
		var m map[string]any
		_ = json.Unmarshal([]byte(e.line), &m)
		parsed[i] = m
		for k := range m {
			if !fixedSet[k] {
				extraSet[k] = struct{}{}
			}
		}
	}
	extra := make([]string, 0, len(extraSet))
	for k := range extraSet {
		extra = append(extra, k)
	}
	sort.Strings(extra)
	cols = append(append([]string{}, fixed...), extra...)

	rows = make([][]string, 0, len(entries))
	for i, e := range entries {
		m := parsed[i]
		row := make([]string, 0, len(cols))
		row = append(row,
			payloadTimeStr(m, "experimentTime", e.t),
			payloadTimeStr(m, "wallTime", e.t),
			e.labels["fsNode"], e.labels["type"], e.labels["engine"], e.labels["run_id"])
		for _, k := range extra {
			row = append(row, anyToStr(m[k]))
		}
		rows = append(rows, row)
	}
	return cols, rows, nil
}

func payloadTimeStr(m map[string]any, key string, fallback time.Time) string {
	if m != nil {
		if s, ok := m[key].(string); ok && s != "" {
			return s
		}
	}
	return fallback.UTC().Format(time.RFC3339Nano)
}

func anyToStr(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		if x {
			return "true"
		}
		return "false"
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}
