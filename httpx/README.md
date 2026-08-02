# httpx

Client-side resilience primitives for talking to flaky third-party APIs: a sliding-window rate limiter, a circuit breaker, and a `Guard` that combines both with bounded retries.

Extracted from [icco/recommender](https://github.com/icco/recommender), where two API clients carried near-identical copies of this code that had already drifted apart.

## Installation

```
go get -u -d -v github.com/icco/gutil/httpx
```

## Documentation

API documentation can be found here: https://pkg.go.dev/github.com/icco/gutil/httpx

## Usage

`Guard.Run` wraps any operation, not just an `http.Request`, so it fits equally around a vendor SDK that does its own transport.

```go
package main

import (
  "context"
  "errors"
  "log/slog"
  "net/http"
  "time"

  "github.com/icco/gutil/httpx"
)

var errNotFound = errors.New("not found")

type Client struct {
  apiKey string
  http   *http.Client
  guard  *httpx.Guard
}

func NewClient(apiKey string) *Client {
  return &Client{
    apiKey: apiKey,
    http:   &http.Client{Timeout: 30 * time.Second, Transport: httpx.NewTransport()},
    guard: &httpx.Guard{
      Limiter: httpx.NewLimiter(40, 10*time.Second), // 40 requests per 10s
      Breaker: httpx.NewBreaker(5, time.Minute),     // open after 5 failures
      OnRetry: func(attempt int, err error) {
        slog.Warn("retrying", "attempt", attempt, "err", err)
      },
    },
  }
}

func (c *Client) Get(ctx context.Context, id string) (*Thing, error) {
  // safeURL carries no credentials, so it is safe to put in errors and logs.
  safeURL := "https://api.example.com/things/" + id

  var out *Thing
  err := c.guard.Run(ctx, func(ctx context.Context) error {
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, safeURL, nil)
    if err != nil {
      return httpx.Permanent(err)
    }

    // The api key is attached here, inside Do, and never reaches safeURL.
    resp, err := httpx.Do(c.http, req, func(r *http.Request) {
      q := r.URL.Query()
      q.Set("api_key", c.apiKey)
      r.URL.RawQuery = q.Encode()
    })
    if err != nil {
      return err
    }
    defer func() { _ = resp.Body.Close() }()

    if resp.StatusCode == http.StatusNotFound {
      return httpx.Permanent(errNotFound)
    }
    if resp.StatusCode != http.StatusOK {
      return &httpx.StatusError{StatusCode: resp.StatusCode, Method: req.Method, URL: safeURL}
    }
    return decode(resp.Body, &out)
  })
  return out, err
}
```

## Notes

- **`Permanent` marks what a retry cannot fix.** A 404, a malformed id, a rejected credential. `Run` returns it immediately and, importantly, does *not* count it against the breaker — a missing record says nothing about whether the service is healthy. The wrapper is transparent to `errors.Is`/`errors.As`, so callers still match their own sentinel.
- **`Do` hides transport errors on purpose.** Go's `net/http` embeds the full request URL in transport errors, so a credential in the query string ends up in every log line and error report that touches it. `Do` returns `ErrTransport` instead of the original message. Build your errors from a URL that has no credentials in it, and attach them in `Do`'s `sign` callback.
- **The breaker's half-open state admits one trial call.** A success closes it; a failure re-opens it for another full timeout.
- **A window is smoothing, not a quota.** If the API publishes a hard per-second cap, match it. If it publishes a daily quota instead, use the limiter to pace a batch and cap the batch size separately.
- Stdlib-only. This package takes no logger and prints nothing; use `Guard.OnRetry` if you want retries recorded.
