package cli

import (
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
	Context     string
	History     int
	APIKey      string
	Global      bool
}

var cfg Config

var rootCmd = &cobra.Command{
	Use:   "git-message",
	Short: "Spontaneously combust commit message",
	Long: `Tool that generates meaningful git commit messages
using Google's Gemini API, based on your staged changes.

It supports multiple personas and detail levels to match your project's style.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	// Default action: generate a commit message from staged changes
	RunE: func(cmd *cobra.Command, args []string) error {
		return runGenerate(cmd, args)
	},
}

// Execute runs the root command. It is the entry point for the CLI.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Persistent flags available to all commands
	rootCmd.PersistentFlags().StringVar((*string)(&cfg.DetailLevel), "detail-level", "standard", "Set the detail level (minimal, standard, detailed)")
	rootCmd.PersistentFlags().StringVar((*string)(&cfg.Persona), "persona", "embedded", "Set output style (embedded, conventional)")
	rootCmd.PersistentFlags().StringVar(&cfg.Hint, "hint", "", "Focus boundaries for the AI")
	rootCmd.PersistentFlags().StringVarP(&cfg.Context, "description", "d", "", "Short description of the change for the commit message")
	rootCmd.PersistentFlags().IntVar(&cfg.History, "history", 5, "Number of recent commits to include as context (0 to disable)")
	rootCmd.PersistentFlags().StringVar(&cfg.Model, "model", "", "Gemini model to use (or use GEMINI_MODEL env)")
	rootCmd.PersistentFlags().Float64Var(&cfg.Temperature, "temperature", 0, "Set the generation temperature (0-2, default: model default)")
	rootCmd.PersistentFlags().StringVar(&cfg.APIKey, "api-key", "", "Gemini API key (or use GEMINI_API_KEY env)")
}
