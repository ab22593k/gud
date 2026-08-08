package dto

import "gud/internal/config"

// ConfigDTO is the JSON-serializable representation of application configuration.
// An empty string for a field means "not set" — the DTO does not encode defaults.
// History is a *int so an explicit "history": 0 (disable) survives JSON
// round-trips; nil means the key is absent and is omitted from output.
type ConfigDTO struct {
	DetailLevel    string `json:"detail_level,omitempty"`
	Profile        string `json:"profile,omitempty"`
	Model          string `json:"model,omitempty"`
	Hint           string `json:"hint,omitempty"`
	History        *int   `json:"history,omitempty"`
	APIKey         string `json:"api_key,omitempty"`
	WrapLine       int    `json:"wrapline,omitempty"`
	EmbeddingModel string `json:"embedding_model,omitempty"`
}

// ToEntity converts the DTO to a domain Config entity.
func (d ConfigDTO) ToEntity() config.Config {
	return config.Config{
		DetailLevel: config.DetailLevel(d.DetailLevel),
		Profile:     config.ProfileName(d.Profile),
		Model:       d.Model,

		Hint:           d.Hint,
		History:        d.History,
		APIKey:         d.APIKey,
		WrapLine:       d.WrapLine,
		EmbeddingModel: d.EmbeddingModel,
	}
}

// FromEntity converts a domain Config entity into a DTO for serialization.
func FromEntity(c config.Config) ConfigDTO {
	return ConfigDTO{
		DetailLevel: string(c.DetailLevel),
		Profile:     string(c.Profile),
		Model:       c.Model,

		Hint:           c.Hint,
		History:        c.History,
		APIKey:         c.APIKey,
		WrapLine:       c.WrapLine,
		EmbeddingModel: c.EmbeddingModel,
	}
}
