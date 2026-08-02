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

	// trialInFlight gates the half-open state to one probe. Without it, every
	// concurrent caller is admitted the moment the timeout expires, which is the
	// full pre-breaker load aimed at a service that has not been shown healthy.
	trialInFlight bool
	// trialStart bounds that gate. A trial whose caller never reports back —
	// cancelled context, panic, a Guard path that returns before recording —
	// would otherwise wedge the breaker shut forever.
	trialStart time.Time
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

// Allow reports whether the breaker permits a call. It transitions an expired
// open breaker to half-open as a side effect, and admits exactly one trial call
// in that state: every caller after the first is rejected until RecordSuccess
// or RecordFailure resolves the trial.
//
// A caller that is admitted must report its outcome. One that never does is
// released after timeout so a dropped trial cannot wedge the breaker shut.
func (b *Breaker) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := b.nowFunc()
	switch b.state {
	case stateClosed:
		return true
	case stateOpen:
		if now.Sub(b.lastFailure) >= b.timeout {
			b.state = stateHalfOpen
			b.trialInFlight = true
			b.trialStart = now
			return true
		}
		return false
	case stateHalfOpen:
		if !b.trialInFlight || now.Sub(b.trialStart) >= b.timeout {
			b.trialInFlight = true
			b.trialStart = now
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
	b.trialInFlight = false
}

// RecordFailure counts a failure, opening the breaker once maxFailures is hit.
// A failure in the half-open state always re-opens it: the trial existed to
// answer exactly this question.
func (b *Breaker) RecordFailure() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.failures++
	b.lastFailure = b.nowFunc()
	if b.state == stateHalfOpen || b.failures >= b.maxFailures {
		b.state = stateOpen
	}
	b.trialInFlight = false
}
