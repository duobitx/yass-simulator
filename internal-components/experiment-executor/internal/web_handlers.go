package internal

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/duobitx/yass-simulator/internal-components/experiment-executor/consts"
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
	for nodeName, nodeData := range t.nodes {
		if nodeName == fsNode {
			output(w, nodeData)
			return
		}
	}
	w.WriteHeader(http.StatusNotFound)
}

func (t *AppType) handleGetFsNodesList(w http.ResponseWriter, r *http.Request) {
	typeFilter := r.URL.Query().Get("type")
	t.nodesLock.Lock()
	list := make([]string, 0, len(t.nodeTypes))
	for nodeName, nodeType := range t.nodeTypes {
		if typeFilter == "" || string(nodeType) == typeFilter {
			list = append(list, nodeName)
		}
	}
	t.nodesLock.Unlock()
	output(w, list)
}

func (t *AppType) DefineEndpoints(router *mux.Router) {
	if router == nil {
		panic("router cannot be nil")
	}
	router.HandleFunc("/", t.handleRoot)
	router.HandleFunc("/info", t.handleInfo).Methods(http.MethodGet)
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
