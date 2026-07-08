package mem

import (
	"context"
	"errors"
	"testing"
)

func TestNewDB_DefaultURL(t *testing.T) {
	db := NewDB(Options{})
	if db == nil {
		t.Fatal("expected non-nil DB")
	}
	if db.BaseURL() != "http://localhost:6969" {
		t.Errorf("expected default URL, got %q", db.BaseURL())
	}
}

func TestNewDB_CustomURL(t *testing.T) {
	db := NewDB(Options{BaseURL: "http://helix-cloud:6969"})
	if db.BaseURL() != "http://helix-cloud:6969" {
		t.Errorf("expected custom URL, got %q", db.BaseURL())
	}
}

func TestNewDB_WithAPIKey(t *testing.T) {
	db := NewDB(Options{BaseURL: "http://localhost:6969", APIKey: "hx_test_key"})
	if db.APIKey() != "hx_test_key" {
		t.Errorf("expected API key to be set, got %q", db.APIKey())
	}
}

func TestDB_IsAvailable_NoConnection(t *testing.T) {
	db := NewDB(Options{BaseURL: "http://localhost:16969"})
	// Should not panic; returns false gracefully
	available := db.IsAvailable(context.Background())
	if available {
		t.Log("expected unavailable, got available (may be false positive if HelixDB is running on 16969)")
	}
}

func TestErrHelixUnavailable(t *testing.T) {
	err := NewHelixUnavailableError(errors.New("connection refused"))
	if !errors.Is(err, ErrHelixUnavailable) {
		t.Errorf("expected ErrHelixUnavailable in error chain")
	}
	if err.Error() == "" {
		t.Errorf("expected non-empty error message")
	}
}

func TestHelixDBSentinelError(t *testing.T) {
	err := NewHelixUnavailableError(nil)
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if !errors.Is(err, ErrHelixUnavailable) {
		t.Errorf("expected ErrHelixUnavailable in error chain")
	}
}
