package internal

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/duobitx/yass-simulator/internal-components/experiment-executor/consts"
	"github.com/duobitx/yass-simulator/internal-components/experiment-executor/internal/model"
	"github.com/gorilla/mux"
	"k8s.io/apimachinery/pkg/util/json"
)

func handleError(err error, w http.ResponseWriter) bool {
	if err != nil {
		message := fmt.Sprintf("error: %s\n", err)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(message))
	}
	return err != nil
}

func (t *AppType) handleRoot(w http.ResponseWriter, r *http.Request) {
	message := fmt.Sprintf("Application %s\n", consts.AppName)
	_, _ = w.Write([]byte(message))
}

func (t *AppType) handleInfo(w http.ResponseWriter, r *http.Request) {
	output(w, map[string]string{"name": t.ExperimentDefData.Name})
}

func (t *AppType) handleStartExperiment(w http.ResponseWriter, r *http.Request) {
	slog.Info("Experiment start requested...")
	err := t.Start(t.mainCtx)
	if handleError(err, w) {
		return
	}
	_, _ = w.Write([]byte("OK\n"))
}

func (t *AppType) handleGetFsNodeData(w http.ResponseWriter, r *http.Request) {
	match := mux.Vars(r)
	fsNode := match["fsNode"]
	t.nodesLock.Lock()
	nodeData, ok := t.nodes[fsNode]
	var snapshot model.FsNodeState
	if ok {
		snapshot = *nodeData
	}
	t.nodesLock.Unlock()
	if ok {
		output(w, snapshot)
		return
	}
	w.WriteHeader(http.StatusNotFound)
}

// fsNodeDetail is the full per-node view returned by GET /fsNodes?detail=true:
// the node name and type plus the live runtime state (online, IP, position)
// the executor tracks from MQTT online-state and geo updates.
type fsNodeDetail struct {
	Name string `json:"name"`
	Type string `json:"type"`
	model.FsNodeState
}

func (t *AppType) handleGetFsNodesList(w http.ResponseWriter, r *http.Request) {
	typeFilter := r.URL.Query().Get("type")
	detail := r.URL.Query().Get("detail") == "true"
	t.nodesLock.Lock()
	if detail {
		list := make([]fsNodeDetail, 0, len(t.nodeTypes))
		for name, nodeType := range t.nodeTypes {
			if typeFilter != "" && string(nodeType) != typeFilter {
				continue
			}
			d := fsNodeDetail{Name: name, Type: string(nodeType)}
			if st, ok := t.nodes[name]; ok {
				d.FsNodeState = *st
			}
			list = append(list, d)
		}
		t.nodesLock.Unlock()
		output(w, list)
		return
	}
	list := make([]string, 0, len(t.nodeTypes))
	for nodeName, nodeType := range t.nodeTypes {
		if typeFilter == "" || string(nodeType) == typeFilter {
			list = append(list, nodeName)
		}
	}
	t.nodesLock.Unlock()
	output(w, list)
}

// handleGetTime reports the current simulated experiment time and whether the
// run is still advancing. ongoing is false before start and once the run has
// reached a terminal state.
func (t *AppType) handleGetTime(w http.ResponseWriter, r *http.Request) {
	resp := map[string]any{"ongoing": t.ongoing.Load()}
	if et := t.experimentTime.Load(); et != nil {
		resp["experimentTime"] = et.UTC()
	}
	output(w, resp)
}

func (t *AppType) DefineEndpoints(router *mux.Router) {
	if router == nil {
		panic("router cannot be nil")
	}
	router.HandleFunc("/", t.handleRoot)
	router.HandleFunc("/info", t.handleInfo).Methods(http.MethodGet)
	router.HandleFunc("/time", t.handleGetTime).Methods(http.MethodGet)
	router.HandleFunc("/start", t.handleStartExperiment).Methods(http.MethodPost)
	router.HandleFunc("/fsNodes/{fsNode}", t.handleGetFsNodeData).Methods(http.MethodGet)
	router.HandleFunc("/fsNodes", t.handleGetFsNodesList).Methods(http.MethodGet)
}

func output(w http.ResponseWriter, payload any) {
	buff, err := json.Marshal(payload)
	if err != nil {
		handleError(err, w)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(buff)
}
