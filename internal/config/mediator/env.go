package mediator

import (
	"os"
	"strconv"

	"gud/internal/config"
)

// configFromEnv reads configuration from GUD_* environment variables.
// It returns only the fields that are explicitly set, leaving others
// as zero values so Merge() applies the correct priority.
//
// Recognised variables:
//
//	GUD_DETAIL_LEVEL  GUD_PROFILE  GUD_MODEL
//	GUD_HINT          GUD_HISTORY  GOOGLE_API_KEY GUD_WRAPLINE
func configFromEnv() config.Config {
	cfg := config.Config{
		APIKey:  firstSet("GOOGLE_API_KEY"),
		Model:   firstSet("GUD_MODEL", "GEMINI_MODEL"),
		Profile: config.ProfileName(firstSet("GUD_PROFILE")),
		Hint:    os.Getenv("GUD_HINT"),
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

	return cfg
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
