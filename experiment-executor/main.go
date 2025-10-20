package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/mux"

	"github.com/ESA-PhiLab/yass-internal-components/experiment-executor/consts"
	"github.com/ESA-PhiLab/yass-internal-components/experiment-executor/internal/model"
	"github.com/ESA-PhiLab/yass-internal-components/go-common/com"
	"github.com/ESA-PhiLab/yass-internal-components/go-common/proto"
	"github.com/ESA-PhiLab/yass-internal-components/go-common/startup"
	"github.com/m-szalik/goutils"
)

type appType struct {
	mainCtx             context.Context
	experimentStartTime *time.Time
	facade              com.Facade
	experiment          string
	nodes               map[string]*model.FsNodeState
	nodesLock           sync.Mutex
}

func (t *appType) handleOnlineUpdate(_ context.Context, data []byte) error {
	msg := &proto.FsNodeOnlineState{}
	err := com.MsgUnmarshall(data, msg)
	if err != nil {
		return err
	}
	if t.experiment != msg.FsNodeId.Experiment {
		return nil
	}
	t.nodesLock.Lock()
	defer t.nodesLock.Unlock()
	if state, ok := t.nodes[msg.FsNodeId.Name]; !ok {
		state = &model.FsNodeState{
			Online: msg.Online,
			IP:     msg.Ip,
		}
		t.nodes[msg.FsNodeId.Name] = state
	} else {
		state.Online = msg.Online
	}
	return nil
}

func main() {
	experiment := goutils.EnvRequired[string]("YASS_EXPERIMENT")
	slog.Info("ExperimentExecutor", "experiment", experiment)
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM)
	defer cancel()
	facade := com.NewFacade(ctx, consts.AppName)
	app := &appType{
		mainCtx:    ctx,
		facade:     facade,
		experiment: experiment,
		nodes:      map[string]*model.FsNodeState{},
	}
	err := facade.Connect()
	goutils.ExitOnError(err, 2)

	err = facade.Subscribe("/online-states/#", func(sCtx context.Context, topic string, retained bool, data []byte) {
		err := app.handleOnlineUpdate(sCtx, data)
		if err != nil {
			slog.Error("error handling incoming update data", "data", string(data), "topic", topic, "error", err)
		}
	})
	goutils.ExitOnError(err, 4)

	err = startup.FileProbe(ctx, consts.AppName)
	goutils.ExitOnError(err, 5)

	router := mux.NewRouter()
	fmt.Println(router, app)
	app.defineEndpoints(router)
	srv := &http.Server{
		Handler: router,
		Addr:    ":8080",
	}
	go func() {
		<-ctx.Done()
		ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	fmt.Println("Server running on http://localhost:8080")
	slog.Info("StartupCompleted")
	err = srv.ListenAndServe()
	slog.Default().Error("webServer", "error", err)
	slog.Default().Info("Terminated")
}
