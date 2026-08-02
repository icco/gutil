package httpx

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// noBackoff keeps retry tests from actually sleeping the default 1s, 2s.
func noBackoff(int) time.Duration { return 0 }

func TestGuardRunSucceedsFirstTry(t *testing.T) {
	t.Parallel()
	calls := 0
	g := &Guard{Backoff: noBackoff}

	if err := g.Run(t.Context(), func(context.Context) error {
		calls++
		return nil
	}); err != nil {
		t.Fatalf("Run = %v, want nil", err)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}

func TestGuardRunRetriesToDefaultAttempts(t *testing.T) {
	t.Parallel()
	calls := 0
	wantErr := errors.New("boom")
	g := &Guard{Backoff: noBackoff}

	err := g.Run(t.Context(), func(context.Context) error {
		calls++
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Run = %v, want %v", err, wantErr)
	}
	if calls != DefaultAttempts {
		t.Errorf("calls = %d, want %d", calls, DefaultAttempts)
	}
}

func TestGuardRunSucceedsOnRetry(t *testing.T) {
	t.Parallel()
	calls := 0
	g := &Guard{Backoff: noBackoff}

	if err := g.Run(t.Context(), func(context.Context) error {
		calls++
		if calls < 3 {
			return errors.New("transient")
		}
		return nil
	}); err != nil {
		t.Fatalf("Run = %v, want nil", err)
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
}

// A Permanent error is the service answering correctly. Retrying it burns quota
// to get the same answer, so Run must return on the first one.
func TestGuardRunDoesNotRetryPermanent(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("not found")
	calls := 0
	g := &Guard{Backoff: noBackoff}

	err := g.Run(t.Context(), func(context.Context) error {
		calls++
		return Permanent(sentinel)
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Run = %v, want %v", err, sentinel)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (no retry on permanent)", calls)
	}
}

// The Permanent wrapper must not survive into the returned error: callers
// compare against their own sentinel and should not see httpx internals.
func TestGuardRunUnwrapsPermanent(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("not found")
	g := &Guard{Backoff: noBackoff}

	err := g.Run(t.Context(), func(context.Context) error {
		return Permanent(sentinel)
	})
	if err != sentinel { //nolint:errorlint // identity is exactly what is under test
		t.Fatalf("Run = %#v, want the bare sentinel", err)
	}
	if IsPermanent(err) {
		t.Error("returned error is still marked Permanent")
	}
}

// A permanent failure says nothing about service health, so it must not count
// toward opening the breaker.
func TestGuardPermanentDoesNotTripBreaker(t *testing.T) {
	t.Parallel()
	b := NewBreaker(2, time.Minute)
	g := &Guard{Breaker: b, Backoff: noBackoff}

	for range 5 {
		_ = g.Run(t.Context(), func(context.Context) error {
			return Permanent(errors.New("not found"))
		})
	}
	if !b.Allow() {
		t.Error("breaker opened on permanent errors; want it untouched")
	}
}

func TestGuardRunReturnsErrCircuitOpen(t *testing.T) {
	t.Parallel()
	b := NewBreaker(1, time.Minute)
	b.RecordFailure()

	calls := 0
	g := &Guard{Breaker: b, Backoff: noBackoff}
	err := g.Run(t.Context(), func(context.Context) error {
		calls++
		return nil
	})

	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("Run = %v, want ErrCircuitOpen", err)
	}
	if calls != 0 {
		t.Errorf("calls = %d, want 0 (fn must not run behind an open breaker)", calls)
	}
}

func TestGuardRunTripsBreakerAcrossRetries(t *testing.T) {
	t.Parallel()
	b := NewBreaker(2, time.Minute)
	g := &Guard{Breaker: b, Attempts: 5, Backoff: noBackoff}

	calls := 0
	err := g.Run(t.Context(), func(context.Context) error {
		calls++
		return errors.New("down")
	})

	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("Run = %v, want ErrCircuitOpen once the breaker trips mid-retry", err)
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2 (breaker opens after the second failure)", calls)
	}
}

func TestGuardRunRecordsSuccessOnBreaker(t *testing.T) {
	t.Parallel()
	b := NewBreaker(2, time.Minute)
	b.RecordFailure()
	g := &Guard{Breaker: b, Backoff: noBackoff}

	if err := g.Run(t.Context(), func(context.Context) error { return nil }); err != nil {
		t.Fatalf("Run = %v, want nil", err)
	}
	// One prior failure plus one more should not open a 2-failure breaker if the
	// success in between reset the count.
	b.RecordFailure()
	if !b.Allow() {
		t.Error("breaker opened; want the intervening success to have reset it")
	}
}

func TestGuardRunStopsOnCancelledContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	g := &Guard{Attempts: 5, Backoff: noBackoff}

	err := g.Run(ctx, func(context.Context) error {
		calls++
		cancel()
		return errors.New("boom")
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run = %v, want context.Canceled", err)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}

func TestGuardRunHonorsLimiter(t *testing.T) {
	t.Parallel()
	l := NewLimiter(1, time.Hour)
	if !l.Allow() {
		t.Fatal("setup: window should start empty")
	}
	l.sleepDur = time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	calls := 0
	g := &Guard{Limiter: l, Backoff: noBackoff}
	err := g.Run(ctx, func(context.Context) error {
		calls++
		return nil
	})

	if err == nil {
		t.Fatal("Run = nil, want a ctx error while waiting on a full window")
	}
	if calls != 0 {
		t.Errorf("calls = %d, want 0 (limiter must gate before fn runs)", calls)
	}
}

func TestGuardOnRetryFiresPerRetriedAttempt(t *testing.T) {
	t.Parallel()
	var attempts []int
	g := &Guard{
		Attempts: 3,
		Backoff:  noBackoff,
		OnRetry:  func(attempt int, _ error) { attempts = append(attempts, attempt) },
	}

	_ = g.Run(t.Context(), func(context.Context) error { return errors.New("boom") })

	// Three attempts means two retries; the final failure is returned, not retried.
	want := []int{1, 2}
	if len(attempts) != len(want) {
		t.Fatalf("OnRetry attempts = %v, want %v", attempts, want)
	}
	for i := range want {
		if attempts[i] != want[i] {
			t.Errorf("OnRetry attempts = %v, want %v", attempts, want)
			break
		}
	}
}

func TestGuardRunClampsAttempts(t *testing.T) {
	t.Parallel()
	calls := 0
	g := &Guard{Attempts: -1, Backoff: noBackoff}

	_ = g.Run(t.Context(), func(context.Context) error {
		calls++
		return errors.New("boom")
	})
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}

func TestPermanentNilIsNil(t *testing.T) {
	t.Parallel()
	if err := Permanent(nil); err != nil {
		t.Errorf("Permanent(nil) = %v, want nil", err)
	}
}

func TestDoAppliesSign(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("apikey"); got != "secret" {
			t.Errorf("apikey = %q, want secret", got)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := Do(srv.Client(), req, func(r *http.Request) {
		q := r.URL.Query()
		q.Set("apikey", "secret")
		r.URL.RawQuery = q.Encode()
	})
	if err != nil {
		t.Fatalf("Do = %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
}

// Go's net/http puts the request URL in transport errors. Anything signed into
// the query string would ride along into logs, so Do discards the message.
func TestDoTransportErrorHidesURL(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close() // nothing is listening now

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url+"?apikey=super-secret", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := Do(&http.Client{Timeout: time.Second}, req, nil)
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("Do to a closed server = nil, want an error")
	}
	if !errors.Is(err, ErrTransport) {
		t.Errorf("Do = %v, want ErrTransport", err)
	}
	if strings.Contains(err.Error(), "super-secret") {
		t.Errorf("transport error leaked the credential: %v", err)
	}
}

func TestDoNilClientUsesDefault(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := Do(nil, req, nil)
	if err != nil {
		t.Fatalf("Do = %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("status = %d, want 204", resp.StatusCode)
	}
}

func TestStatusErrorMessage(t *testing.T) {
	t.Parallel()
	e := &StatusError{StatusCode: 503, Method: http.MethodGet, URL: "https://api.example/x", Body: "down"}
	want := "httpx: 503 for GET https://api.example/x: down"
	if got := e.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestNewTransportSetsTimeouts(t *testing.T) {
	t.Parallel()
	tr := NewTransport()
	if tr.TLSHandshakeTimeout == 0 || tr.IdleConnTimeout == 0 {
		t.Error("NewTransport left handshake or idle timeout unset")
	}
}
