package core

import (
	"context"
	"fmt"

	"gud/internal/helixdb"

	"github.com/spf13/cobra"
)

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show commit statistics from HelixDB memory",
	Long: `Display analytics about commits stored in HelixDB.

Requires HelixDB integration to be enabled (--helixdb-enabled).`,
	RunE: runStats,
}

var statsRepoCmd = &cobra.Command{
	Use:   "repo",
	Short: "Show summary stats for the current repository",
	RunE:  runStatsRepo,
}

func init() {
	statsCmd.AddCommand(statsRepoCmd)
}

func runStats(cmd *cobra.Command, _ []string) error {
	return cmd.Help()
}

func runStatsRepo(cmd *cobra.Command, _ []string) error {
	app, err := NewAppContext(cmd)
	if err != nil {
		return err
	}

	ctx := context.Background()

	if err := app.InitHelixDB(ctx); err != nil {
		return fmt.Errorf("helixdb: %w", err)
	}
	if !app.HelixDB().Enabled() {
		return fmt.Errorf("HelixDB is not enabled. Use --helixdb-enabled or set GUD_HELIXDB_ENABLED=true")
	}

	repoPath := getRepoRoot(ctx)

	q := helixdb.BuildRepoSummaryQuery(repoPath)
	var resp map[string]any
	if err := app.HelixDB().Exec(ctx, q, &resp); err != nil {
		return fmt.Errorf("failed to query stats: %w", err)
	}

	stats := helixdb.ParseRepoSummary(resp)
	_, _ = fmt.Fprint(cmd.OutOrStdout(), helixdb.FormatRepoSummary(stats))

	return nil
}

// getRepoRoot returns the git repo root path. Returns empty on error.
func getRepoRoot(ctx context.Context) string {
	return "." // simplified for now; would use git rev-parse --show-toplevel
}
