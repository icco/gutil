package httpx

import (
	"sync"
	"testing"
	"time"
)

func TestBreakerOpensAtMaxFailures(t *testing.T) {
	t.Parallel()
	b := NewBreaker(3, time.Minute)

	for i := range 2 {
		b.RecordFailure()
		if !b.Allow() {
			t.Fatalf("Allow() after %d failures = false, want true", i+1)
		}
	}
	b.RecordFailure()
	if b.Allow() {
		t.Error("Allow() after 3 failures = true, want false")
	}
}

func TestBreakerSuccessResetsFailureCount(t *testing.T) {
	t.Parallel()
	b := NewBreaker(3, time.Minute)

	b.RecordFailure()
	b.RecordFailure()
	b.RecordSuccess()
	b.RecordFailure()
	b.RecordFailure()

	if !b.Allow() {
		t.Error("Allow() = false; a success should have cleared the earlier failures")
	}
}

func TestBreakerHalfOpensAfterTimeout(t *testing.T) {
	t.Parallel()
	now := time.Now()
	b := NewBreaker(1, time.Minute)
	b.nowFunc = func() time.Time { return now }

	b.RecordFailure()
	if b.Allow() {
		t.Fatal("Allow() immediately after opening = true, want false")
	}

	now = now.Add(2 * time.Minute)
	if !b.Allow() {
		t.Fatal("Allow() after timeout = false, want true (half-open trial)")
	}
	if b.state != stateHalfOpen {
		t.Errorf("state = %v, want half-open", b.state)
	}
}

// Half-open must admit exactly one probe. Letting every waiting caller through
// the moment the timeout expires aims the full pre-breaker load at a service
// that has not been shown healthy yet — the thing the breaker exists to stop.
func TestBreakerHalfOpenAdmitsOneTrial(t *testing.T) {
	t.Parallel()
	now := time.Now()
	b := NewBreaker(1, time.Minute)
	b.nowFunc = func() time.Time { return now }

	b.RecordFailure()
	now = now.Add(2 * time.Minute)

	if !b.Allow() {
		t.Fatal("first Allow() after timeout = false, want the trial")
	}
	for i := range 3 {
		if b.Allow() {
			t.Fatalf("Allow() #%d during an in-flight trial = true, want false", i+2)
		}
	}
}

func TestBreakerHalfOpenAdmitsAgainAfterTrialResolves(t *testing.T) {
	t.Parallel()
	now := time.Now()
	b := NewBreaker(1, time.Minute)
	b.nowFunc = func() time.Time { return now }

	b.RecordFailure()
	now = now.Add(2 * time.Minute)
	if !b.Allow() {
		t.Fatal("setup: expected a trial")
	}
	b.RecordSuccess()

	// Closed again, so traffic flows freely.
	for i := range 3 {
		if !b.Allow() {
			t.Errorf("Allow() #%d after a successful trial = false, want true", i+1)
		}
	}
}

// A trial whose caller never reports back — cancelled context, panic, an early
// return before Record* — must not wedge the breaker shut forever.
func TestBreakerHalfOpenTrialSelfHeals(t *testing.T) {
	t.Parallel()
	now := time.Now()
	b := NewBreaker(1, time.Minute)
	b.nowFunc = func() time.Time { return now }

	b.RecordFailure()
	now = now.Add(2 * time.Minute)
	if !b.Allow() {
		t.Fatal("setup: expected a trial")
	}
	if b.Allow() {
		t.Fatal("setup: second Allow() should be gated")
	}

	// The trial never resolved. After another timeout, admit a fresh one.
	now = now.Add(2 * time.Minute)
	if !b.Allow() {
		t.Error("Allow() after an abandoned trial aged out = false, want a fresh trial")
	}
}

// Concurrent callers arriving the instant the timeout expires must not all slip
// through: exactly one gets the probe.
func TestBreakerHalfOpenTrialIsConcurrencySafe(t *testing.T) {
	t.Parallel()
	now := time.Now()
	b := NewBreaker(1, time.Minute)
	b.nowFunc = func() time.Time { return now }

	b.RecordFailure()
	now = now.Add(2 * time.Minute)

	var mu sync.Mutex
	admitted := 0
	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			if b.Allow() {
				mu.Lock()
				admitted++
				mu.Unlock()
			}
		})
	}
	wg.Wait()

	if admitted != 1 {
		t.Errorf("admitted = %d concurrent callers, want exactly 1", admitted)
	}
}

// The breaker is open for exactly timeout, not timeout plus an instant.
func TestBreakerReopensAtExactlyTimeout(t *testing.T) {
	t.Parallel()
	now := time.Now()
	b := NewBreaker(1, time.Minute)
	b.nowFunc = func() time.Time { return now }

	b.RecordFailure()
	now = now.Add(time.Minute)
	if !b.Allow() {
		t.Error("Allow() at exactly timeout = false, want true")
	}
}

// A half-open trial that fails must re-open the breaker rather than leaving it
// admitting traffic to a service that is still down.
func TestBreakerHalfOpenFailureReopens(t *testing.T) {
	t.Parallel()
	now := time.Now()
	b := NewBreaker(1, time.Minute)
	b.nowFunc = func() time.Time { return now }

	b.RecordFailure()
	now = now.Add(2 * time.Minute)
	if !b.Allow() {
		t.Fatal("setup: expected a half-open trial")
	}

	b.RecordFailure()
	if b.Allow() {
		t.Error("Allow() after a failed half-open trial = true, want false")
	}
}

func TestBreakerHalfOpenSuccessCloses(t *testing.T) {
	t.Parallel()
	now := time.Now()
	b := NewBreaker(1, time.Minute)
	b.nowFunc = func() time.Time { return now }

	b.RecordFailure()
	now = now.Add(2 * time.Minute)
	if !b.Allow() {
		t.Fatal("setup: expected a half-open trial")
	}

	b.RecordSuccess()
	if b.state != stateClosed {
		t.Errorf("state = %v, want closed", b.state)
	}
	if !b.Allow() {
		t.Error("Allow() after a successful trial = false, want true")
	}
}

func TestBreakerClampsNonPositiveMaxFailures(t *testing.T) {
	t.Parallel()
	b := NewBreaker(0, time.Minute)
	if !b.Allow() {
		t.Fatal("a fresh breaker should allow")
	}
	b.RecordFailure()
	if b.Allow() {
		t.Error("Allow() after one failure = true; want maxFailures clamped to 1")
	}
}
