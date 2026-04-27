package logging

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
)

// TestFromContextFallbackIsCached verifies that FromContext returns the
// same fallback logger instance across multiple calls when the context
// has no logger attached. Re-allocating on every miss was the source of
// repeated startup log spam in production.
func TestFromContextFallbackIsCached(t *testing.T) {
	first := FromContext(context.Background())
	second := FromContext(context.Background())
	third := FromContext(nil) //nolint:staticcheck // exercising the nil branch

	if first == nil {
		t.Fatal("FromContext returned nil")
	}
	if first != second || second != third {
		t.Errorf("FromContext returned different instances: %p %p %p", first, second, third)
	}
}

// TestNewLoggerNoSyncDebugLine verifies that constructing a logger does
// not emit the historical "could not sync logger" debug line. We capture
// os.Stderr (which the production zap config writes to) for the duration
// of the call and assert the line is absent.
func TestNewLoggerNoSyncDebugLine(t *testing.T) {
	out := captureStderr(t, func() {
		log, err := NewLogger("test-no-sync")
		if err != nil {
			t.Fatalf("NewLogger: %v", err)
		}
		if log == nil {
			t.Fatal("nil logger")
		}
	})

	if strings.Contains(out, "could not sync logger") {
		t.Errorf("NewLogger emitted spurious sync debug line: %q", out)
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("NewLogger wrote unexpected output to stderr at construction: %q", out)
	}
}

// captureStderr redirects os.Stderr through an os.Pipe for the duration
// of fn, then returns whatever was written. zap's "stderr" sink resolves
// os.Stderr at Build time, so swapping it before calling fn is enough to
// intercept what NewLogger writes during construction.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	orig := os.Stderr
	os.Stderr = w

	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()

	defer func() {
		os.Stderr = orig
	}()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("close pipe: %v", err)
	}
	return <-done
}
