package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	yassv1 "github.com/duobitx/yass-simulator/yass-operator/api/v1"
	"github.com/m-szalik/goutils"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// labelExperiment is the label the operator stamps on every FsNode of an
// experiment. Must stay in sync with yass-operator controller.LabelExperiment.
const labelExperiment = "yass-experiment"

// Backend resolves runtime data for the aggregated API. It does NOT run the
// experiment: it reads the Experiment/FsNode CRs from k8s and proxies the
// per-namespace experiment-executor and events-webapp Services.
type Backend struct {
	client client.Client
	loki   *lokiClient
	prom   *promClient
}

func NewBackend() (*Backend, error) {
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		return nil, err
	}
	if err := yassv1.AddToScheme(scheme); err != nil {
		return nil, err
	}
	cfg, err := ctrl.GetConfig()
	if err != nil {
		return nil, err
	}
	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, err
	}
	return &Backend{
		client: c,
		loki:   newLokiClient(goutils.Env("LOKI_URL", "http://loki.yass-system.svc.cluster.local:3100"), goutils.Env("LOKI_TENANT", "")),
		prom:   newPromClient(goutils.Env("PROMETHEUS_URL", "http://prometheus.yass-system.svc.cluster.local:9090")),
	}, nil
}

func executorBase(ns string) string {
	return fmt.Sprintf("http://experiment-executor.%s.svc.cluster.local:8080", ns)
}

func eventsWebappBase(ns string) string {
	return fmt.Sprintf("http://events-webapp.%s.svc.cluster.local:8080", ns)
}

// proxyTo reverse-proxies the current request to base+path. flush=true streams
// the response immediately (Server-Sent Events).
func proxyTo(w http.ResponseWriter, req *http.Request, base, path string, flush bool) {
	u, err := url.Parse(base)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	rp := httputil.NewSingleHostReverseProxy(u)
	director := rp.Director
	rp.Director = func(r *http.Request) {
		director(r)
		r.URL.Path = path
		r.Host = u.Host
	}
	if flush {
		rp.FlushInterval = -1
	}
	rp.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, e error) {
		http.Error(w, fmt.Sprintf("upstream unavailable: %v", e), http.StatusServiceUnavailable)
	}
	rp.ServeHTTP(w, req)
}

// timePayload mirrors the executor's GET /time response, so the CR-status
// fallback is byte-compatible with the live endpoint.
type timePayload struct {
	Ongoing        bool   `json:"ongoing"`
	ExperimentTime string `json:"experimentTime,omitempty"`
}

// handleTime — experiment time for subresource /time. Prefers the live value
// from the experiment-executor; when the executor is unreachable (e.g. after
// evictResourcesAfter has freed the experiment's compute) it falls back to the
// last time recorded on the Experiment CR status, so /time keeps answering for
// terminated experiments.
func (b *Backend) handleTime(ctx context.Context, ns, name string, w http.ResponseWriter, _ *http.Request) {
	if body, ok := fetchExecutorTime(ctx, ns); ok {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
		return
	}
	exp := &yassv1.Experiment{}
	if err := b.client.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, exp); err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	payload := timePayload{Ongoing: exp.Status.ExperimentState == yassv1.ExperimentStateOngoing}
	if t := exp.Status.ExperimentTime; !t.IsZero() {
		payload.ExperimentTime = t.UTC().Format(time.RFC3339)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

// fetchExecutorTime returns the executor's raw GET /time body, or ok=false if
// the executor is unreachable or does not answer 200.
func fetchExecutorTime(ctx context.Context, ns string) ([]byte, bool) {
	reqCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, executorBase(ns)+"/time", nil)
	if err != nil {
		return nil, false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, false
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, false
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false
	}
	return body, true
}

// handleEvents — SSE stream from events-webapp (subresource /events).
func (b *Backend) handleEvents(_ context.Context, ns, _ string, w http.ResponseWriter, req *http.Request) {
	proxyTo(w, req, eventsWebappBase(ns), "/events-sse", true)
}

// handleStart — POST proxied to the executor (subresource /start).
func (b *Backend) handleStart(_ context.Context, ns, _ string, w http.ResponseWriter, req *http.Request) {
	proxyTo(w, req, executorBase(ns), "/start", false)
}

// fsNodeView is the full per-node object returned by subresource /fsnodes:
// CR-sourced identity/lifecycle merged with the executor's live runtime state.
type fsNodeView struct {
	Name     string  `json:"name"`
	Type     string  `json:"type"`
	Ready    bool    `json:"ready"`
	Phase    string  `json:"phase"`
	Online   bool    `json:"online"`
	IP       string  `json:"ip,omitempty"`
	Lat      float32 `json:"lat"`
	Lng      float32 `json:"lng"`
	Alt      float32 `json:"alt"`
	InShadow bool    `json:"inShadow"`
}

// executorFsNode is the subset of GET /fsNodes?detail=true we consume.
type executorFsNode struct {
	Name     string  `json:"name"`
	Online   bool    `json:"Online"`
	IP       string  `json:"IP"`
	Lat      float32 `json:"Lat"`
	Lng      float32 `json:"Lng"`
	Alt      float32 `json:"Alt"`
	InShadow bool    `json:"InShadow"`
}

// handleFsNodes — full FsNode objects for the experiment (subresource /fsnodes).
// FsNode CRs give identity/type/lifecycle and are always available; the
// executor's live state (online/IP/position) enriches them best-effort, so the
// endpoint still works before the executor is up.
func (b *Backend) handleFsNodes(ctx context.Context, ns, name string, w http.ResponseWriter, _ *http.Request) {
	list := &yassv1.FsNodeList{}
	if err := b.client.List(ctx, list, client.InNamespace(ns), client.MatchingLabels{labelExperiment: name}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	views := make(map[string]*fsNodeView, len(list.Items))
	order := make([]string, 0, len(list.Items))
	for i := range list.Items {
		n := &list.Items[i]
		views[n.Name] = &fsNodeView{
			Name:  n.Name,
			Type:  string(n.Spec.NodeType),
			Ready: n.Status.Ready,
			Phase: string(n.Status.Phase),
		}
		order = append(order, n.Name)
	}
	if live, err := fetchExecutorDetail(ctx, ns); err == nil {
		for _, d := range live {
			if v, ok := views[d.Name]; ok {
				v.Online, v.IP = d.Online, d.IP
				v.Lat, v.Lng, v.Alt, v.InShadow = d.Lat, d.Lng, d.Alt, d.InShadow
			}
		}
	}
	out := make([]*fsNodeView, 0, len(order))
	for _, n := range order {
		out = append(out, views[n])
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func fetchExecutorDetail(ctx context.Context, ns string) ([]executorFsNode, error) {
	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, executorBase(ns)+"/fsNodes?detail=true", nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("executor returned %d", resp.StatusCode)
	}
	var out []executorFsNode
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}
