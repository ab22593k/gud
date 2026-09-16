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

	t.Cleanup(app.CloseHelixDB)

	if err := app.InitHelixDB(context.Background()); err != nil {
		t.Fatalf("InitHelixDB should never fail, got: %v", err)
	}

	if db := app.HelixDB(); db != nil && (!db.Enabled() || !db.IsAvailable(context.Background())) {
		t.Error("HelixDB() must be nil or an enabled, available DB")
	}
}

// TestAppContext_CloseHelixDB verifies the close contract: closing with no DB
// is a no-op returning nothing, closing twice is safe, and after InitHelixDB
// plus CloseHelixDB, HelixDB() is nil.
func TestAppContext_CloseHelixDB(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		app      *AppContext
		withInit bool
	}{
		{name: "nil app", app: nil},
		{name: "no DB", app: &AppContext{}},
		{name: "init then close", app: &AppContext{cfg: config.Config{}}, withInit: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.withInit {
				if err := tt.app.InitHelixDB(context.Background()); err != nil {
					t.Fatalf("InitHelixDB()=%v, want nil", err)
				}
			}

			tt.app.CloseHelixDB()

			if tt.app != nil && tt.app.HelixDB() != nil {
				t.Error("HelixDB()!=nil after CloseHelixDB, want cleared")
			}

			tt.app.CloseHelixDB()
		})
	}
}
