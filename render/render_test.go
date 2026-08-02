package render

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// observedLogger returns a logger plus the sink recording what it wrote.
func observedLogger() (*zap.SugaredLogger, *observer.ObservedLogs) {
	core, logs := observer.New(zap.ErrorLevel)
	return zap.New(core).Sugar(), logs
}

func TestJSONWritesBody(t *testing.T) {
	t.Parallel()
	log, logs := observedLogger()
	w := httptest.NewRecorder()

	JSON(log, w, 200, map[string]string{"hello": "world"})

	body := w.Body.String()
	if !strings.Contains(body, `"hello"`) || !strings.Contains(body, `"world"`) {
		t.Errorf("body = %q, want the marshalled map", body)
	}
	if logs.Len() != 0 {
		t.Errorf("logged %d errors on a successful write: %v", logs.Len(), logs.All())
	}
}

// The options set IndentJSON, so output should be pretty-printed rather than
// compact.
func TestJSONIsIndented(t *testing.T) {
	t.Parallel()
	log, _ := observedLogger()
	w := httptest.NewRecorder()

	JSON(log, w, 200, map[string]any{"a": 1})

	if !strings.Contains(w.Body.String(), "\n") {
		t.Errorf("body = %q, want indented output", w.Body.String())
	}
}

// A value json cannot marshal must be logged, not silently dropped.
func TestJSONLogsMarshalFailure(t *testing.T) {
	t.Parallel()
	log, logs := observedLogger()
	w := httptest.NewRecorder()

	JSON(log, w, 200, make(chan int)) // channels are unmarshallable

	if logs.Len() == 0 {
		t.Fatal("an unmarshallable value produced no error log")
	}
	if got := logs.All()[0].Message; !strings.Contains(got, "could not write response") {
		t.Errorf("log message = %q", got)
	}
}

// A writer that refuses everything must also be logged rather than ignored.
func TestJSONLogsWriteFailure(t *testing.T) {
	t.Parallel()
	log, logs := observedLogger()

	JSON(log, failingWriter{}, 200, map[string]string{"a": "b"})

	if logs.Len() == 0 {
		t.Fatal("a failing writer produced no error log")
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("nope") }
