package core

import (
	"os"
	"strconv"

	"gud/internal/config"

	"github.com/spf13/cobra"
)

const version = "0.1.0"

var rootCmd = &cobra.Command{
	Use:   "message",
	Short: "Spontaneously combust commit message",
	Long: `Tool that generates meaningful git commit messages
using AI, based on your staged changes.

It supports multiple profiles and detail levels to match your project's style.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	// Default action: generate a commit message from staged changes
	RunE: runGenerate,
}

// Execute runs the root command. It is the entry point for the CLI.
func Execute() error {
	return rootCmd.Execute()
}

// mustGet panics if the flag name is not registered. Only use for flags
// defined in init() — a missing flag is a programming error.
func mustGet[T any](_ *cobra.Command, name string, fn func(string) (T, error)) T {
	v, err := fn(name)
	if err != nil {
		panic("config: " + err.Error())
	}

	return v
}

func init() {
	// Persistent flags available to all commands (no global variable binding)
	rootCmd.PersistentFlags().String("detail-level", "standard", "Set the detail level (minimal, standard, detailed)")
	rootCmd.PersistentFlags().String("profile", "", "AI agent profile slug (download with 'gud profile save <slug>')")
	rootCmd.PersistentFlags().String("hint", "", "Focus boundaries for the AI")
	rootCmd.PersistentFlags().Int("history", 5, "Number of recent commits to include as context (0 to disable)")
	rootCmd.PersistentFlags().String("model", "", "Gemini model to use (or use GEMINI_MODEL env)")
	rootCmd.PersistentFlags().Float64("temperature", 1, "Set the generation temperature (0-2, default: 1)")
	rootCmd.PersistentFlags().Int("wrapline", 72, "Wrap all lines at this character width")

	rootCmd.AddCommand(profileCmd)
}

// configFromCmd reads flags from the cobra command to build the CLI override layer.
// It does NOT read environment variables — that is handled by configFromEnv.
func configFromCmd(cmd *cobra.Command) config.Config {
	detail := mustGet(cmd, "detail-level", cmd.Flags().GetString)
	profile := mustGet(cmd, "profile", cmd.Flags().GetString)
	hint := mustGet(cmd, "hint", cmd.Flags().GetString)
	history := mustGet(cmd, "history", cmd.Flags().GetInt)
	model := mustGet(cmd, "model", cmd.Flags().GetString)
	temp := mustGet(cmd, "temperature", cmd.Flags().GetFloat64)
	wrapLine := mustGet(cmd, "wrapline", cmd.Flags().GetInt)

	return config.Config{
		DetailLevel: config.DetailLevel(detail),
		Profile:     config.ProfileName(profile),
		Hint:        hint,
		History:     history,
		Model:       model,
		Temperature: temp,
		WrapLine:    wrapLine,
		ACP:         config.ACPOpencode,
	}
}

// configFromEnv reads configuration from GUD_* environment variables.
// It returns only the fields that are explicitly set, leaving others
// as zero values so Merge() applies the correct priority.
//
// Recognised variables:
//
//	GUD_DETAIL_LEVEL  GUD_PROFILE  GUD_MODEL   GUD_TEMPERATURE
//	GUD_HINT          GUD_HISTORY  GUD_API_KEY GUD_WRAPLINE
//	OPENCODE_API_KEY                  (alias for GUD_API_KEY)
//	GEMINI_MODEL                      (alias for GUD_MODEL)
func configFromEnv() config.Config {
	cfg := config.Config{
		APIKey:  firstSet("GUD_API_KEY", "OPENCODE_API_KEY"),
		Model:   firstSet("GUD_MODEL", "GEMINI_MODEL"),
		Profile: config.ProfileName(firstSet("GUD_PROFILE")),
		Hint:    os.Getenv("GUD_HINT"),
	}

	v := os.Getenv("GUD_DETAIL_LEVEL")
	if v != "" {
		cfg.DetailLevel = config.DetailLevel(v)
	}

	v = os.Getenv("GUD_TEMPERATURE")
	if v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.Temperature = f
		}
	}

	v = os.Getenv("GUD_HISTORY")
	if v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.History = n
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

// firstSet returns the first non-empty environment variable from the given keys.
func firstSet(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}

	return ""
}
