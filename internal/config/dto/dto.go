// Package dto provides Data Transfer Objects for configuration serialization.
// This is the interface/infrastructure layer — DTOs include format-specific
// tags (JSON) and convert to/from the domain entity in the parent package.
package dto

import "gud/internal/config"

// ConfigDTO is the JSON-serializable representation of application configuration.
// It lives in the interface layer and carries format-specific tags.
// An empty string for a field means "not set" — the DTO does not encode defaults.
type ConfigDTO struct {
	DetailLevel string  `json:"detail_level,omitempty"`
	Profile     string  `json:"profile,omitempty"`
	Model       string  `json:"model,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
	Hint        string  `json:"hint,omitempty"`
	History     int     `json:"history,omitempty"`
	APIKey      string  `json:"api_key,omitempty"`
	ACP         string  `json:"acp,omitempty"`
	WrapLine    int     `json:"wrapline,omitempty"`
}

// ToEntity converts the DTO to a domain Config entity.
// Empty string fields map to zero-value domain types (zero-config merges cleanly).
func (d ConfigDTO) ToEntity() config.Config {
	return config.Config{
		DetailLevel: config.DetailLevel(d.DetailLevel),
		Profile:     config.ProfileName(d.Profile),
		Model:       d.Model,
		Temperature: d.Temperature,
		Hint:        d.Hint,
		History:     d.History,
		APIKey:      d.APIKey,
		ACP:         config.ACPProvider(d.ACP),
		WrapLine:    d.WrapLine,
	}
}

// FromEntity converts a domain Config entity into a DTO for serialization.
func FromEntity(c config.Config) ConfigDTO {
	return ConfigDTO{
		DetailLevel: string(c.DetailLevel),
		Profile:     string(c.Profile),
		Model:       c.Model,
		Temperature: c.Temperature,
		Hint:        c.Hint,
		History:     c.History,
		APIKey:      c.APIKey,
		ACP:         string(c.ACP),
		WrapLine:    c.WrapLine,
	}
}
