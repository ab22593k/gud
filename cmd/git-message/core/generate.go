package core

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"gud/internal/git"
	"gud/internal/helixdb"

	"github.com/spf13/cobra"
)

// maxHistory is the maximum number of recent commits the --history flag can request.
// This prevents accidentally dumping hundreds of commits into the prompt and wasting tokens.
const maxHistory = git.MaxRecentCommits

// runGenerate is the default action: generate a commit message from staged changes.
func runGenerate(cmd *cobra.Command, _ []string) error {
	app, err := NewAppContext(cmd)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := app.InitHelixDB(ctx); err != nil {
		slog.Debug("helixdb init failed, proceeding without", "error", err)
	}

	// Stop HelixDB container on return if we auto-started it.
	defer app.StopHelixDB(ctx)

	if err := app.InitClient(ctx); err != nil {
		return err
	}

	diff, err := getStagedDiffOrError(ctx)
	if err != nil {
		return err
	}

	units := git.ExtractCodeUnits(diff)

	promptContext := buildHistoryContext(ctx, app)
	promptContext = maybeAppendHelixDBContext(ctx, app, diff, units, promptContext)

	return interactiveCommit(ctx, cmd, app, diff, promptContext, units)
}

// resolveProfileContent returns the AGENTS.md content for a cached profile.
// Returns empty string if no profile is set.
func resolveProfileContent(profileName string) string {
	if profileName == "" {
		return ""
	}
	initProfileManager()
	p, err := profileManager.Get(profileName)
	if err != nil {
		slog.Debug("profile not found in cache", "profile", profileName)

		return ""
	}

	return p.Content
}

// requireProfile checks that the given profile is cached (or empty).
// If set but not found, it tells the user to download it first.
func requireProfile(profileName string) error {
	if profileName == "" {
		return nil
	}
	initProfileManager()
	_, err := profileManager.Get(profileName)
	if err != nil {
		return fmt.Errorf("profile %q not found.\n\n"+
			"First download it:  gud profile save %s\n"+
			"See all:            gud profile list --remote", profileName, profileName)
	}

	return nil
}

// getStagedDiffOrError retrieves the staged diff and returns an error if none exists.
func getStagedDiffOrError(ctx context.Context) (string, error) {
	diff, deleted, err := getStagedDiffAndDeleted(ctx)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(diff) == "" {
		return "", errors.New("no staged changes found. Use 'git add' to stage changes")
	}

	return appendDeletedContext(diff, deleted), nil
}

// getStagedDiffAndDeleted retrieves both the staged diff (excluding deleted content)
// and the list of deleted file names.
func getStagedDiffAndDeleted(ctx context.Context) (diff, deleted string, err error) {
	diff, err = git.GetStagedDiff(ctx)
	if err != nil {
		return "", "", fmt.Errorf("failed to get staged diff: %w", err)
	}

	deleted, err = git.GetStagedDeletedFiles(ctx)
	if err != nil {
		return "", "", fmt.Errorf("failed to get deleted files: %w", err)
	}

	return diff, deleted, nil
}

// appendDeletedContext appends a note about deleted files to the diff if any exist.
func appendDeletedContext(diff, deleted string) string {
	if strings.TrimSpace(deleted) == "" {
		return diff
	}

	// Sanitize each filename line: trim whitespace, filter empty lines.
	// This prevents injection of fake diff context via malicious filenames
	// containing embedded newlines, while preserving the one-per-line format.
	lines := strings.Split(deleted, "\n")
	var clean []string
	for _, line := range lines {
		if line = strings.TrimSpace(line); line != "" {
			clean = append(clean, line)
		}
	}

	var b strings.Builder
	b.WriteString(diff)
	b.WriteString("\n\nDeleted files:\n")
	for i, line := range clean {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(line)
	}
	b.WriteString("\n")

	return b.String()
}

// maybeAppendHelixDBContext queries HelixDB for semantically relevant context
// based on the current diff and appends it to the prompt context string.
// Errors are logged and silently discarded — HelixDB context is optional.
func maybeAppendHelixDBContext(ctx context.Context, app *AppContext, diff string, units []git.CodeUnit, existingContext string) string {
	db := app.HelixDB()
	if db == nil || !db.Enabled() || !db.IsAvailable(ctx) {
		return existingContext
	}

	if len(units) == 0 {
		return existingContext
	}

	filePaths := make([]string, 0, len(units))
	for _, u := range units {
		filePaths = append(filePaths, u.FilePath)
	}

	repoPath, err := git.GetRepoRoot(ctx)
	if err != nil || repoPath == "" {
		slog.Debug("helixdb: failed to get repo root", "error", err)
		return existingContext
	}

	branch := ""
	query := helixdb.BuildContextQuery(repoPath, branch, filePaths, diff)
	var resp map[string]any
	if err := db.Exec(ctx, query, &resp); err != nil {
		slog.Debug("helixdb context query failed", "error", err)
		return existingContext
	}

	records := helixdb.ParseContextResults(resp)
	if len(records) == 0 {
		return existingContext
	}

	ctxStr := helixdb.FormatContextRecords(records)
	if existingContext != "" {
		return existingContext + "\n\n" + ctxStr
	}

	return ctxStr
}

// buildHistoryContext returns a formatted string of recent commit history, or
// empty string if --history is disabled or there are no recent commits.
// Errors from git are logged at debug level and silently discarded —
// history is optional context for the AI prompt and should never block generation.
func buildHistoryContext(ctx context.Context, app *AppContext) string {
	var history string
	if app.Config().History > 0 {
		var err error
		history, err = git.GetRecentCommits(ctx, app.Config().History)
		if err != nil {
			slog.Debug("failed to get recent commits, proceeding without history", "error", err)
			history = ""
		}
	}
	if history != "" {
		history = "Recent commits:\n" + history
	}

	return history
}
