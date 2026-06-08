package core

import (
	"os"

	"gud/internal/request"

	"github.com/spf13/cobra"
)

const version = "0.1.0"

// Config holds CLI configuration shared across commands.
type Config struct {
	DetailLevel request.DetailLevel
	Persona     request.PersonaName
	Model       string
	Temperature float64
	Hint        string
	History     int
	APIKey      string
}

var rootCmd = &cobra.Command{
	Use:   "message",
	Short: "Spontaneously combust commit message",
	Long: `Tool that generates meaningful git commit messages
using Google's Gemini API, based on your staged changes.

It supports multiple personas and detail levels to match your project's style.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	// Default action: generate a commit message from staged changes
	RunE: runGenerate,
}

// Execute runs the root command. It is the entry point for the CLI.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Persistent flags available to all commands (no global variable binding)
	rootCmd.PersistentFlags().String("detail-level", "standard", "Set the detail level (minimal, standard, detailed)")
	rootCmd.PersistentFlags().String("persona", "embedded", "Set output style (embedded, conventional)")
	rootCmd.PersistentFlags().String("hint", "", "Focus boundaries for the AI")
	rootCmd.PersistentFlags().Int("history", 5, "Number of recent commits to include as context (0 to disable)")
	rootCmd.PersistentFlags().String("model", "", "Gemini model to use (or use GEMINI_MODEL env)")
	rootCmd.PersistentFlags().Float64("temperature", 0, "Set the generation temperature (0-2, default: model default)")
}

// configFromCmd reads flags from the cobra command and environment variables
// to construct a normalized Config.
func configFromCmd(cmd *cobra.Command) Config {
	detail, _ := cmd.Flags().GetString("detail-level")
	persona, _ := cmd.Flags().GetString("persona")
	hint, _ := cmd.Flags().GetString("hint")
	history, _ := cmd.Flags().GetInt("history")
	model, _ := cmd.Flags().GetString("model")
	temp, _ := cmd.Flags().GetFloat64("temperature")

	cfg := Config{
		DetailLevel: request.DetailLevel(detail),
		Persona:     request.PersonaName(persona),
		Hint:        hint,
		History:     history,
		Model:       model,
		Temperature: temp,
		APIKey:      os.Getenv("GOOGLE_API_KEY"),
	}
	if cfg.Model == "" {
		cfg.Model = os.Getenv("GEMINI_MODEL")
	}
	cfg = validateConfig(cfg)

	return cfg
}
