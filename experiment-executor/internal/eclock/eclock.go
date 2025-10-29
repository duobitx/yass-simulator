package eclock

import (
	"context"
	"time"
)

type EClock interface {
	Done() <-chan struct{}
	Now() time.Time
	Tick() <-chan time.Time
}

type eClock struct {
	now   time.Time
	end   *time.Time
	delta time.Duration
	done  chan struct{}
	t     chan time.Time
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

func NewExperimentClock(ctx context.Context, timeSource <-chan time.Time, maxDuration *time.Duration) (EClock, error) {
	var startAt time.Time
	select {
	case t := <-timeSource:
		startAt = t
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	instance := &eClock{
		now:  startAt,
		done: make(chan struct{}),
		t:    make(chan time.Time),
	}
	if maxDuration != nil {
		end := startAt.Add(*maxDuration)
		instance.end = &end
	}
	go func() {
		defer func() {
			close(instance.done)
			close(instance.t)
		}()
		for {
			select {
			case tNow := <-timeSource:
				instance.now = tNow
				instance.t <- tNow
			case <-ctx.Done():
				return
			}
		}
	}()
	return instance, nil
}
