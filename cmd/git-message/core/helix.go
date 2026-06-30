package core

import (
	"fmt"

	"github.com/spf13/cobra"
)

var helixCmd = &cobra.Command{
	Use:   "helix",
	Short: "Manage the local HelixDB instance",
	Long: `Start, stop, and check the HelixDB Docker container used for
commit memory and analytics.

Requires Docker to be installed and the daemon running.`,
	RunE: runHelix,
}

var helixStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the managed HelixDB container",
	RunE:  runHelixStop,
}

var helixStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check if the HelixDB container is running",
	RunE:  runHelixStatus,
}

func init() {
	helixCmd.AddCommand(helixStopCmd)
	helixCmd.AddCommand(helixStatusCmd)
}

func runHelix(cmd *cobra.Command, _ []string) error {
	return cmd.Help()
}

func runHelixStop(cmd *cobra.Command, _ []string) error {
	app, err := NewAppContext(cmd)
	if err != nil {
		return err
	}

	container := app.ContainerManager()
	if !container.IsRunning(cmd.Context()) {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "HelixDB container is not running.")

		return nil
	}

	if err := container.Stop(cmd.Context()); err != nil {
		return fmt.Errorf("stop helixdb: %w", err)
	}
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "HelixDB container stopped.")

	return nil
}

func runHelixStatus(cmd *cobra.Command, _ []string) error {
	app, err := NewAppContext(cmd)
	if err != nil {
		return err
	}

	container := app.ContainerManager()
	if container.IsRunning(cmd.Context()) {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(),
			"HelixDB container %q is running.\n", container.ContainerName())
		_, _ = fmt.Fprintln(cmd.OutOrStdout(),
			"URL: http://localhost:6969")
	} else {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "HelixDB container is not running.")
	}

	return nil
}

// extractPort extracts the port from a URL like "http://localhost:6969".
func extractPort(url string) string {
	if url == "" {
		return "6969"
	}
	// Simple heuristic: find the last colon after "://"
	for i := len(url) - 1; i >= 0; i-- {
		if url[i] == ':' {
			return url[i+1:]
		}
	}

	return "6969"
}
