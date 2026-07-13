package core

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

func TestShowProgress_ReturnsValue(t *testing.T) {
	t.Parallel()

	msg := "Loading..."
	expected := "hello world"

	got, err := showProgress(t.Context(), msg, func() (string, error) {
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

	_, err := showProgress(t.Context(), "Working...", func() (string, error) {
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

	got, err := showProgress(t.Context(), "Counting...", func() (int, error) {
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

	got, err := showProgress(t.Context(), "Fast...", func() (string, error) {
		return testDoneStr, nil
	})
	if err != nil {
		t.Errorf("showProgress() returned error: %v", err)
	}
	if got != testDoneStr {
		t.Errorf("showProgress() = %q, want %q", got, testDoneStr)
	}
}

func TestShowProgress_WritesToStderr(t *testing.T) {
	// Capture stderr by redirecting os.Stderr to a pipe.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}

	savedStderr := os.Stderr
	os.Stderr = w

	done := make(chan struct{})
	var captured string
	go func() {
		data, _ := io.ReadAll(r)
		captured = string(data)
		close(done)
	}()

	msg := "test message"
	_, fnErr := showProgress(t.Context(), msg, func() (string, error) {
		return "ok", nil
	})

	_ = w.Close()
	<-done
	os.Stderr = savedStderr

	if fnErr != nil {
		t.Errorf("showProgress() returned error: %v", fnErr)
	}

	if captured == "" {
		t.Error("showProgress() did not write anything to stderr")
	}
	if !strings.Contains(captured, msg) {
		t.Errorf("showProgress() stderr output = %q, want it to contain %q", captured, msg)
	}
	if !strings.Contains(captured, "✓") {
		t.Errorf("showProgress() stderr output = %q, want it to contain checkmark", captured)
	}
}
