// Package config defines the domain entity for application configuration.
// It is a pure business concept with no format-specific tags or serialization
// concerns. Use the dto sub-package for JSON interchange.
package config

// DetailLevel controls the verbosity of generated commit messages.
type DetailLevel string

const (
	DetailMinimal  DetailLevel = "minimal"
	DetailStandard DetailLevel = "standard"
	DetailDetailed DetailLevel = "detailed"
)

// ProfileName identifies an AI agent profile used for commit message generation.
type ProfileName string

// Config is the domain entity representing the full application configuration.
// It contains no format-specific tags — it is the pure business concept.
// Zero values represent "not set" and are treated as undefined, allowing
// layered overrides (file → env → CLI flags).
type Config struct {
	DetailLevel    DetailLevel
	Profile        ProfileName
	Model          string
	Temperature    float64
	Hint           string
	History        int
	APIKey         string
	WrapLine       int
	HelixDBEnabled bool
	HelixDBURL     string
}

const (
	maxHistory = 50
	minWrap    = 40
	maxWrap    = 200
)

// Validate returns a copy of the config with normalized and clamped values.
// Invalid or out-of-range values are replaced with sensible defaults.
func (c Config) Validate() Config {
	switch c.DetailLevel {
	case DetailMinimal, DetailStandard, DetailDetailed:
	case "":
		c.DetailLevel = DetailStandard
	default:
		c.DetailLevel = DetailStandard
	}

	if c.History < 0 {
		c.History = 0
	} else if c.History > maxHistory {
		c.History = maxHistory
	}

	if c.WrapLine < minWrap {
		c.WrapLine = minWrap
	} else if c.WrapLine > maxWrap {
		c.WrapLine = maxWrap
	}

	return c
}

// Merge returns a new Config with non-zero fields from override applied on top.
// Zero-value fields in override are left as-is from the receiver. This allows
// layering: file config → env overrides → CLI flag overrides.
func (c Config) Merge(override Config) Config {
	merged := c

	if override.DetailLevel != "" {
		merged.DetailLevel = override.DetailLevel
	}
	if override.Profile != "" {
		merged.Profile = override.Profile
	}
	if override.Model != "" {
		merged.Model = override.Model
	}
	if override.Temperature != 0 {
		merged.Temperature = override.Temperature
	}
	if override.Hint != "" {
		merged.Hint = override.Hint
	}
	if override.History != 0 {
		merged.History = override.History
	}
	if override.APIKey != "" {
		merged.APIKey = override.APIKey
	}
	if override.WrapLine != 0 {
		merged.WrapLine = override.WrapLine
	}
	if override.HelixDBEnabled {
		merged.HelixDBEnabled = true
	}
	if override.HelixDBURL != "" {
		merged.HelixDBURL = override.HelixDBURL
	}

	return merged
}

// DefaultConfig returns a Config populated with sensible defaults.
func DefaultConfig() Config {
	return Config{
		DetailLevel: DetailStandard,
		WrapLine:    72,
	}
}
