package core

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestShowProgress_ReturnsValue(t *testing.T) {
	t.Parallel()

	msg := "Loading..."
	expected := "hello world"

	got, err := showProgress(t.Context(), io.Discard, msg, func() (string, error) {
		return expected, nil
	})
	if err != nil {
		t.Errorf("showProgress() returned error: %v", err)
	}

	if got != expected {
		t.Errorf("showProgress() = %q, want %q", got, expected)
	}
}

func TestShowProgress_ReturnsError(t *testing.T) {
	t.Parallel()

	expectedErr := errors.New("something went wrong")

	_, err := showProgress(t.Context(), io.Discard, "Working...", func() (string, error) {
		return "", expectedErr
	})
	if err == nil {
		t.Fatal("showProgress() expected error, got nil")
	}

	if !errors.Is(err, expectedErr) {
		t.Errorf("showProgress() error = %v, want %v", err, expectedErr)
	}
}

func TestShowProgress_WithIntType(t *testing.T) {
	t.Parallel()

	got, err := showProgress(t.Context(), io.Discard, "Counting...", func() (int, error) {
		return 42, nil
	})
	if err != nil {
		t.Errorf("showProgress() returned error: %v", err)
	}

	if got != 42 {
		t.Errorf("showProgress() = %d, want %d", got, 42)
	}
}

func TestShowProgress_CompletesImmediately(t *testing.T) {
	t.Parallel()

	got, err := showProgress(t.Context(), io.Discard, "Fast...", func() (string, error) {
		return testDoneStr, nil
	})
	if err != nil {
		t.Errorf("showProgress() returned error: %v", err)
	}

	if got != testDoneStr {
		t.Errorf("showProgress() = %q, want %q", got, testDoneStr)
	}
}

func TestShowProgress_WritesToCommandStderr(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	msg := "test message"

	_, err := showProgress(t.Context(), &buf, msg, func() (string, error) {
		return "ok", nil
	})
	if err != nil {
		t.Errorf("showProgress() returned error: %v", err)
	}

	captured := buf.String()
	if captured == "" {
		t.Error("showProgress() did not write anything to the command stderr writer")
	}

	if !strings.Contains(captured, msg) {
		t.Errorf("showProgress() stderr output = %q, want it to contain %q", captured, msg)
	}

	if !strings.Contains(captured, "✓") {
		t.Errorf("showProgress() stderr output = %q, want it to contain checkmark", captured)
	}
}

// TestShowProgress_QuietDiscardWritesNothing guards the --quiet path: a
// caller passing io.Discard must silence the spinner without errors.
func TestShowProgress_QuietDiscardWritesNothing(t *testing.T) {
	t.Parallel()

	got, err := showProgress(t.Context(), io.Discard, "quiet...", func() (string, error) {
		return "ok", nil
	})
	if err != nil {
		t.Errorf("showProgress() returned error: %v", err)
	}

	if got != "ok" {
		t.Errorf("showProgress() = %q, want %q", got, "ok")
	}
}
