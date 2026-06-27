package core

import (
	"context"
	"fmt"

	"gud/internal/config"
	"gud/internal/config/mediator"
	"gud/internal/request"

	"github.com/spf13/cobra"
)

// AppContext bundles resolved application configuration with the request client.
// Create with NewAppContext, then call InitClient with a cancellable context.
type AppContext struct {
	cfg    config.Config
	client *request.Client
}

// NewAppContext loads and merges configuration from all sources (CLI flags,
// environment variables, config files) and returns an AppContext with the
// resolved config. The request client is NOT created here — call InitClient
// separately with a context that carries timeouts/cancellation.
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
