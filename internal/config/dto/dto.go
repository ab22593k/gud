// Package dto converts the domain config entity to and from its JSON
// representation. It owns the wire format — key names, omitempty behaviour —
// so the config package stays free of serialization concerns.
package dto

import "gud/internal/config"

// Config is the JSON-serializable representation of application configuration.
// An empty string for a field means "not set" — the DTO does not encode
// defaults. History is a *int and NoAssistedBy a *bool so an explicit 0 or
// false survives JSON round-trips; nil means the key is absent and is omitted
// from output.
//
// Conversions carry pointer values across verbatim rather than copying them,
// which is safe because no layer writes through a pointer — see the ownership
// contract on config.Config.
//
// Issues is deliberately absent: issue numbers describe the invocation being
// generated, not durable configuration, so they are never persisted.
type Config struct {
	DetailLevel  string `json:"detail_level,omitempty"`
	Model        string `json:"model,omitempty"`
	Hint         string `json:"hint,omitempty"`
	History      *int   `json:"history,omitempty"`
	APIKey       string `json:"api_key,omitempty"`
	WrapLine     int    `json:"wrapline,omitempty"`
	NoAssistedBy *bool  `json:"no_assisted_by,omitempty"`
}

// ToEntity converts the DTO to a domain Config entity.
func (d Config) ToEntity() config.Config {
	return config.Config{
		DetailLevel:  config.DetailLevel(d.DetailLevel),
		Model:        d.Model,
		Hint:         d.Hint,
		History:      d.History,
		APIKey:       d.APIKey,
		WrapLine:     d.WrapLine,
		NoAssistedBy: d.NoAssistedBy,
	}
}

// FromEntity converts a domain Config entity into a DTO for serialization.
func FromEntity(c config.Config) Config {
	return Config{
		DetailLevel:  string(c.DetailLevel),
		Model:        c.Model,
		Hint:         c.Hint,
		History:      c.History,
		APIKey:       c.APIKey,
		WrapLine:     c.WrapLine,
		NoAssistedBy: c.NoAssistedBy,
	}
}

// ConfigDTO is an alias for Config, kept for backward compatibility.
type ConfigDTO = Config
