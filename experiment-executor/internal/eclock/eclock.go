package eclock

import (
	"context"
	"fmt"
	"time"
)

type EClock interface {
	Done() <-chan struct{}
	Now() time.Time
	Tick() <-chan time.Time
	SetTime(newTime time.Time)
}

type eClock struct {
	now    time.Time
	end    *time.Time
	done   chan struct{}
	t      chan time.Time
	cancel context.CancelCauseFunc
}

func (e *eClock) SetTime(newTime time.Time) {
	e.now = newTime
	e.t <- newTime
	if e.end != nil && newTime.After(*e.end) {
		e.cancel(fmt.Errorf("experiment clock stopped"))
	}
}

func (e *eClock) Tick() <-chan time.Time {
	return e.t
}

func (e *eClock) Done() <-chan struct{} {
	return e.done
}

func (e *eClock) Now() time.Time {
	return e.now
}

func NewExperimentClock(parentCtx context.Context, startAt time.Time, maxDuration *time.Duration) (EClock, error) {
	instance := &eClock{
		now:  startAt,
		done: make(chan struct{}),
		t:    make(chan time.Time),
	}
	if maxDuration != nil {
		end := startAt.Add(*maxDuration)
		instance.end = &end
	}
	ctx, cancel := context.WithCancelCause(parentCtx)
	instance.cancel = cancel
	go func() {
		defer func() {
			close(instance.done)
			close(instance.t)
		}()
		<-ctx.Done()
	}()
	return instance, nil
}
