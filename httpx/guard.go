package httpx

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// DefaultAttempts is the total number of tries Guard.Run makes when Attempts is
// left at zero.
const DefaultAttempts = 3

// Guard runs an operation behind a Limiter, a Breaker, and bounded retries.
// Every field is optional: a zero Guard still retries DefaultAttempts times.
type Guard struct {
	// Limiter paces calls. Nil means unlimited.
	Limiter *Limiter
	// Breaker trips after repeated failures. Nil means never trip.
	Breaker *Breaker
	// Attempts is the total number of tries, including the first. Zero means
	// DefaultAttempts; a value below 1 is treated as 1.
	Attempts int
	// Backoff returns how long to sleep before retrying. attempt is 1-based and
	// counts the try that just failed. Nil means linear: 1s, 2s, 3s...
	Backoff func(attempt int) time.Duration
	// OnRetry, when set, is called after each failed attempt that will be
	// retried. It exists so callers can log without this package taking a
	// logging dependency.
	OnRetry func(attempt int, err error)
}

// permanentError marks an error that must not be retried.
type permanentError struct{ err error }

func (p *permanentError) Error() string { return p.err.Error() }
func (p *permanentError) Unwrap() error { return p.err }

// Permanent wraps err so Guard.Run returns it immediately instead of retrying.
// Use it for outcomes a retry cannot change: a 404, a malformed id, a rejected
// credential. The wrapper is transparent to errors.Is and errors.As, so callers
// still match the underlying sentinel.
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return &permanentError{err: err}
}

// IsPermanent reports whether err was marked with Permanent.
func IsPermanent(err error) bool {
	var p *permanentError
	return errors.As(err, &p)
}

// Run calls fn until it succeeds, exhausts attempts, hits a Permanent error, or
// ctx is done.
//
// The breaker is consulted before every attempt and returns ErrCircuitOpen
// without calling fn. Each attempt's outcome is recorded on the breaker, except
// that a Permanent error counts as neither success nor failure: a missing
// record says nothing about the health of the service that reported it.
func (g *Guard) Run(ctx context.Context, fn func(ctx context.Context) error) error {
	attempts := g.Attempts
	if attempts == 0 {
		attempts = DefaultAttempts
	}
	if attempts < 1 {
		attempts = 1
	}

	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		if g.Breaker != nil && !g.Breaker.Allow() {
			return ErrCircuitOpen
		}
		if g.Limiter != nil {
			if err := g.Limiter.Wait(ctx); err != nil {
				return err
			}
		}

		err := fn(ctx)
		if err == nil {
			if g.Breaker != nil {
				g.Breaker.RecordSuccess()
			}
			return nil
		}
		lastErr = err

		// A permanent failure is the service answering correctly, so it must not
		// count against the breaker or be retried.
		if IsPermanent(err) {
			return errors.Unwrap(err)
		}
		if g.Breaker != nil {
			g.Breaker.RecordFailure()
		}
		// Callers bound this work with a deadline; sleeping past it would
		// overrun the budget to reach a request that cannot succeed anyway.
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if attempt == attempts {
			break
		}

		if g.OnRetry != nil {
			g.OnRetry(attempt, err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(g.backoff(attempt)):
		}
	}
	return lastErr
}

func (g *Guard) backoff(attempt int) time.Duration {
	if g.Backoff != nil {
		return g.Backoff(attempt)
	}
	return time.Duration(attempt) * time.Second
}

// ErrTransport replaces a transport-layer error rather than wrapping it.
//
// Go's net/http embeds the full request URL in transport errors, so an error
// from a request whose credentials ride in the query string will carry the key
// into every log line and error report that touches it. Do discards the
// original message for this one instead.
var ErrTransport = errors.New("httpx: transport error")

// Do sends req through client, first applying sign (which may attach
// credentials). It returns ErrTransport in place of any transport failure, so
// a credential in req.URL cannot leak into the returned error.
//
// Do is a single attempt and consults neither the Limiter nor the Breaker; call
// it from inside Run to get those.
func Do(client *http.Client, req *http.Request, sign func(*http.Request)) (*http.Response, error) {
	if client == nil {
		client = http.DefaultClient
	}
	if sign != nil {
		sign(req)
	}
	// gosec flags the caller-supplied URL as an SSRF taint. That is inherent to a
	// generic HTTP helper: Do exists to send a request the caller already built,
	// and validating its destination is the caller's job, not this package's.
	resp, err := client.Do(req) //nolint:gosec // G704: destination is the caller's to validate
	if err != nil {
		return nil, ErrTransport
	}
	return resp, nil
}

// NewTransport returns an http.Transport with connection-pool and handshake
// timeouts suited to a chatty API client. The stdlib defaults leave
// TLSHandshakeTimeout and IdleConnTimeout generous enough that a wedged host
// holds a connection far longer than a request budget allows.
func NewTransport() *http.Transport {
	return &http.Transport{
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
	}
}

// StatusError is an unexpected HTTP status from an API. It carries the request
// URL, so build it from a URL that has no credentials in it.
type StatusError struct {
	StatusCode int
	Method     string
	URL        string
	Body       string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("httpx: %d for %s %s: %s", e.StatusCode, e.Method, e.URL, e.Body)
}
