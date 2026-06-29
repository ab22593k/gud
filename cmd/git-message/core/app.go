package core

import (
	"context"
	"fmt"

	"gud/internal/config"
	"gud/internal/config/mediator"
	"gud/internal/helixdb"
	"gud/internal/request"

	"github.com/spf13/cobra"
)

// AppContext bundles resolved application configuration with the request client
// and optional HelixDB connection.
type AppContext struct {
	cfg     config.Config
	client  *request.Client
	helixDB *helixdb.DB
}

// NewAppContext loads and merges configuration from all sources (CLI flags,
// environment variables, config files) and returns an AppContext with the
// resolved config. The request client is NOT created here — call InitClient
// separately. HelixDB is NOT initialized here — call InitHelixDB separately.
func NewAppContext(cmd *cobra.Command) (*AppContext, error) {
	cliCfg := configFromCmd(cmd)

	m, err := mediator.New()
	if err != nil {
		return nil, fmt.Errorf("config mediator: %w", err)
	}

	cfg, err := m.Load(cliCfg)
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}

	if err := requireProfile(string(cfg.Profile)); err != nil {
		return nil, err
	}

	return &AppContext{cfg: cfg}, nil
}

// Config returns the resolved application configuration.
func (a *AppContext) Config() config.Config {
	return a.cfg
}

// Client returns the request client, or nil if InitClient has not been called.
func (a *AppContext) Client() *request.Client {
	return a.client
}

// HelixDB returns the HelixDB connection, or nil if not initialized.
func (a *AppContext) HelixDB() *helixdb.DB {
	return a.helixDB
}

// InitClient creates the request client from the resolved configuration.
// Must be called at most once with a context that supports cancellation.
func (a *AppContext) InitClient(ctx context.Context) error {
	client, err := request.NewClient(ctx, request.ClientConfig{
		APIKey:      a.cfg.APIKey,
		Model:       a.cfg.Model,
		Temperature: a.cfg.Temperature,
	})
	if err != nil {
		return fmt.Errorf("failed to create request client: %w", err)
	}
	a.client = client

	return nil
}

// InitHelixDB creates the HelixDB connection from the resolved configuration.
// This is safe to call even when HelixDB is not enabled — it returns nil for
// the DB and error is nil.
func (a *AppContext) InitHelixDB(ctx context.Context) error {
	if !a.cfg.HelixDBEnabled {
		return nil
	}

	db := helixdb.NewDB(helixdb.Options{
		BaseURL: a.cfg.HelixDBURL,
		Enabled: a.cfg.HelixDBEnabled,
	})

	if !db.Enabled() {
		return nil
	}

	// Check availability and run schema migration.
	if db.IsAvailable(ctx) {
		if err := db.EnsureSchema(ctx); err != nil {
			return fmt.Errorf("helixdb schema: %w", err)
		}
		a.helixDB = db
	}

	return nil
}
