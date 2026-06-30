package core

import (
	"context"
	"fmt"
	"log/slog"

	"gud/internal/config"
	"gud/internal/config/mediator"
	"gud/internal/helixdb"
	"gud/internal/request"

	"github.com/spf13/cobra"
)

// ConfigGetter allows read-only access to resolved configuration.
type ConfigGetter interface {
	Config() config.Config
}

// AppContext bundles resolved application configuration with the request client
// and optional HelixDB connection.
type AppContext struct {
	cfg          config.Config
	client       *request.Client
	helixDB      *helixdb.DB
	containerMgr *helixdb.ContainerManager
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

	return &AppContext{
		cfg: cfg,
		containerMgr: helixdb.NewContainerManager(
			cfg.HelixDBContainerName,
			extractPort(cfg.HelixDBURL),
		),
	}, nil
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

// ContainerManager returns the Docker container manager.
func (a *AppContext) ContainerManager() *helixdb.ContainerManager {
	return a.containerMgr
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
// If auto-manage is enabled and the container is not running, it starts one
// automatically. This is safe to call even when HelixDB is not enabled — it
// returns nil for the DB and error is nil.
func (a *AppContext) InitHelixDB(ctx context.Context) error {
	if !a.cfg.HelixDBEnabled {
		return nil
	}

	// Auto-manage: ensure the Docker container is running.
	if a.cfg.HelixDBAutoManage {
		url, err := a.containerMgr.EnsureRunning(ctx)
		if err != nil {
			slog.Debug("helixdb: auto-manage failed, proceeding without",
				"error", err)
			// Don't return error — degraded mode.
			return nil
		}
		// Update URL to the auto-managed container.
		a.cfg.HelixDBURL = url
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

// extractPort extracts the port from a URL like "http://localhost:6969".
func extractPort(url string) string {
	if url == "" {
		return "6969"
	}
	for i := len(url) - 1; i >= 0; i-- {
		if url[i] == ':' {
			return url[i+1:]
		}
	}
	return "6969"
}

// StopHelixDB stops the managed container if it was started by this session.
func (a *AppContext) StopHelixDB(ctx context.Context) {
	if a.containerMgr.StartedByUs() {
		_ = a.containerMgr.Stop(ctx)
	}
}
