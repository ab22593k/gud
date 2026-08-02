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
//
// NOTE: Temperature, top_p, and top_k have been deprecated by Google for
// Gemini 3.6+ models and are no longer sent to the API.
type Config struct {
	DetailLevel    DetailLevel
	Profile        ProfileName
	Model          string
	EmbeddingModel string
	Hint           string
	History        int
	APIKey         string
	WrapLine       int
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
//
// CAUTION: Zero is an ambiguous signal — it means "not set" rather than
// "set to zero". This means you cannot explicitly set History=0
// or WrapLine=0 via CLI or env overrides; they will be silently
// ignored. CLI flag defaults should avoid zero for any field that has a
// meaningful zero value.
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
	if override.EmbeddingModel != "" {
		merged.EmbeddingModel = override.EmbeddingModel
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

	return merged
}

// DefaultConfig returns a Config populated with sensible defaults.
func DefaultConfig() Config {
	return Config{
		DetailLevel: DetailStandard,
		WrapLine:    72,
	}
}
