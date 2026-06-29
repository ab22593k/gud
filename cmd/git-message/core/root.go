package core

import (
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
	rootCmd.PersistentFlags().Bool("helixdb-enabled", false, "Enable HelixDB memory and analytics")
	rootCmd.PersistentFlags().String("helixdb-url", "http://localhost:6969", "HelixDB server URL")
	rootCmd.PersistentFlags().Bool("helixdb-auto-manage", false, "Auto-start and stop HelixDB Docker container")
	rootCmd.PersistentFlags().String("helixdb-container-name", "gud-helixdb", "Docker container name for auto-managed HelixDB")

	rootCmd.AddCommand(profileCmd)
	rootCmd.AddCommand(statsCmd)
	rootCmd.AddCommand(helixCmd)
}

// configFromCmd reads flags from the cobra command to build the CLI override layer.
// It does NOT read environment variables — that is handled by the mediator.
func configFromCmd(cmd *cobra.Command) config.Config {
	detail := mustGet(cmd, "detail-level", cmd.Flags().GetString)
	profile := mustGet(cmd, "profile", cmd.Flags().GetString)
	hint := mustGet(cmd, "hint", cmd.Flags().GetString)
	history := mustGet(cmd, "history", cmd.Flags().GetInt)
	model := mustGet(cmd, "model", cmd.Flags().GetString)
	temp := mustGet(cmd, "temperature", cmd.Flags().GetFloat64)
	wrapLine := mustGet(cmd, "wrapline", cmd.Flags().GetInt)
	helixDBEnabled := mustGet(cmd, "helixdb-enabled", cmd.Flags().GetBool)
	helixDBURL := mustGet(cmd, "helixdb-url", cmd.Flags().GetString)
	helixDBAutoManage := mustGet(cmd, "helixdb-auto-manage", cmd.Flags().GetBool)
	helixDBContainerName := mustGet(cmd, "helixdb-container-name", cmd.Flags().GetString)

	return config.Config{
		DetailLevel:          config.DetailLevel(detail),
		Profile:              config.ProfileName(profile),
		Hint:                 hint,
		History:              history,
		Model:                model,
		Temperature:          temp,
		WrapLine:             wrapLine,
		HelixDBEnabled:       helixDBEnabled,
		HelixDBURL:           helixDBURL,
		HelixDBAutoManage:    helixDBAutoManage,
		HelixDBContainerName: helixDBContainerName,
	}
}
