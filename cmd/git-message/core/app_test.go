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
	if err := app.InitHelixDB(context.Background()); err != nil {
		t.Fatalf("InitHelixDB should never fail, got: %v", err)
	}

	if db := app.HelixDB(); db != nil && (!db.Enabled() || !db.IsAvailable(context.Background())) {
		t.Error("HelixDB() must be nil or an enabled, available DB")
	}
}
