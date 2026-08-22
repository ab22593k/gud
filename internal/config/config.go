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
// History is the exception: it is a *int because 0 is a meaningful value that
// disables recent-commit context. nil means "not set" in an override layer; a
// non-nil pointer — including one pointing to 0 — means "explicitly set".
type Config struct {
	DetailLevel DetailLevel
	Profile     ProfileName
	Model       string
	Hint        string
	History     *int
	APIKey      string
	WrapLine    int
	// Issues are the issue-tracker numbers this commit fixes. nil means "not
	// set"; each number adds a "Fixes: #N" git trailer before "Assisted-by:".
	Issues []int
}

// Ptr returns a pointer to a copy of v, expressing "explicitly set to v" —
// including v == 0 — in an override layer, as opposed to "not set" (nil).
// The returned pointer is never written through, only replaced, so sharing it
// across merged configs is safe.
func Ptr[T any](v T) *T {
	x := v

	return &x
}

// HistoryValue returns the effective History value, treating an unset History
// as 0 (disabled). It is a nil-safe accessor for consumers.
func (c Config) HistoryValue() int {
	if c.History == nil {
		return 0
	}

	return *c.History
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

	if c.History != nil {
		if *c.History < 0 {
			c.History = Ptr(0)
		} else if *c.History > maxHistory {
			c.History = Ptr(maxHistory)
		}
	}

	if c.WrapLine < minWrap {
		c.WrapLine = minWrap
	} else if c.WrapLine > maxWrap {
		c.WrapLine = maxWrap
	}

	if c.Issues != nil {
		// Drop non-positive entries and duplicates, preserving order.
		seen := make(map[int]struct{}, len(c.Issues))

		deduped := make([]int, 0, len(c.Issues))
		for _, n := range c.Issues {
			if n <= 0 {
				continue
			}

			if _, ok := seen[n]; ok {
				continue
			}

			seen[n] = struct{}{}
			deduped = append(deduped, n)
		}

		c.Issues = deduped
	}

	return c
}

// Merge returns a new Config with explicitly-set fields from override applied
// on top. Zero-value fields in override are left as-is from the receiver. This
// allows layering: file config → env overrides → CLI flag overrides.
//
// History is the only field that can be explicitly set to its zero value: a
// non-nil override.History — including one pointing to 0 (disable history) —
// always wins. WrapLine keeps zero-as-unset because 0 is meaningless for it
// and is clamped to minWrap during Validate.
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

	if override.Hint != "" {
		merged.Hint = override.Hint
	}

	if override.History != nil {
		merged.History = override.History
	}

	if override.APIKey != "" {
		merged.APIKey = override.APIKey
	}

	if override.WrapLine != 0 {
		merged.WrapLine = override.WrapLine
	}

	if override.Issues != nil {
		merged.Issues = override.Issues
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
