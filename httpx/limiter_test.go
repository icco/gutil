package httpx

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestLimiterAllowsUpToMax(t *testing.T) {
	t.Parallel()
	l := NewLimiter(3, time.Minute)

	for i := range 3 {
		if !l.Allow() {
			t.Fatalf("Allow() #%d = false, want true", i+1)
		}
	}
	if l.Allow() {
		t.Error("Allow() past max = true, want false")
	}
}

func TestLimiterEvictsPastWindow(t *testing.T) {
	t.Parallel()
	now := time.Now()
	l := NewLimiter(2, time.Second)
	l.nowFunc = func() time.Time { return now }

	for i := range 2 {
		if !l.Allow() {
			t.Fatalf("Allow() #%d = false, want true", i+1)
		}
	}
	if l.Allow() {
		t.Fatal("third Allow() within window should fail")
	}

	now = now.Add(2 * time.Second)
	if !l.Allow() {
		t.Error("Allow() after window elapsed = false, want true")
	}
}

// A max below 1 must still let work through, one at a time. Clamping to 0 would
// turn a config typo into a permanent deadlock inside Wait.
func TestLimiterClampsNonPositiveMax(t *testing.T) {
	t.Parallel()
	if !NewLimiter(0, time.Minute).Allow() {
		t.Error("NewLimiter(0, ...) never allows; want max clamped to 1")
	}
	if !NewLimiter(-5, time.Minute).Allow() {
		t.Error("NewLimiter(-5, ...) never allows; want max clamped to 1")
	}
}

func TestLimiterWaitReturnsOnCancelledContext(t *testing.T) {
	t.Parallel()
	l := NewLimiter(1, time.Hour)
	l.sleepDur = time.Millisecond
	if !l.Allow() {
		t.Fatal("setup: first Allow() should succeed")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	if err := l.Wait(ctx); err == nil {
		t.Fatal("Wait on a full window with an expiring ctx = nil, want ctx error")
	}
}

func TestLimiterWaitProceedsWhenWindowClears(t *testing.T) {
	t.Parallel()
	l := NewLimiter(1, 20*time.Millisecond)
	l.sleepDur = time.Millisecond
	if !l.Allow() {
		t.Fatal("setup: first Allow() should succeed")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := l.Wait(ctx); err != nil {
		t.Fatalf("Wait = %v, want nil once the window clears", err)
	}
}

func TestLimiterIsConcurrencySafe(t *testing.T) {
	t.Parallel()
	const goroutines = 50
	l := NewLimiter(10, time.Minute)

	var mu sync.Mutex
	allowed := 0
	var wg sync.WaitGroup
	for range goroutines {
		wg.Go(func() {
			if l.Allow() {
				mu.Lock()
				allowed++
				mu.Unlock()
			}
		})
	}
	wg.Wait()

	if allowed != 10 {
		t.Errorf("allowed = %d, want exactly 10", allowed)
	}
}
