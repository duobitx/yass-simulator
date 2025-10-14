package internal

import (
	"fmt"
	"net/http"

	"github.com/ESA-PhiLab/yass-internal-components/experiment-executor/consts"
	"github.com/gorilla/mux"
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

func (t *AppType) handleStartExperiment(w http.ResponseWriter, r *http.Request) {
	err := t.Start()
	if handleError(err, w) {
		return
	}
	_, _ = w.Write([]byte(fmt.Sprintf("OK\n")))
}

func (t *AppType) DefineEndpoints(router *mux.Router) {
	if router == nil {
		panic("router cannot be nil")
	}
	router.HandleFunc("/", t.handleRoot)
	router.HandleFunc("/start", t.handleStartExperiment).Methods(http.MethodPost)
}
