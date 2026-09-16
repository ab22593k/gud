package core

import (
	"context"
	"testing"

	"gud/internal/config"
)

// TestInitHelixDB_NeverFails verifies the embedded degraded-mode contract:
// InitHelixDB always returns nil, leaving HelixDB() nil when the embedded
// runtime is unavailable or holding an enabled DB when it opened.
func TestInitHelixDB_NeverFails(t *testing.T) {
	t.Parallel()

	app := &AppContext{cfg: config.Config{}}

	t.Cleanup(func() { _ = app.Close() })

	if err := app.InitHelixDB(context.Background()); err != nil {
		t.Fatalf("InitHelixDB should never fail, got: %v", err)
	}

	if db := app.HelixDB(); db != nil && (!db.Enabled() || !db.IsAvailable(context.Background())) {
		t.Error("HelixDB() must be nil or an enabled, available DB")
	}
}

// TestAppContext_Close verifies the close contract: nil-safe, no-DB-safe,
// and idempotent with HelixDB() cleared after the first call.
func TestAppContext_Close(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		app  *AppContext
	}{
		{name: "nil app", app: nil},
		{name: "no DB", app: &AppContext{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if err := tt.app.Close(); err != nil {
				t.Errorf("Close()=%v, want nil", err)
			}

			if err := tt.app.Close(); err != nil {
				t.Errorf("second Close()=%v, want nil idempotent", err)
			}
		})
	}

	app := &AppContext{cfg: config.Config{}}

	t.Cleanup(func() { _ = app.Close() })

	if err := app.InitHelixDB(context.Background()); err != nil {
		t.Fatalf("InitHelixDB()=%v, want nil", err)
	}

	if err := app.Close(); err != nil {
		t.Fatalf("Close()=%v, want nil", err)
	}

	if app.HelixDB() != nil {
		t.Error("HelixDB()!=nil after Close, want cleared")
	}

	if err := app.Close(); err != nil {
		t.Errorf("second Close()=%v, want nil idempotent", err)
	}
}
