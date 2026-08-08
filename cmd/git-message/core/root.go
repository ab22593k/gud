package core

import (
	"log/slog"
	"os"
	"strconv"
	"strings"

	"gud/internal/config"

	"github.com/spf13/cobra"
)

// The root command is intentionally named "message" (not "gud") so git
// invokes it as `git message`: the binary is built from cmd/git-message and
// named git-message, and git runs any git-* executable on PATH as `git <name>`.
// "gud" is the product name used in version output (gud version X.Y.Z); the
// two spellings refer to the same command. See README "Naming".
var rootCmd = &cobra.Command{
	Use:   "message",
	Short: "Spontaneously combust commit message",
	Long: `Tool that generates meaningful git commit messages
using AI, based on your staged changes.

It supports multiple profiles and detail levels to match your project's style.

Invoked as 'git message'; 'gud message' is the same command.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	// Default action: generate a commit message from staged changes
	RunE: runGenerate,
}

// Execute runs the root command. It is the entry point for the CLI.
func Execute() error {
	setupLogLevel()

	return rootCmd.Execute()
}

const (
	logLevelDebug = "debug"
	logLevelWarn  = "warn"
	logLevelError = "error"
)

// parseLogLevel maps a GUD_LOG_LEVEL value to a slog level. Unknown or empty
// values map to Info (the slog default).
func parseLogLevel(v string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case logLevelDebug:
		return slog.LevelDebug
	case logLevelWarn:
		return slog.LevelWarn
	case logLevelError:
		return slog.LevelError
	default: // "", "info", and anything unrecognised
		return slog.LevelInfo
	}
}

// setupLogLevel configures the global slog level from the GUD_LOG_LEVEL
// environment variable. All gud diagnostics (including HelixDB memory
// retrieval) use slog.Debug, so set GUD_LOG_LEVEL=debug to observe them.
func setupLogLevel() {
	slog.SetLogLoggerLevel(parseLogLevel(os.Getenv("GUD_LOG_LEVEL")))
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

// addPersistentFlags registers all persistent flags on cmd. The flag defaults
// are cobra defaults only — configFromCmd only applies a flag when the user
// explicitly set it, so these defaults never override gud.json or env values.
// Subcommands are attached to rootCmd separately in init().
func addPersistentFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().String("detail-level", "standard", "Set the detail level (minimal, standard, detailed)")
	cmd.PersistentFlags().String("profile", "", "AI agent profile slug (download with 'git message profile save <slug>')")
	cmd.PersistentFlags().String("hint", "", "Focus boundaries for the AI")
	cmd.PersistentFlags().Int("history", 5, "Number of recent commits to include as context (0 to disable)")
	cmd.PersistentFlags().String("model", "", "Gemini model to use (or use GEMINI_MODEL env)")
	cmd.PersistentFlags().StringSlice("issue", nil,
		"Issue numbers this commit fixes (comma-separated, e.g. 123,456; adds a 'Fixes: #N' trailer per issue)")
	cmd.PersistentFlags().Int("wrapline", 72, "Wrap all lines at this character width")
}

func init() {
	addPersistentFlags(rootCmd)

	rootCmd.AddCommand(profileCmd)
}

// configFromCmd reads flags from the cobra command to build the CLI override layer.
// It does NOT read environment variables — that is handled by the mediator.
//
// Only flags that were explicitly set on the command line are populated; the
// rest are left as zero values. This is critical: cobra flag defaults (e.g.
// --detail-level standard, --wrapline 72, --history 5) are NOT user intent, so
// they must not leak into the override layer and clobber gud.json / env
// settings. config.Config.Merge treats any non-zero field as "explicitly set",
// so leaving these zero preserves the documented priority
// "CLI flags → env → gud.json" for anything the user did not pass.
func configFromCmd(cmd *cobra.Command) config.Config {
	cfg := config.Config{}
	flags := cmd.Flags()

	if flags.Changed("detail-level") {
		cfg.DetailLevel = config.DetailLevel(mustGet(cmd, "detail-level", flags.GetString))
	}
	if flags.Changed("profile") {
		cfg.Profile = config.ProfileName(mustGet(cmd, "profile", flags.GetString))
	}
	if flags.Changed("hint") {
		cfg.Hint = mustGet(cmd, "hint", flags.GetString)
	}
	if flags.Changed("history") {
		// Pointer (not plain int) so --history 0 reliably disables history
		// instead of being treated as "not set" by config.Config.Merge.
		cfg.History = config.Ptr(mustGet(cmd, "history", flags.GetInt))
	}
	if flags.Changed("model") {
		cfg.Model = mustGet(cmd, "model", flags.GetString)
	}
	if flags.Changed("wrapline") {
		cfg.WrapLine = mustGet(cmd, "wrapline", flags.GetInt)
	}
	if flags.Changed("issue") {
		cfg.Issues = parseIssueList(mustGet(cmd, "issue", flags.GetStringSlice))
	}

	return cfg
}

// parseIssueList converts --issue flag values (comma-separated via cobra's
// StringSlice, which also accepts repeated flags) into issue numbers. Empty
// and non-numeric entries are ignored; the caller's Validate deduplicates.
func parseIssueList(values []string) []int {
	var issues []int
	for _, v := range values {
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			continue
		}
		n, err := strconv.Atoi(trimmed)
		if err != nil || n <= 0 {
			continue
		}
		issues = append(issues, n)
	}

	return issues
}
