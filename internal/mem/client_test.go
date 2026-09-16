package mem

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestNewDB_Defaults(t *testing.T) {
	db := NewDB(Options{Enabled: false})
	if db == nil {
		t.Fatal("expected non-nil DB")
	}

	if db.DataDir() == "" {
		t.Error("expected non-empty DataDir")
	}

	if !strings.Contains(db.DataDir(), "gud") {
		t.Errorf("expected DataDir to contain gud, got %q", db.DataDir())
	}

	if db.Database() != DefaultDatabase {
		t.Errorf("expected database %q, got %q", DefaultDatabase, db.Database())
	}
}

func TestNewDB_CustomDir(t *testing.T) {
	db := NewDB(Options{DataDir: "/tmp/gud-test-helix", Database: "testdb", Enabled: false})
	if db.DataDir() != "/tmp/gud-test-helix" {
		t.Errorf("expected custom DataDir, got %q", db.DataDir())
	}

	if db.Database() != "testdb" {
		t.Errorf("expected custom database, got %q", db.Database())
	}
}

func TestDefaultDataDir_NonEmpty(t *testing.T) {
	dir := DefaultDataDir()
	if dir == "" {
		t.Fatal("expected non-empty default data dir")
	}

	if !strings.Contains(dir, "helixdb") {
		t.Errorf("expected default dir to contain helixdb, got %q", dir)
	}
}

func TestDB_DisabledIsUnavailable(t *testing.T) {
	db := NewDB(Options{Enabled: false})
	if db.Enabled() {
		t.Error("expected disabled DB")
	}

	if db.IsAvailable(context.Background()) {
		t.Error("expected IsAvailable false when disabled")
	}

	if err := db.Exec(context.Background(), BuildTrendsQuery("/repo"), nil); !errors.Is(err, ErrHelixUnavailable) {
		t.Errorf("expected ErrHelixUnavailable, got %v", err)
	}

	if err := db.Close(); err != nil {
		t.Errorf("expected nil Close on disabled DB, got %v", err)
	}
}

func TestNewDB_EmbeddedGracefulWithoutBindings(t *testing.T) {
	dir := t.TempDir()

	db := NewDB(Options{DataDir: dir, Database: "gud-test", Enabled: true})
	if db.DataDir() != dir {
		t.Errorf("expected DataDir %q, got %q", dir, db.DataDir())
	}

	// Without native bindings NewDB disables itself; with bindings it
	// stays enabled. Both states must degrade gracefully.
	if !db.Enabled() {
		if db.IsAvailable(context.Background()) {
			t.Error("expected IsAvailable false when embedded unavailable")
		}

		if err := db.Close(); err != nil {
			t.Errorf("expected nil Close when unavailable, got %v", err)
		}

		if got := db.UnavailableCause(); got == nil {
			t.Error("expected non-nil UnavailableCause when embedded open failed")
		}

		return
	}

	if !db.IsAvailable(context.Background()) {
		t.Error("expected IsAvailable true when embedded opened")
	}

	if err := db.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

func TestNewDB_DisabledHasNoCause(t *testing.T) {
	db := NewDB(Options{Enabled: false})

	if got := db.UnavailableCause(); got != nil {
		t.Errorf("expected nil UnavailableCause when explicitly disabled, got %v", got)
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
