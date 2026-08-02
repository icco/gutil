package httpx

import (
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
