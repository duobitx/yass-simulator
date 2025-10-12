package main

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/ESA-PhiLab/yass-internal-components/go-common/proto"
	"github.com/pkg/errors"
)

func (t *appType) start(startTime time.Time) error {
	if t.experimentStartTime != nil {
		return errors.New("experiment already started")
	}
	t.experimentStartTime = &startTime
	// TODO send info about experiment stat to a topic
	tick := time.Tick(1 * time.Second)
	go func() {
		for {
			select {
			case <-t.mainCtx.Done():
				return
			case <-tick:
				err := t.mockSentUpdates()
				if err != nil {
					slog.Default().Error("error sending mock update", "error", err)
				}
			}
		}
	}()
	return nil
}

func (t *appType) mockSentUpdates() error {
	for name := range t.nodes {
		err := t.sendUpdateToFsNode(name)
		if err != nil {
			return err
		}
	}
	return nil
}

func (t *appType) sendUpdateToFsNode(fsNodeName string) error {
	obj := &proto.FsNodeUpdate{
		Id:             fsNodeName,
		InShadow:       false,
		UpdatedUnixSec: time.Now().Unix(),
	}
	return t.facade.Publish(t.mainCtx, fmt.Sprintf("updates/%s", fsNodeName), 0, true, obj)
}
