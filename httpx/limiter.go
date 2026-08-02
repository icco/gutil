// Package httpx provides client-side resilience primitives for talking to
// flaky third-party APIs: a sliding-window rate limiter, a circuit breaker, and
// a Guard that combines both with bounded retries.
//
// The pieces are usable separately. Guard wraps any operation, not just an
// http.Request, so it also fits around a vendor SDK call that does its own
// transport.
//
// This package is deliberately stdlib-only: it takes no logger and emits no
// output. Use Guard.OnRetry if you want retry attempts recorded.
package httpx

import (
	"context"
	"sync"
	"time"
)

// pollInterval is how often Wait re-checks a full window.
const pollInterval = 100 * time.Millisecond

// Limiter is a sliding-window rate limiter. It permits at most max events in
// any trailing window. The zero value is not usable; call NewLimiter.
//
// A window is a smoothing device, not a quota: it keeps a burst of work from
// hammering a service. If the API you are calling publishes a hard per-second
// cap, set max and window to match it; if it publishes a daily quota instead,
// pick something that paces your own batch and cap the batch size separately.
type Limiter struct {
	mu       sync.Mutex
	events   []time.Time
	max      int
	window   time.Duration
	nowFunc  func() time.Time
	sleepDur time.Duration
}

// NewLimiter returns a Limiter allowing max events per window. A max below 1 is
// treated as 1, so a misconfigured limiter throttles rather than deadlocks.
func NewLimiter(max int, window time.Duration) *Limiter {
	if max < 1 {
		max = 1
	}
	return &Limiter{
		max:      max,
		window:   window,
		nowFunc:  time.Now,
		sleepDur: pollInterval,
	}
}

// Allow records and permits an event if one fits in the current window,
// reporting whether it did. It never blocks.
func (l *Limiter) Allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.nowFunc()
	for len(l.events) > 0 && now.Sub(l.events[0]) > l.window {
		l.events = l.events[1:]
	}
	if len(l.events) < l.max {
		l.events = append(l.events, now)
		return true
	}
	return false
}

// Wait blocks until an event is permitted or ctx is done, returning ctx.Err()
// in the latter case.
func (l *Limiter) Wait(ctx context.Context) error {
	for !l.Allow() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(l.sleepDur):
		}
	}
	return nil
}
