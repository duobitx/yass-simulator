// Package lokipush is a tiny batched HTTP client for the Loki /loki/api/v1/push
// endpoint. It buffers entries per stream-label-set and flushes either on size
// or on a periodic tick.
//
// One Pusher instance is shared by the whole bridge; callers do
// `p.Push(labels, time, line)` from any goroutine.
package lokipush

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	defaultFlushInterval = 2 * time.Second
	defaultBatchSize     = 256
	pushPath             = "/loki/api/v1/push"
)

type entry struct {
	ts   time.Time
	line string
}

type stream struct {
	labels  string // canonical "{k=\"v\",k2=\"v2\"}" form
	entries []entry
}

type Pusher struct {
	url        string
	tenant     string
	httpClient *http.Client
	mu         sync.Mutex
	streams    map[string]*stream
	maxBatch   int
}

// New returns a Pusher that flushes to url+"/loki/api/v1/push".
// Passing url == "" disables pushing entirely (Push becomes a no-op).
// tenant is forwarded as X-Scope-OrgID; "" means no header (Loki without auth).
func New(url, tenant string) *Pusher {
	return &Pusher{
		url:        strings.TrimRight(url, "/"),
		tenant:     tenant,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		streams:    make(map[string]*stream),
		maxBatch:   defaultBatchSize,
	}
}

// Enabled returns true if the pusher is configured to actually send.
func (p *Pusher) Enabled() bool { return p.url != "" }

// Push queues one log entry for a given label-set. The labels map should
// contain only low-cardinality fields (experiment, engine, fsNode, kind, ...);
// high-cardinality detail belongs in the JSON body of `line`.
func (p *Pusher) Push(labels map[string]string, ts time.Time, line string) {
	if !p.Enabled() {
		return
	}
	if ts.IsZero() {
		ts = time.Now()
	}
	canon := canonicalize(labels)
	p.mu.Lock()
	s, ok := p.streams[canon]
	if !ok {
		s = &stream{labels: canon}
		p.streams[canon] = s
	}
	s.entries = append(s.entries, entry{ts: ts, line: line})
	flush := len(s.entries) >= p.maxBatch
	p.mu.Unlock()

	if flush {
		_ = p.Flush(context.Background())
	}
}

// Run flushes periodically until ctx is cancelled, then performs a final flush.
func (p *Pusher) Run(ctx context.Context) {
	if !p.Enabled() {
		<-ctx.Done()
		return
	}
	t := time.NewTicker(defaultFlushInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			fctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = p.Flush(fctx)
			cancel()
			return
		case <-t.C:
			if err := p.Flush(ctx); err != nil {
				slog.Warn("loki flush failed", "error", err)
			}
		}
	}
}

// Flush sends all buffered entries in a single push request.
func (p *Pusher) Flush(ctx context.Context) error {
	if !p.Enabled() {
		return nil
	}
	p.mu.Lock()
	if len(p.streams) == 0 {
		p.mu.Unlock()
		return nil
	}
	pending := p.streams
	p.streams = make(map[string]*stream)
	p.mu.Unlock()

	type lokiStream struct {
		Stream map[string]string `json:"stream"`
		Values [][2]string       `json:"values"`
	}
	type pushReq struct {
		Streams []lokiStream `json:"streams"`
	}

	body := pushReq{}
	for _, s := range pending {
		ls := parseCanonical(s.labels)
		vals := make([][2]string, 0, len(s.entries))
		for _, e := range s.entries {
			vals = append(vals, [2]string{fmt.Sprintf("%d", e.ts.UnixNano()), e.line})
		}
		body.Streams = append(body.Streams, lokiStream{Stream: ls, Values: vals})
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.url+pushPath, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if p.tenant != "" {
		req.Header.Set("X-Scope-OrgID", p.tenant)
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("loki push: status %d", resp.StatusCode)
	}
	return nil
}

func canonicalize(labels map[string]string) string {
	if len(labels) == 0 {
		return "{}"
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteByte('"')
		b.WriteString(strings.ReplaceAll(labels[k], `"`, `\"`))
		b.WriteByte('"')
	}
	b.WriteByte('}')
	return b.String()
}

func parseCanonical(s string) map[string]string {
	out := map[string]string{}
	s = strings.TrimPrefix(s, "{")
	s = strings.TrimSuffix(s, "}")
	if s == "" {
		return out
	}
	for _, kv := range splitTopLevel(s) {
		eq := strings.Index(kv, "=")
		if eq < 0 {
			continue
		}
		k := kv[:eq]
		v := strings.TrimPrefix(strings.TrimSuffix(kv[eq+1:], `"`), `"`)
		out[k] = strings.ReplaceAll(v, `\"`, `"`)
	}
	return out
}

// splitTopLevel splits on commas not inside quotes.
func splitTopLevel(s string) []string {
	var out []string
	in := false
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"':
			if i == 0 || s[i-1] != '\\' {
				in = !in
			}
		case ',':
			if !in {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	out = append(out, s[start:])
	return out
}
