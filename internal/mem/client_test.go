package mem

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestNewDB_DefaultURL(t *testing.T) {
	db := NewDB(Options{})
	if db == nil {
		t.Fatal("expected non-nil DB")
	}

	if db.BaseURL() != DefaultBaseURL {
		t.Errorf("expected default URL, got %q", db.BaseURL())
	}
}

func TestNewDB_CustomURL(t *testing.T) {
	db := NewDB(Options{BaseURL: "http://helix-cloud:3223"})
	if db.BaseURL() != "http://helix-cloud:3223" {
		t.Errorf("expected custom URL, got %q", db.BaseURL())
	}
}

func TestNewDB_WithAPIKey(t *testing.T) {
	db := NewDB(Options{BaseURL: DefaultBaseURL, APIKey: "hx_test_key"})
	if db.APIKey() != "hx_test_key" {
		t.Errorf("expected API key to be set, got %q", db.APIKey())
	}
}

func TestDB_IsAvailable_NoConnection(t *testing.T) {
	db := NewDB(Options{BaseURL: DefaultBaseURL})
	// Should not panic; returns false gracefully
	available := db.IsAvailable(context.Background())
	if available {
		t.Log("expected unavailable, got available (may be false positive if HelixDB is running on 2232)")
	}
}

// countHealthRequests starts a fake HelixDB health endpoint that counts
// requests and returns the given status code.
func countHealthRequests(t *testing.T, status int) (*DB, *atomic.Int64) {
	t.Helper()

	var hits atomic.Int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)

	db := NewDB(Options{BaseURL: srv.URL, Enabled: true})
	if !db.Enabled() {
		t.Fatal("expected DB to be enabled")
	}

	return db, &hits
}

// TestDB_IsAvailable_CachesUp verifies that repeated IsAvailable calls issue
// exactly one health request when the server is up, returning true each time.
func TestDB_IsAvailable_CachesUp(t *testing.T) {
	db, hits := countHealthRequests(t, http.StatusOK)

	for range 3 {
		if !db.IsAvailable(context.Background()) {
			t.Fatal("expected IsAvailable to be true")
		}
	}

	if got := hits.Load(); got != 1 {
		t.Errorf("expected 1 health request, got %d", got)
	}
}

// TestDB_IsAvailable_CachesDown verifies that a server reporting non-200 is
// probed only once and stays unavailable on subsequent calls.
func TestDB_IsAvailable_CachesDown(t *testing.T) {
	db, hits := countHealthRequests(t, http.StatusInternalServerError)

	for range 3 {
		if db.IsAvailable(context.Background()) {
			t.Fatal("expected IsAvailable to be false")
		}
	}

	if got := hits.Load(); got != 1 {
		t.Errorf("expected 1 health request, got %d", got)
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
