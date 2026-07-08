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
	"gud/internal/mem"

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
	promptContext = maybeAppendMEMContext(ctx, app, diff, units, promptContext)

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

// maybeAppendMEMContext queries HelixDB for semantically relevant context
// based on the current diff and appends it to the prompt context string.
// Uses BM25 text search and entity-aware recall via MENTIONS edges.
// Errors are logged and silently discarded — HelixDB context is optional.
func maybeAppendMEMContext(
	ctx context.Context, app *AppContext, diff string,
	units []git.CodeUnit, existingContext string,
) string {
	db := app.HelixDB()
	if db == nil || !db.Enabled() || !db.IsAvailable(ctx) {
		return existingContext
	}

	repoPath, err := git.GetRepoRoot(ctx)
	if err != nil || repoPath == "" {
		slog.Debug("helixdb: failed to get repo root", "error", err)

		return existingContext
	}

	filePaths := make([]string, 0, len(units))
	codeElemKeys := make([]string, 0, len(units))
	for _, u := range units {
		filePaths = append(filePaths, u.FilePath)
		codeElemKeys = append(codeElemKeys, fmt.Sprintf("%s:%s:%s", repoPath, u.FilePath, u.Name))
	}

	var allRecords []mem.CommitRecord

	// 1. BM25 context query using diff text and file name signals.
	branch := ""
	query := mem.BuildContextQuery(repoPath, branch, filePaths, diff)
	var resp map[string]any
	if err := db.Exec(ctx, query, &resp); err != nil {
		slog.Debug("helixdb bm25 context query failed", "error", err)
	} else {
		allRecords = mem.ParseContextResults(resp)
	}

	// 2. Entity-aware recall: find commits mentioning the same code elements.
	if len(codeElemKeys) > 0 {
		entityQ := mem.BuildEntityContextQuery(repoPath, codeElemKeys, 3)
		var entityResp map[string]any
		if err := db.Exec(ctx, entityQ, &entityResp); err != nil {
			slog.Debug("helixdb entity context query failed", "error", err)
		} else {
			entityRecords := mem.ParseContextResults(entityResp)
			allRecords = append(allRecords, entityRecords...)
		}
	}

	// Deduplicate by SHA.
	seen := make(map[string]bool)
	var deduped []mem.CommitRecord
	for _, r := range allRecords {
		if !seen[r.SHA] {
			seen[r.SHA] = true
			deduped = append(deduped, r)
		}
	}

	if len(deduped) == 0 {
		return existingContext
	}

	ctxStr := mem.FormatContextRecords(deduped)
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
