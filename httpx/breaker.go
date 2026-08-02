package httpx

import (
	"errors"
	"sync"
	"time"
)

// ErrCircuitOpen is returned by Guard.Run when the breaker is open. Callers
// match it with errors.Is to skip retry and log loops for a known-down service.
var ErrCircuitOpen = errors.New("httpx: circuit open")

type state int

const (
	stateClosed state = iota
	stateOpen
	stateHalfOpen
)

// Breaker is a circuit breaker. After maxFailures consecutive failures it opens
// and rejects calls for timeout, then admits a single trial call (half-open); a
// success closes it, a failure re-opens it. The zero value is not usable; call
// NewBreaker.
type Breaker struct {
	mu          sync.Mutex
	state       state
	failures    int
	lastFailure time.Time
	maxFailures int
	timeout     time.Duration
	nowFunc     func() time.Time
}

// NewBreaker returns a Breaker that opens after maxFailures consecutive
// failures and stays open for timeout. A maxFailures below 1 is treated as 1.
func NewBreaker(maxFailures int, timeout time.Duration) *Breaker {
	if maxFailures < 1 {
		maxFailures = 1
	}
	return &Breaker{
		maxFailures: maxFailures,
		timeout:     timeout,
		nowFunc:     time.Now,
	}
}

// Allow reports whether the breaker permits a call, transitioning an expired
// open breaker to half-open as a side effect.
func (b *Breaker) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case stateClosed, stateHalfOpen:
		return true
	case stateOpen:
		if b.nowFunc().Sub(b.lastFailure) > b.timeout {
			b.state = stateHalfOpen
			return true
		}
		return false
	default:
		return false
	}
}

// RecordSuccess closes the breaker and clears the failure count.
func (b *Breaker) RecordSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.failures = 0
	b.state = stateClosed
}

// RecordFailure counts a failure, opening the breaker once maxFailures is hit.
func (b *Breaker) RecordFailure() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.failures++
	b.lastFailure = b.nowFunc()
	if b.failures >= b.maxFailures {
		b.state = stateOpen
	}
}
