package main

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ESA-PhiLab/yass-internal-components/experiment-executor/consts"
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

func (t *appType) handleRoot(w http.ResponseWriter, r *http.Request) {
	message := fmt.Sprintf("Application %s\n", consts.AppName)
	_, _ = w.Write([]byte(message))
}

func (t *appType) handleStartExperiment(w http.ResponseWriter, r *http.Request) {
	buff, err := io.ReadAll(r.Body)
	if handleError(err, w) {
		return
	}
	startAt := time.Now()
	if buff != nil && len(buff) > 0 {
		js := map[string]any{}
		err = json.Unmarshal(buff, &js)
		if handleError(err, w) {
			return
		}
		timeStr := js["startAt"]
		startAt, err = time.Parse(time.RFC3339, fmt.Sprint(timeStr))
		if handleError(err, w) {
			return
		}
	}
	err = t.start(startAt)
	if handleError(err, w) {
		return
	}
	_, _ = w.Write([]byte(fmt.Sprintf("OK\n%s\n", string(buff))))
}

func (t *appType) defineEndpoints(router *mux.Router) {
	if router == nil {
		panic("router cannot be nil")
	}
	router.HandleFunc("/", t.handleRoot)
	router.HandleFunc("/start", t.handleStartExperiment).Methods(http.MethodPost)
}
