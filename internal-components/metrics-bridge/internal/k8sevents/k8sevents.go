// Package k8sevents mirrors the metrics-bridge's Loki events as Kubernetes
// Events on the Experiment CR that owns the current namespace. The bridge
// calls Emit() alongside every loki push; the recorder takes care of
// deduplication and rate-limiting inside client-go.
package k8sevents

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
)

const (
	experimentGroup    = "int.esa.yass"
	experimentVersion  = "v1"
	experimentResource = "experiments"
	experimentKind     = "Experiment"
)

// Emitter writes one Kubernetes Event per call. Implementations must be
// goroutine-safe and never block the caller.
type Emitter interface {
	Emit(kind, eventType string, payload map[string]any)
}

type noopEmitter struct{}

func (noopEmitter) Emit(string, string, map[string]any) {}

// Noop returns an Emitter that discards every call. Used in tests and when
// the bridge cannot reach the API server.
func Noop() Emitter { return noopEmitter{} }

type recorderEmitter struct {
	recorder  record.EventRecorder
	ref       *corev1.ObjectReference
	skipKinds map[string]struct{}
}

// New builds an Emitter from in-cluster config. If anything fails the
// Emitter falls back to Noop and the error is returned — callers can log
// and continue.
//
//	source     Source.Component string stamped on every event.
//	skipKinds  Loki-event kinds (e.g. "crud") that should NOT be mirrored.
func New(ctx context.Context, experimentName, namespace, source string, skipKinds []string) (Emitter, error) {
	if experimentName == "" || namespace == "" {
		return Noop(), fmt.Errorf("k8sevents: experimentName and namespace are required")
	}
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return Noop(), fmt.Errorf("k8sevents: in-cluster config: %w", err)
	}
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return Noop(), fmt.Errorf("k8sevents: clientset: %w", err)
	}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return Noop(), fmt.Errorf("k8sevents: dynamic client: %w", err)
	}

	ref := &corev1.ObjectReference{
		APIVersion: experimentGroup + "/" + experimentVersion,
		Kind:       experimentKind,
		Namespace:  namespace,
		Name:       experimentName,
	}
	// UID is best-effort: events still appear under the Experiment via
	// involvedObject.name+namespace+kind even without it. Fetching it
	// lets the events cascade-delete when the Experiment is removed.
	gvr := schema.GroupVersionResource{Group: experimentGroup, Version: experimentVersion, Resource: experimentResource}
	exp, getErr := dyn.Resource(gvr).Namespace(namespace).Get(ctx, experimentName, metav1.GetOptions{})
	switch {
	case getErr == nil:
		ref.UID = exp.GetUID()
	case apierrors.IsNotFound(getErr):
		slog.Warn("k8sevents: experiment CR not found, events will have empty UID",
			"namespace", namespace, "name", experimentName)
	default:
		slog.Warn("k8sevents: cannot fetch experiment UID", "error", getErr)
	}

	bc := record.NewBroadcaster()
	bc.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: clientset.CoreV1().Events(namespace)})
	bc.StartStructuredLogging(0)
	go func() {
		<-ctx.Done()
		bc.Shutdown()
	}()

	skip := make(map[string]struct{}, len(skipKinds))
	for _, k := range skipKinds {
		if k = strings.TrimSpace(k); k != "" {
			skip[k] = struct{}{}
		}
	}

	return &recorderEmitter{
		recorder:  bc.NewRecorder(scheme.Scheme, corev1.EventSource{Component: source}),
		ref:       ref,
		skipKinds: skip,
	}, nil
}

func (e *recorderEmitter) Emit(kind, eventType string, payload map[string]any) {
	if _, skip := e.skipKinds[kind]; skip {
		return
	}
	severity := classify(kind, eventType, payload)
	reason := buildReason(kind, eventType)
	msg := buildMessage(kind, eventType, payload)

	annotations := map[string]string{}
	for _, key := range []string{"experimentTime", "wallTime", "fsNode"} {
		if v, ok := payload[key]; ok {
			annotations[key] = fmt.Sprint(v)
		}
	}

	// AnnotatedEventf accepts any runtime.Object; ObjectReference satisfies it.
	// involvedObject is populated from the reference fields directly.
	if len(annotations) == 0 {
		e.recorder.Event(e.ref, severity, reason, msg)
		return
	}
	e.recorder.AnnotatedEventf(e.ref, annotations, severity, reason, "%s", msg)
}

// classify maps (kind, eventType) to corev1.EventTypeNormal or
// corev1.EventTypeWarning. Adverse transitions become Warning so they stand
// out in `kubectl describe experiment`.
func classify(kind, eventType string, payload map[string]any) string {
	t := strings.ToLower(eventType)
	switch kind {
	case "lifecycle":
		// Lifecycle eventType is the state ("started"/"ended"); the adverse
		// outcome is carried in the reason ("scenario-failure", "scenario-timeout", ...).
		reason := strings.ToLower(fmt.Sprint(payload["reason"]))
		for _, kw := range []string{"fail", "timeout", "error"} {
			if strings.Contains(reason, kw) {
				return corev1.EventTypeWarning
			}
		}
	case "online_state":
		if t == "offline" {
			return corev1.EventTypeWarning
		}
	case "power":
		if t == "enter_low_power" {
			return corev1.EventTypeWarning
		}
	case "hardware":
		for _, kw := range []string{"error", "fail", "fault", "alert"} {
			if strings.Contains(t, kw) {
				return corev1.EventTypeWarning
			}
		}
	}
	return corev1.EventTypeNormal
}

// buildReason composes the K8s Event reason field. K8s event aggregation
// groups by (reason, involvedObject, source, type) — so keeping the reason
// stable per (kind, eventType) lets the broadcaster dedupe spammy streams.
func buildReason(kind, eventType string) string {
	clean := func(s string) string {
		out := strings.Builder{}
		nextUpper := true
		for _, r := range s {
			switch {
			case r == '_' || r == '-' || r == '.' || r == ' ':
				nextUpper = true
			case nextUpper:
				out.WriteRune(toUpper(r))
				nextUpper = false
			default:
				out.WriteRune(r)
			}
		}
		return out.String()
	}
	return clean(kind) + clean(eventType)
}

func toUpper(r rune) rune {
	if r >= 'a' && r <= 'z' {
		return r - 32
	}
	return r
}

func buildMessage(kind, eventType string, payload map[string]any) string {
	pick := func(keys ...string) string {
		for _, k := range keys {
			if v, ok := payload[k]; ok && v != nil {
				if s := fmt.Sprint(v); s != "" {
					return s
				}
			}
		}
		return ""
	}
	fsNode := pick("fsNode")
	switch kind {
	case "crud":
		name := pick("name")
		size := pick("size")
		if name == "" {
			return fmt.Sprintf("%s on %s", eventType, fsNode)
		}
		if size != "" {
			return fmt.Sprintf("%s %s on %s (%sB)", eventType, name, fsNode, size)
		}
		return fmt.Sprintf("%s %s on %s", eventType, name, fsNode)
	case "online_state":
		return fmt.Sprintf("%s went %s", fsNode, eventType)
	case "power":
		bat := pick("batteryWh")
		if bat != "" {
			return fmt.Sprintf("%s %s (batteryWh=%s)", fsNode, eventType, bat)
		}
		return fmt.Sprintf("%s %s", fsNode, eventType)
	case "lifecycle":
		reason := pick("reason")
		if reason != "" {
			return fmt.Sprintf("Experiment state: %s (%s)", eventType, reason)
		}
		return fmt.Sprintf("Experiment state: %s", eventType)
	case "hardware":
		if fsNode == "" {
			return fmt.Sprintf("hardware: %s", eventType)
		}
		return fmt.Sprintf("%s: %s", fsNode, eventType)
	}
	if fsNode != "" {
		return fmt.Sprintf("%s/%s on %s", kind, eventType, fsNode)
	}
	return fmt.Sprintf("%s/%s", kind, eventType)
}

// resolveExperimentRef is a stable bundle the bridge can pass around when
// it needs the involvedObject reference for non-recorder uses (e.g. logs).
func ResolveExperimentRef(experimentName, namespace string, uid types.UID) *corev1.ObjectReference {
	return &corev1.ObjectReference{
		APIVersion: experimentGroup + "/" + experimentVersion,
		Kind:       experimentKind,
		Namespace:  namespace,
		Name:       experimentName,
		UID:        uid,
	}
}
