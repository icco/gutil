package etag

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-http-utils/headers"
)

// serve runs h behind the ETag middleware and returns the recorded response.
func serve(t *testing.T, weak bool, req *http.Request, h http.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	Handler(weak)(h).ServeHTTP(w, req)
	return w
}

func okHandler(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}
}

func TestHandlerSetsETag(t *testing.T) {
	t.Parallel()
	w := serve(t, false, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil), okHandler("hello"))

	etag := w.Header().Get(headers.ETag)
	if etag == "" {
		t.Fatal("no ETag set on a 200 with a body")
	}
	if !strings.HasPrefix(etag, "5-") {
		t.Errorf("ETag = %q, want it to start with the body length", etag)
	}
	if w.Body.String() != "hello" {
		t.Errorf("body = %q, want it passed through", w.Body.String())
	}
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestHandlerWeakPrefix(t *testing.T) {
	t.Parallel()
	w := serve(t, true, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil), okHandler("hello"))

	if got := w.Header().Get(headers.ETag); !strings.HasPrefix(got, "W/") {
		t.Errorf("ETag = %q, want a W/ prefix in weak mode", got)
	}
}

// The same body must always hash to the same tag, and a different body must
// not — otherwise conditional requests either never or always match.
func TestHandlerETagIsContentAddressed(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)

	a := serve(t, false, req, okHandler("hello")).Header().Get(headers.ETag)
	b := serve(t, false, req, okHandler("hello")).Header().Get(headers.ETag)
	c := serve(t, false, req, okHandler("goodbye")).Header().Get(headers.ETag)

	if a != b {
		t.Errorf("same body produced different tags: %q vs %q", a, b)
	}
	if a == c {
		t.Errorf("different bodies produced the same tag: %q", a)
	}
}

func TestHandlerReturns304OnMatchingIfNoneMatch(t *testing.T) {
	t.Parallel()
	first := serve(t, false, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil), okHandler("hello"))
	etag := first.Header().Get(headers.ETag)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	req.Header.Set(headers.IfNoneMatch, etag)
	w := serve(t, false, req, okHandler("hello"))

	if w.Code != http.StatusNotModified {
		t.Errorf("status = %d, want 304 for a matching If-None-Match", w.Code)
	}
	if w.Body.Len() != 0 {
		t.Errorf("body = %q, want empty on a 304", w.Body.String())
	}
}

func TestHandlerSendsBodyOnStaleIfNoneMatch(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	req.Header.Set(headers.IfNoneMatch, `"some-other-tag"`)
	w := serve(t, false, req, okHandler("hello"))

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 for a stale If-None-Match", w.Code)
	}
	if w.Body.String() != "hello" {
		t.Errorf("body = %q, want the full body", w.Body.String())
	}
}

// An ETag the handler set itself must be left alone.
func TestHandlerRespectsExistingETag(t *testing.T) {
	t.Parallel()
	w := serve(t, false, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil),
		func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set(headers.ETag, `"mine"`)
			_, _ = w.Write([]byte("hello"))
		})

	if got := w.Header().Get(headers.ETag); got != `"mine"` {
		t.Errorf("ETag = %q, want the handler's own tag preserved", got)
	}
	if w.Body.String() != "hello" {
		t.Errorf("body = %q", w.Body.String())
	}
}

// Only 2xx responses are cacheable by content hash.
func TestHandlerSkipsNon2xx(t *testing.T) {
	t.Parallel()
	for _, status := range []int{http.StatusNotFound, http.StatusInternalServerError, http.StatusMovedPermanently} {
		w := serve(t, false, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil),
			func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
				_, _ = w.Write([]byte("nope"))
			})

		if got := w.Header().Get(headers.ETag); got != "" {
			t.Errorf("status %d got ETag %q, want none", status, got)
		}
		if w.Code != status {
			t.Errorf("status = %d, want %d passed through", w.Code, status)
		}
	}
}

func TestHandlerSkipsEmptyBody(t *testing.T) {
	t.Parallel()
	w := serve(t, false, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil),
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

	if got := w.Header().Get(headers.ETag); got != "" {
		t.Errorf("ETag = %q, want none for an empty body", got)
	}
}

// 204 carries no body by definition, so there is nothing to hash.
func TestHandlerSkipsNoContent(t *testing.T) {
	t.Parallel()
	w := serve(t, false, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil),
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})

	if got := w.Header().Get(headers.ETag); got != "" {
		t.Errorf("ETag = %q, want none on a 204", got)
	}
	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", w.Code)
	}
}

// A handler that writes without calling WriteHeader is an implicit 200.
func TestHandlerImplicit200(t *testing.T) {
	t.Parallel()
	w := serve(t, false, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil), okHandler("hi"))

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want an implicit 200", w.Code)
	}
	if w.Header().Get(headers.ETag) == "" {
		t.Error("no ETag on an implicit 200")
	}
}

func TestHandlerLengthIsBytesNotRunes(t *testing.T) {
	t.Parallel()
	// Four bytes, one rune.
	w := serve(t, false, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil), okHandler("🐶"))

	if got := w.Header().Get(headers.ETag); !strings.HasPrefix(got, "4-") {
		t.Errorf("ETag = %q, want a byte length prefix of 4", got)
	}
}

// Multiple Writes must hash as one body, not just the last chunk.
func TestHandlerAccumulatesWrites(t *testing.T) {
	t.Parallel()
	chunked := serve(t, false, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil),
		func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("hel"))
			_, _ = w.Write([]byte("lo"))
		})
	whole := serve(t, false, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil), okHandler("hello"))

	if chunked.Body.String() != "hello" {
		t.Errorf("body = %q, want hello", chunked.Body.String())
	}
	if got, want := chunked.Header().Get(headers.ETag), whole.Header().Get(headers.ETag); got != want {
		t.Errorf("chunked ETag = %q, want %q — writes must hash as one body", got, want)
	}
}

func TestVersionIsSet(t *testing.T) {
	t.Parallel()
	if Version == "" {
		t.Error("Version is empty")
	}
}
