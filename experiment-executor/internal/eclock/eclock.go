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

func NewExperimentClock(ctx context.Context, startAt time.Time, tickInterval, tickDelta time.Duration, maxDuration *time.Duration) EClock {
	instance := &eClock{
		now:  startAt,
		done: make(chan struct{}),
		t:    make(chan time.Time),
	}
	if maxDuration != nil {
		end := startAt.Add(*maxDuration)
		instance.end = &end
	}
	ti := time.NewTicker(tickInterval)
	go func() {
		defer func() {
			ti.Stop()
			close(instance.done)
			close(instance.t)
		}()
		for {
			select {
			case <-ti.C:
				now := instance.now.Add(tickDelta)
				if instance.end != nil && instance.end.Before(now) {
					now = instance.now
				}
				instance.now = now
				instance.t <- now
			case <-ctx.Done():
				return
			}
		}
	}()
	return instance
}

func NewExperimentRealClock(ctx context.Context, startAt time.Time, maxDuration *time.Duration) EClock {
	tickInterval := 1000 * time.Millisecond
	return NewExperimentClock(ctx, startAt, tickInterval, tickInterval, maxDuration)
}
