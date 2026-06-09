// Package opencode implements an ACP (Agent Client Protocol) client for
// OpenCode.ai. It spawns the "opencode acp" subprocess and communicates
// via JSON-RPC 2.0 over stdio to generate commit messages.
package opencode

import (
	"context"
	"iter"
	"sync"

	"google.golang.org/adk/model"
)

const defaultModel = "deepseek-v4-flash"

// Config holds configuration for the OpenCode.ai ACP client.
type Config struct {
	APIKey   string // Passed as env var to the subprocess
	Model    string // Model name for identification
	BasePath string // Path to opencode binary (default: "opencode")
}

// Model implements model.LLM using OpenCode.ai's ACP protocol.
// It spawns `opencode acp` as a subprocess and communicates via JSON-RPC over stdio.
type Model struct {
	config Config
	client *acpClient
	mu     sync.Mutex
}

// NewModel creates a new OpenCode.ai ACP model client.
func NewModel(cfg Config) *Model {
	if cfg.BasePath == "" {
		cfg.BasePath = "opencode"
	}
	if cfg.Model == "" {
		cfg.Model = defaultModel
	}

	return &Model{config: cfg}
}

// Name returns the configured model name.
func (m *Model) Name() string { return m.config.Model }

// GenerateContent implements model.LLM by sending a prompt to the ACP agent.
func (m *Model) GenerateContent(
	ctx context.Context,
	req *model.LLMRequest,
	_ bool,
) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		client, err := m.getOrStartClient(ctx)
		if err != nil {
			yield(nil, err)

			return
		}
		resp, err := client.sendPrompt(ctx, req)
		if err != nil {
			yield(nil, err)

			return
		}
		yield(resp, nil)
	}
}

// getOrStartClient returns the existing ACP client if still alive, or starts a new one.
func (m *Model) getOrStartClient(ctx context.Context) (*acpClient, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.client != nil && m.client.alive() {
		return m.client, nil
	}

	if m.client != nil {
		m.client.close()
	}

	client, err := startACPClient(ctx, m.config)
	if err != nil {
		return nil, err
	}
	m.client = client

	return client, nil
}
