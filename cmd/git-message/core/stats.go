package core

import (
	"context"
	"errors"
	"fmt"

	"gud/internal/git"
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

var (
	statsFileLimit int
	statsTrendDays int
)

var statsRepoCmd = &cobra.Command{
	Use:   "repo",
	Short: "Show summary stats for the current repository",
	RunE:  runStatsRepo,
}

var statsAuthorCmd = &cobra.Command{
	Use:   "author [email]",
	Short: "Show commit stats grouped by author",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runStatsAuthor,
}

var statsFilesCmd = &cobra.Command{
	Use:   "files",
	Short: "Show most frequently changed files",
	RunE:  runStatsFiles,
}

var statsTrendsCmd = &cobra.Command{
	Use:   "trends",
	Short: "Show commit activity over time",
	RunE:  runStatsTrends,
}

func init() {
	statsCmd.AddCommand(statsRepoCmd)
	statsCmd.AddCommand(statsAuthorCmd)
	statsCmd.AddCommand(statsFilesCmd)
	statsCmd.AddCommand(statsTrendsCmd)

	statsFilesCmd.Flags().IntVarP(&statsFileLimit, "limit", "n", 10, "Number of top files to show")
	statsTrendsCmd.Flags().IntVarP(&statsTrendDays, "days", "d", 30, "Number of days of history")
}

func runStats(cmd *cobra.Command, _ []string) error {
	return cmd.Help()
}

func initStatsApp(cmd *cobra.Command) (*AppContext, context.Context, error) {
	app, err := NewAppContext(cmd)
	if err != nil {
		return nil, nil, err
	}

	ctx := context.Background()

	if err := app.InitHelixDB(ctx); err != nil {
		return nil, nil, fmt.Errorf("helixdb: %w", err)
	}
	if !app.HelixDB().Enabled() {
		return nil, nil, errors.New("HelixDB is not enabled. Use --helixdb-enabled or set GUD_HELIXDB_ENABLED=true")
	}

	return app, ctx, nil
}

func repoPathOrError(ctx context.Context) (string, error) {
	repoPath, err := git.GetRepoRoot(ctx)
	if err != nil || repoPath == "" {
		return "", errors.New("not a git repository or unable to determine repo root")
	}

	return repoPath, nil
}

func runStatsRepo(cmd *cobra.Command, _ []string) error {
	app, ctx, err := initStatsApp(cmd)
	if err != nil {
		return err
	}

	repoPath, err := repoPathOrError(ctx)
	if err != nil {
		return err
	}

	q := helixdb.BuildRepoSummaryQuery(repoPath)
	var resp map[string]any
	if err := app.HelixDB().Exec(ctx, q, &resp); err != nil {
		return fmt.Errorf("failed to query stats: %w", err)
	}

	stats := helixdb.ParseRepoSummary(resp)
	_, _ = fmt.Fprint(cmd.OutOrStdout(), helixdb.FormatRepoSummary(stats))

	return nil
}

func runStatsAuthor(cmd *cobra.Command, args []string) error {
	app, ctx, err := initStatsApp(cmd)
	if err != nil {
		return err
	}

	repoPath, err := repoPathOrError(ctx)
	if err != nil {
		return err
	}

	q := helixdb.BuildAuthorStatsQuery(repoPath)
	var resp map[string]any
	if err := app.HelixDB().Exec(ctx, q, &resp); err != nil {
		return fmt.Errorf("failed to query author stats: %w", err)
	}

	stats := helixdb.ParseAuthorStats(resp)

	if len(args) > 0 {
		filtered := stats[:0]
		for _, s := range stats {
			if s.Email == args[0] {
				filtered = append(filtered, s)
			}
		}
		stats = filtered
	}

	_, _ = fmt.Fprint(cmd.OutOrStdout(), helixdb.FormatAuthorStats(stats))

	return nil
}

func runStatsFiles(cmd *cobra.Command, _ []string) error {
	app, ctx, err := initStatsApp(cmd)
	if err != nil {
		return err
	}

	repoPath, err := repoPathOrError(ctx)
	if err != nil {
		return err
	}

	q := helixdb.BuildTopFilesQuery(repoPath, statsFileLimit)
	var resp map[string]any
	if err := app.HelixDB().Exec(ctx, q, &resp); err != nil {
		return fmt.Errorf("failed to query file stats: %w", err)
	}

	stats := helixdb.ParseTopFiles(resp)
	_, _ = fmt.Fprint(cmd.OutOrStdout(), helixdb.FormatTopFiles(stats))

	return nil
}

func runStatsTrends(cmd *cobra.Command, _ []string) error {
	app, ctx, err := initStatsApp(cmd)
	if err != nil {
		return err
	}

	repoPath, err := repoPathOrError(ctx)
	if err != nil {
		return err
	}

	q := helixdb.BuildTrendsQuery(repoPath)
	var resp map[string]any
	if err := app.HelixDB().Exec(ctx, q, &resp); err != nil {
		return fmt.Errorf("failed to query trends: %w", err)
	}

	trends := helixdb.ParseTrends(resp)
	_, _ = fmt.Fprint(cmd.OutOrStdout(), helixdb.FormatTrends(trends))

	return nil
}
