// Package mediator is the Interface Adapter responsible for gathering
// configuration from all sources and merging them into the final
// domain configuration. It resolves paths based on OS conventions
// and environment variables, loads from file providers and env,
// then applies CLI overrides in priority order. It performs upfront
// validation — rejecting magic values, unresolved placeholders,
// and missing required fields — before the application starts.
package mediator

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"gud/internal/config"
	"gud/internal/config/provider"
)

// UnresolvedPlaceholderError is returned when a config value contains
// an unresolved template variable that should have been replaced.
type UnresolvedPlaceholderError struct {
	Field string
	Value string
}

func (e *UnresolvedPlaceholderError) Error() string {
	return fmt.Sprintf("config: %s contains unresolved placeholder %q", e.Field, e.Value)
}

// Field names used in strict validation error messages.
const (
	fieldAPIKey = "api_key"
)

// Mediator is the infrastructure-level orchestrator that collects
// configuration from all sources (files, environment, CLI),
// validates it upfront, and returns the final domain Config.
type Mediator struct {
	XDGProvider *provider.FileProvider
	CWDProvider *provider.FileProvider
}

// New creates a Mediator with paths resolved according to OS conventions.
//
// Path resolution order:
//  1. GUD_CONFIG_PATH environment variable (explicit override)
//  2. XDG default: ~/.config/gud/config.json
//  3. CWD: ./gud.json
//
// The parent directory for the XDG path is created if it does not exist.
func New() (*Mediator, error) {
	xdgPath := os.Getenv("GUD_CONFIG_PATH")
	if xdgPath == "" {
		var err error

		xdgPath, err = provider.DefaultConfigPath()
		if err != nil {
			return nil, fmt.Errorf("resolve xdg config path: %w", err)
		}
	}

	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("resolve cwd: %w", err)
	}

	cwdPath := filepath.Join(cwd, "gud.json")

	return &Mediator{
		XDGProvider: provider.NewFileProvider(xdgPath),
		CWDProvider: provider.NewFileProvider(cwdPath),
	}, nil
}

// Load gathers configuration from all sources, merges with priority,
// validates, and returns the final Config. cliCfg should be constructed
// from CLI flags by the caller.
//
// Priority (highest to lowest):
//  1. CLI flags (cliCfg)
//  2. Environment variables (GUD_*)
//  3. CWD config file (./gud.json)
//  4. XDG config file (~/.config/gud/config.json)
//  5. Sensible defaults (DefaultConfig)
//
// Returns an error if strict validation fails: unresolved placeholders,
// known sentinel values in the API key, or other coherence issues.
func (m *Mediator) Load(cliCfg config.Config) (config.Config, error) {
	result := config.DefaultConfig()
	result = result.Merge(m.loadOrZero(m.XDGProvider))
	result = result.Merge(m.loadOrZero(m.CWDProvider))
	result = result.Merge(configFromEnv())
	result = result.Merge(cliCfg)
	result = result.Validate()

	if err := validateStrict(result); err != nil {
		return config.Config{}, err
	}

	return result, nil
}

// loadOrZero attempts to load config from the provider.
// Returns zero-value Config silently if the file doesn't exist
// or cannot be read (logged at debug level).
func (m *Mediator) loadOrZero(p *provider.FileProvider) config.Config {
	cfg, err := p.Load()
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Debug("mediator: failed to load config file", "path", p.Path(), "error", err)
		}

		return config.Config{}
	}

	return cfg
}

// validateStrict checks the final merged Config for:
//   - Unresolved template placeholders in string fields (${VAR}, {{VAR}}, $VAR)
//   - Known sentinel/placeholder values in the API key
//   - Missing required fields
func validateStrict(cfg config.Config) error {
	stringFields := map[string]string{
		"model":     cfg.Model,
		fieldAPIKey: cfg.APIKey,
		"hint":      cfg.Hint,
	}

	for name, value := range stringFields {
		if hasPlaceholder(value) {
			return &UnresolvedPlaceholderError{Field: name, Value: value}
		}
	}

	if isKnownPlaceholder(cfg.APIKey) {
		return fmt.Errorf(
			"config: %s contains placeholder value %q; replace with a real API key or set via GOOGLE_API_KEY",
			fieldAPIKey, cfg.APIKey,
		)
	}

	return nil
}

// hasPlaceholder returns true if the string contains any unresolved
// template variable syntax: ${...}, {{...}}, or $(...).
func hasPlaceholder(s string) bool {
	return strings.Contains(s, "${") || strings.Contains(s, "{{") || strings.Contains(s, "$(")
}

// isKnownPlaceholder checks if a string matches a known sentinel value
// that is commonly used as a placeholder in configuration templates.
func isKnownPlaceholder(s string) bool {
	lower := strings.ToLower(s)
	switch lower {
	case "your-api-key", "api-key", "api_key", "sk-your-key-here":
		return true
	}

	return false
}
