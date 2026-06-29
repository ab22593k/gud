package dto

import "gud/internal/config"

// ConfigDTO is the JSON-serializable representation of application configuration.
// An empty string for a field means "not set" — the DTO does not encode defaults.
type ConfigDTO struct {
	DetailLevel          string  `json:"detail_level,omitempty"`
	Profile              string  `json:"profile,omitempty"`
	Model                string  `json:"model,omitempty"`
	Temperature          float64 `json:"temperature,omitempty"`
	Hint                 string  `json:"hint,omitempty"`
	History              int     `json:"history,omitempty"`
	APIKey               string  `json:"api_key,omitempty"`
	WrapLine             int     `json:"wrapline,omitempty"`
	HelixDBEnabled       bool    `json:"helixdb_enabled,omitempty"`
	HelixDBURL           string  `json:"helixdb_url,omitempty"`
	HelixDBAutoManage    bool    `json:"helixdb_auto_manage,omitempty"`
	HelixDBContainerName string  `json:"helixdb_container_name,omitempty"`
}

// ToEntity converts the DTO to a domain Config entity.
func (d ConfigDTO) ToEntity() config.Config {
	return config.Config{
		DetailLevel:          config.DetailLevel(d.DetailLevel),
		Profile:              config.ProfileName(d.Profile),
		Model:                d.Model,
		Temperature:          d.Temperature,
		Hint:                 d.Hint,
		History:              d.History,
		APIKey:               d.APIKey,
		WrapLine:             d.WrapLine,
		HelixDBEnabled:       d.HelixDBEnabled,
		HelixDBURL:           d.HelixDBURL,
		HelixDBAutoManage:    d.HelixDBAutoManage,
		HelixDBContainerName: d.HelixDBContainerName,
	}
}

// FromEntity converts a domain Config entity into a DTO for serialization.
func FromEntity(c config.Config) ConfigDTO {
	return ConfigDTO{
		DetailLevel:          string(c.DetailLevel),
		Profile:              string(c.Profile),
		Model:                c.Model,
		Temperature:          c.Temperature,
		Hint:                 c.Hint,
		History:              c.History,
		APIKey:               c.APIKey,
		WrapLine:             c.WrapLine,
		HelixDBEnabled:       c.HelixDBEnabled,
		HelixDBURL:           c.HelixDBURL,
		HelixDBAutoManage:    c.HelixDBAutoManage,
		HelixDBContainerName: c.HelixDBContainerName,
	}
}
