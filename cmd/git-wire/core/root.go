// Package core implements the git wire command tree: single-subfolder
// fetch by default, tracked update with conflict-free merge, and
// list/status of tracked folders.
package core

import (
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// The root command fetches: `git wire <url>` downloads only the named
// subfolder and records it for future updates. It mirrors the fetch action
// the wire layer documents in wire.go.
var rootCmd = &cobra.Command{
	Use:   "wire <url>",
	Short: "Fetch a single subfolder from a hosted repository",
	Args:  cobra.ExactArgs(1),
	Long: `Fetch a single subfolder from a hosted repository without cloning it.

The URL names the owner, repository, reference, and folder, for example:
https://github.com/OCA/server-tools/tree/19.0/auto_backup

The folder is tracked for future updates (see 'git wire update').
Run 'git wire list' to see tracked folders.`,
	Example: `  git wire https://github.com/OCA/server-tools/tree/19.0/auto_backup -t ./auto_backup`,
	RunE: func(cmd *cobra.Command, args []string) error {
		initWireBackend()

		return fetchWith(cmd, args, wireBackend)
	},
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute runs the root command. It is the entry point for the CLI.
func Execute() error {
	setupLogLevel()

	return rootCmd.Execute()
}

// parseLogLevel maps a GUD_LOG_LEVEL value to a slog level. Unknown or empty
// values map to Info (the slog default). Duplicated from git-message (not
// shared) so the two binaries stay uncoupled.
func parseLogLevel(v string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default: // "", "info", and anything unrecognised
		return slog.LevelInfo
	}
}

// setupLogLevel configures the global slog level from the GUD_LOG_LEVEL
// environment variable.
func setupLogLevel() {
	slog.SetLogLoggerLevel(parseLogLevel(os.Getenv("GUD_LOG_LEVEL")))
}

func init() {
	addWireTargetFlags(rootCmd, fetchForceUsage)
	addWireTargetNameFlag(rootCmd)
	rootCmd.AddCommand(gitWireUpdateCmd)
	addWireTargetFlags(gitWireUpdateCmd, updateForceUsage)
	rootCmd.AddCommand(gitWireListCmd)
}
