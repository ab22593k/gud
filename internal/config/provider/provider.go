// Package provider loads and persists configuration from/to JSON files.
// It is an infrastructure adapter that bridges the domain entity and DTO
// with the filesystem.
package provider

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"gud/internal/config"
	"gud/internal/config/dto"
)

// FileProvider reads and writes configuration from a JSON file.
type FileProvider struct {
	path string
}

// NewFileProvider creates a FileProvider backed by the given file path.
func NewFileProvider(path string) *FileProvider {
	return &FileProvider{path: path}
}

// Path returns the file path this provider reads and writes.
func (p *FileProvider) Path() string {
	return p.path
}

// DefaultConfigPath returns the default config file path
// (~/.config/gud/config.json), creating the parent directory if needed.
func DefaultConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}

	dir := filepath.Join(home, ".config", "gud")
	if err := os.MkdirAll(dir, 0750); err != nil {
		return "", fmt.Errorf("create config dir: %w", err)
	}

	return filepath.Join(dir, "config.json"), nil
}

// Load reads configuration from the JSON file.
// Returns config.Config (zero value) and the underlying error if the file
// cannot be read or parsed. Callers should check os.IsNotExist to distinguish
// "no config file yet" from other errors.
func (p *FileProvider) Load() (config.Config, error) {
	data, err := os.ReadFile(p.path)
	if err != nil {
		return config.Config{}, err
	}

	var cfgDTO dto.ConfigDTO
	if err := json.Unmarshal(data, &cfgDTO); err != nil {
		return config.Config{}, fmt.Errorf("parse config: %w", err)
	}

	return cfgDTO.ToEntity(), nil
}

// Save writes the configuration to the JSON file.
func (p *FileProvider) Save(cfg config.Config) error {
	cfgDTO := dto.FromEntity(cfg)

	//nolint:gosec // Config files intentionally store API keys for persistence.
	data, err := json.MarshalIndent(cfgDTO, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(p.path, data, 0600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}
