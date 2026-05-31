package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/duobitx/yass-simulator/internal-components/events-webapp/internal/conv"
	"github.com/duobitx/yass-simulator/internal-components/events-webapp/pkg/api"
	"github.com/duobitx/yass-simulator/internal-components/go-common/startup"
	com "github.com/m-szalik/com-facade"
	"github.com/m-szalik/com-facade/mqtt"
	"github.com/m-szalik/goutils"
	"github.com/m-szalik/goutils/pubsub"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/apimachinery/pkg/util/rand"
)

const agentEventsHistoryPerNode = 50

type appType struct {
	mainCtx           context.Context
	facade            com.Facade
	ps                pubsub.PubSub[any]
	psProducer        chan<- any
	eventsFilePath    string
	agentEventsMu     sync.Mutex
	agentEventsByNode map[string][]api.AgentFileEvent
}

func (t *appType) rememberAgentEvent(e api.AgentFileEvent) {
	t.agentEventsMu.Lock()
	defer t.agentEventsMu.Unlock()
	list := t.agentEventsByNode[e.Source]
	list = append(list, e)
	if len(list) > agentEventsHistoryPerNode {
		list = list[len(list)-agentEventsHistoryPerNode:]
	}
	t.agentEventsByNode[e.Source] = list
}

func (t *appType) snapshotAgentEvents() []api.AgentFileEvent {
	t.agentEventsMu.Lock()
	defer t.agentEventsMu.Unlock()
	out := make([]api.AgentFileEvent, 0)
	for _, list := range t.agentEventsByNode {
		out = append(out, list...)
	}
	return out
}

func (t *appType) eventsSSE(w http.ResponseWriter, request *http.Request) {
	// Required headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Important: allow streaming
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}
	clientID := fmt.Sprintf("httpCl-%s", request.RemoteAddr)
	slog.Info("Incoming connection", "clientID", clientID)
	// Subscribe FIRST so concurrent live events aren't lost between replay and live.
	ch := t.ps.NewSubscriber(request.Context())
	// pubsub.push() does a blocking send on an unbuffered subscriber channel,
	// so a slow client (or one stalled mid-replay/flush) would stall delivery
	// to every other subscriber and to the file saver. Insulate the shared
	// dispatcher: drain into a buffered queue and drop events for THIS client
	// when it falls behind.
	buffered := make(chan any, 1024)
	go func() {
		for {
			select {
			case <-request.Context().Done():
				return
			case evt, ok := <-ch:
				if !ok {
					return
				}
				select {
				case buffered <- evt:
				default:
					// client too slow; drop to protect the shared dispatcher
				}
			}
		}
	}()
	// Replay agent-event history so new clients see PUT/RECEIVED/DELETE that
	// landed before they connected. crud-events MQTT is retained but only the
	// last message per topic, so the broker alone doesn't cover this.
	for _, e := range t.snapshotAgentEvents() {
		buff, err := json.Marshal(e)
		if err != nil {
			continue
		}
		if _, err = fmt.Fprintln(w, string(buff)); err != nil {
			slog.Info("Client disconnected during replay", "clientID", clientID, "error", err)
			return
		}
	}
	flusher.Flush()
	for {
		select {
		case evt := <-buffered:
			if evt == nil {
				continue
			}
			buff, err := json.Marshal(evt)
			if err != nil {
				slog.Error("error marshalling event", "error", err, "clientID", clientID)
				continue
			}
			_, err = fmt.Fprintln(w, string(buff))
			if err != nil {
				slog.Error("error sending event", "error", err, "clientID", clientID)
				continue
			}
			flusher.Flush()

		case <-request.Context().Done():
			slog.Info("Client disconnected", "clientID", clientID)
			return
		}
	}
}

func (t *appType) message(_ context.Context, topic string, _ bool, data []byte) {
	var cf conv.CFunc
	if strings.HasPrefix(topic, "updates") && !strings.HasSuffix(topic, "_") {
		cf = conv.FsNodeUpdateConv
	}
	if strings.HasPrefix(topic, "total-network-stats") && !strings.HasSuffix(topic, "_") {
		cf = conv.FsNodeNetworkUsageConv
	}
	if strings.HasSuffix(topic, "/resources") {
		cf = conv.FsNodeResourcesConv
	}
	if topic == "crud-events" {
		cf = conv.AgentFileEventConv
	}
	if strings.HasPrefix(topic, "hardware-events/") && !strings.HasSuffix(topic, "_") {
		cf = conv.HardwareEventConv
	}
	if cf != nil {
		apiResponse, err := cf(topic, data)
		if err != nil {
			slog.Error("error unmarshalling message", "error", err)
		}
		if apiResponse != nil {
			if af, ok := apiResponse.(api.AgentFileEvent); ok {
				t.rememberAgentEvent(af)
			}
			t.psProducer <- apiResponse
		}
	}
}

func (t *appType) saveEventsToFile(ctx context.Context) {
	if t.eventsFilePath == "" {
		return
	}
	f, err := os.OpenFile(t.eventsFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		slog.Error("error opening events file", "error", err, "path", t.eventsFilePath)
		return
	}
	defer goutils.CloseQuietly(f)

	slog.Info("Saving events to file", "path", t.eventsFilePath)
	ch := t.ps.NewSubscriber(ctx)
	for {
		select {
		case evt := <-ch:
			if evt == nil {
				continue
			}
			buff, err := json.Marshal(evt)
			if err != nil {
				slog.Error("error marshalling event for file", "error", err)
				continue
			}
			_, err = fmt.Fprintln(f, string(buff))
			if err != nil {
				slog.Error("error writing event to file", "error", err)
				continue
			}

		case <-ctx.Done():
			slog.Info("Stop saving events to file", "path", t.eventsFilePath)
			return
		}
	}
}

const appName = "events-webapp"

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func main() {
	goutils.ExitOnErrorf(startup.InitSlog(), 1, "cannot initialize slog")
	slog.Info("Events Webapp")
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()
	facade := mqtt.NewFacade(ctx, fmt.Sprintf("%s-%d", appName, rand.Intn(100)),
		mqtt.WithHostPort("tcp://"+goutils.Env("MESSAGING_BROKER_HOST_PORT", "messaging:1883")))

	ps := pubsub.NewPubSub[any](ctx)
	app := &appType{
		mainCtx:           ctx,
		facade:            facade,
		ps:                ps,
		psProducer:        ps.NewPublisher(),
		eventsFilePath:    getEnv("EVENTS_FILE_PATH", "events.log"),
		agentEventsByNode: make(map[string][]api.AgentFileEvent),
	}

	err := facade.Connect()
	goutils.ExitOnError(err, 2)

	err = startup.FileProbe(ctx)
	goutils.ExitOnError(err, 6)
	go app.saveEventsToFile(ctx)
	slog.Info("StartupCompleted")

	http.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
	})
	http.HandleFunc("/events-sse", app.eventsSSE)
	listenOn := fmt.Sprintf(":%d", 8080)
	fmt.Printf("Server running on %s\n", listenOn)

	err = facade.Subscribe("#", app.message)
	goutils.ExitOnError(err, 8)

	err = http.ListenAndServe(listenOn, nil)
	goutils.ExitOnError(err, 8)

	<-ctx.Done()
	time.Sleep(1 * time.Second)
	slog.Info("Terminated")
}
