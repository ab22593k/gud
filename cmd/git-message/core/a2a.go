package core

import (
	"errors"

	"github.com/spf13/cobra"
)

var a2aCmd = &cobra.Command{
	Use:   "a2a",
	Short: "Run as an A2A agent server (WIP)",
	Long: `Start an A2A (Agent-to-Agent) protocol server that exposes
the commit message generation agent for inter-agent communication.

NOTE: Full A2A server implementation is pending the ADK's HTTP binding API.`,

	RunE: func(_ *cobra.Command, _ []string) error {
		return errors.New("A2A server not yet implemented; " +
			"use 'gud' without subcommand for normal usage, " +
			"or set --acp opencode to use OpenCode.ai models")
	},
}

func init() {
	rootCmd.AddCommand(a2aCmd)
}
