package mqttpub

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	com "github.com/m-szalik/com-facade"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// fakeFacade collects every Publish for assertions.
type fakeFacade struct {
	mu       sync.Mutex
	messages []fakeMsg
}

type fakeMsg struct {
	topic    string
	qos      byte
	retained bool
	payload  []byte
}

func (f *fakeFacade) Publish(_ context.Context, topic string, qos byte, retained bool, payload interface{}) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	body, err := json.Marshal(payload)
	if err != nil {
		// allow raw byte slices to pass through
		if b, ok := payload.([]byte); ok {
			body = b
		} else {
			return err
		}
	}
	if b, ok := payload.([]byte); ok {
		body = b
	}
	f.messages = append(f.messages, fakeMsg{topic: topic, qos: qos, retained: retained, payload: body})
	return nil
}

func (f *fakeFacade) Subscribe(_ string, _ com.MessageSubscriptionFunct) error { return nil }
func (f *fakeFacade) Unsubscribe(_ string) error                                { return nil }
func (f *fakeFacade) Connect() error                                            { return nil }
func (f *fakeFacade) IsConnected() bool                                         { return true }
func (f *fakeFacade) Close() error                                              { return nil }

func (f *fakeFacade) findTopic(prefix string) (fakeMsg, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, m := range f.messages {
		if strings.HasPrefix(m.topic, prefix) {
			return m, true
		}
	}
	return fakeMsg{}, false
}

func newRegistryWithSamples() *prometheus.Registry {
	reg := prometheus.NewRegistry()
	g := prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "yass_test_gauge"}, []string{"fsNode"})
	g.WithLabelValues("oneweb-0008").Set(42)
	reg.MustRegister(g)
	c := prometheus.NewCounterVec(prometheus.CounterOpts{Name: "yass_test_counter"}, []string{"fsNode"})
	c.WithLabelValues("estrack-kiruna").Add(7)
	reg.MustRegister(c)
	h := prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "yass_test_hist", Buckets: []float64{1, 5}}, []string{"src"})
	h.WithLabelValues("a").Observe(0.5)
	h.WithLabelValues("a").Observe(3)
	reg.MustRegister(h)
	return reg
}

func TestPublishMetaIsRetained(t *testing.T) {
	f := &fakeFacade{}
	p := New(f, newRegistryWithSamples(), Meta{
		Aggregator: "spain-shot-tus",
		Experiment: "spain-shot",
		Engine:     "tus",
		RunID:      "spain-shot_2026",
		Layout:     "spain-shot-layout",
		Namespace:  "spain-shot-tus",
	}, time.Second)
	if err := p.PublishMeta(context.Background()); err != nil {
		t.Fatalf("PublishMeta: %v", err)
	}
	msg, ok := f.findTopic("metrics/_meta_/")
	if !ok {
		t.Fatalf("meta not published")
	}
	if !msg.retained {
		t.Errorf("meta must be retained")
	}
	if msg.topic != "metrics/_meta_/spain-shot-tus" {
		t.Errorf("unexpected meta topic: %q", msg.topic)
	}
	var got Meta
	if err := json.Unmarshal(msg.payload, &got); err != nil {
		t.Fatalf("decode meta: %v", err)
	}
	if got.Engine != "tus" || got.RunID != "spain-shot_2026" || got.Layout != "spain-shot-layout" {
		t.Errorf("unexpected meta payload: %+v", got)
	}
}

func TestSnapshotCoversCounterGaugeHistogram(t *testing.T) {
	f := &fakeFacade{}
	p := New(f, newRegistryWithSamples(), Meta{Aggregator: "tns"}, time.Second)
	p.snapshot(context.Background(), time.Now())
	gotKinds := map[string]string{}
	for _, m := range f.messages {
		var s snapshot
		if err := json.Unmarshal(m.payload, &s); err != nil {
			t.Errorf("decode: %v", err)
			continue
		}
		if !m.retained {
			t.Errorf("snapshot for %s not retained", s.Metric)
		}
		if s.Aggregator != "tns" {
			t.Errorf("aggregator label missing on %s: %q", s.Metric, s.Aggregator)
		}
		gotKinds[s.Metric] = s.Kind
	}
	for _, want := range []string{"yass_test_gauge", "yass_test_counter", "yass_test_hist"} {
		if _, ok := gotKinds[want]; !ok {
			t.Errorf("missing snapshot for %s", want)
		}
	}
	if k := gotKinds["yass_test_gauge"]; k != "gauge" {
		t.Errorf("yass_test_gauge kind=%q, want gauge", k)
	}
	if k := gotKinds["yass_test_counter"]; k != "counter" {
		t.Errorf("yass_test_counter kind=%q, want counter", k)
	}
	if k := gotKinds["yass_test_hist"]; k != "histogram" {
		t.Errorf("yass_test_hist kind=%q, want histogram", k)
	}
}

func TestLabelsetHashStableForReorderedLabels(t *testing.T) {
	a := []*dto.LabelPair{
		mkLabel("fsNode", "x"),
		mkLabel("type", "PUT"),
	}
	b := []*dto.LabelPair{
		mkLabel("type", "PUT"),
		mkLabel("fsNode", "x"),
	}
	if labelsetHash(a) != labelsetHash(b) {
		t.Errorf("hash should be order-independent: %s vs %s", labelsetHash(a), labelsetHash(b))
	}
}

func TestClearMetaEmptyPayload(t *testing.T) {
	f := &fakeFacade{}
	p := New(f, newRegistryWithSamples(), Meta{Aggregator: "x"}, time.Second)
	if err := p.ClearMeta(context.Background()); err != nil {
		t.Fatalf("ClearMeta: %v", err)
	}
	msg, ok := f.findTopic("metrics/_meta_/x")
	if !ok {
		t.Fatalf("clear meta not published")
	}
	if len(msg.payload) != 0 {
		t.Errorf("clear meta payload should be empty, got %d bytes", len(msg.payload))
	}
	if !msg.retained {
		t.Errorf("clear meta must be retained (so subscribers get cleared)")
	}
}

func mkLabel(k, v string) *dto.LabelPair {
	return &dto.LabelPair{Name: &k, Value: &v}
}
