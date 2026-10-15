// Package config defines the domain entity for application configuration.
// It is a pure business concept with no format-specific tags or serialization
// concerns. Use the dto sub-package for JSON interchange.
package config

import "slices"

// DetailLevel controls the verbosity of generated commit messages.
type DetailLevel string

const (
	DetailMinimal  DetailLevel = "minimal"
	DetailStandard DetailLevel = "standard"
	DetailDetailed DetailLevel = "detailed"
)

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
	Model       string
	Hint        string
	History     *int
	APIKey      string
	WrapLine    int
	// Issues are the issue-tracker numbers this commit fixes. nil means "not
	// set"; each number adds a "Fixes: #N" git trailer before "Assisted-by:".
	Issues []int
	// NoAssistedBy explicitly suppresses the "Assisted-by" trailer, even for
	// generated messages. nil means "not set"; a non-nil value wins during
	// Merge. The effective default is false, leaving the trailer on.
	NoAssistedBy *bool
}

// AssistedByTrailerSuppressed reports whether the "Assisted-by" footer should
// be omitted as configured by the user.
func (c Config) AssistedByTrailerSuppressed() bool {
	return c.NoAssistedBy != nil && *c.NoAssistedBy
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
// as 0 (disabled). A nil History only occurs in override layers or zero-value
// Configs; the merged effective config carries DefaultHistory unless a layer
// explicitly sets it. It is a nil-safe accessor for consumers.
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

// DefaultWrapLine is the canonical default for wrapping commit-message lines.
// request.defaultWrapLine aliases this so the 72-char default has one source.
const DefaultWrapLine = 72

// DefaultHistory is the canonical default for the number of recent commits
// included as prompt context. The --history flag default in root.go aliases
// this so the advertised default and the effective default cannot drift:
// configFromCmd only applies the flag when explicitly Changed, so an unset
// flag leaves the base layer's DefaultHistory intact, while --history 0
// still disables history via an explicit Ptr(0) override.
const DefaultHistory = 5

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
		merged.Issues = slices.Clone(override.Issues)
	}

	if override.NoAssistedBy != nil {
		merged.NoAssistedBy = override.NoAssistedBy
	}

	return merged
}

// DefaultConfig returns a Config populated with sensible defaults.
// History defaults to DefaultHistory (not nil): a nil History in an override
// layer means "not set", but the merged effective config always carries the
// default unless a layer explicitly sets it (including Ptr(0) to disable).
func DefaultConfig() Config {
	return Config{
		DetailLevel:  DetailStandard,
		WrapLine:     DefaultWrapLine,
		History:      Ptr(DefaultHistory),
		NoAssistedBy: Ptr(false),
	}
}
