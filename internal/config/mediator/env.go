package mediator

import (
	"os"
	"strconv"
	"strings"

	"gud/internal/config"
)

// configFromEnv reads configuration from GUD_* environment variables.
// It returns only the fields that are explicitly set, leaving others
// as zero values so Merge() applies the correct priority.
//
// Recognised variables:
//
//	GUD_DETAIL_LEVEL  GUD_MODEL
//	GUD_HINT          GUD_HISTORY  GOOGLE_API_KEY GUD_WRAPLINE
//	GUD_NO_ASSISTED_BY
func configFromEnv() config.Config {
	cfg := config.Config{
		APIKey: firstSet("GOOGLE_API_KEY"),
		Model:  firstSet("GUD_MODEL", "GEMINI_MODEL"),
		Hint:   os.Getenv("GUD_HINT"),
	}

	v := os.Getenv("GUD_DETAIL_LEVEL")
	if v != "" {
		cfg.DetailLevel = config.DetailLevel(v)
	}

	v = os.Getenv("GUD_HISTORY")
	if v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			// Pointer (not plain int) so GUD_HISTORY=0 reliably disables
			// history instead of being treated as "not set" by Merge.
			cfg.History = config.Ptr(n)
		}
	}

	v = os.Getenv("GUD_WRAPLINE")
	if v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.WrapLine = n
		}
	}

	if value, ok := parseEnvFlag(os.Getenv("GUD_NO_ASSISTED_BY")); ok {
		cfg.NoAssistedBy = value
	}

	return cfg
}

// parseEnvFlag converts an environment-variable value to an explicit boolean
// setting. An empty or unrecognised value means "not set"; recognised
// true/false spellings preserve an explicit choice through Merge.
func parseEnvFlag(v string) (*bool, bool) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "t", "true", "y", "yes", "on":
		return config.Ptr(true), true
	case "0", "f", "false", "n", "no", "off":
		return config.Ptr(false), true
	default:
		return nil, false
	}
}

// firstSet returns the first non-empty environment variable value
// from the given keys. Returns empty string if none are set.
func firstSet(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}

	return ""
}
