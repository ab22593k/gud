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

	"gud/internal/detect"
	"gud/internal/git"
	"gud/internal/mem"

	"github.com/spf13/cobra"
)

// maxHistory is the maximum number of recent commits the --history flag can request.
// This prevents accidentally dumping hundreds of commits into the prompt and wasting tokens.
const maxHistory = git.MaxRecentCommits

// maxContextRecords caps how many related commits HelixDB recall may return to
// the prompt. Matches the per-source limit used by the mem package so fused
// results stay within the same token budget.
const maxContextRecords = 5

// runGenerate is the default action: generate a commit message from staged changes.
func runGenerate(cmd *cobra.Command, _ []string) error {
	app, err := NewAppContext(cmd)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Check for staged changes before any interactive flow. A user with
	// nothing staged should get the "no staged changes" error immediately,
	// not a HelixDB probe or a profile suggestion that may write
	// .gud-skip/gud.json. InitHelixDB/InitClient run after this check.
	diff, err := getStagedDiffOrError(ctx)
	if err != nil {
		return err
	}

	if err := app.InitHelixDB(ctx); err != nil {
		slog.Debug("helixdb init failed, proceeding without", "error", err)
	}

	if err := app.InitClient(ctx); err != nil {
		return err
	}

	// Suggest a profile if none is configured (first invocation in this repo).
	if app.Config().Profile == "" {
		if err := suggestProfileIfNeeded(ctx, cmd, app); err != nil {
			slog.Debug("profile suggestion skipped", "error", err)
		}
	}

	units := git.ExtractCodeUnits(diff)

	promptContext := buildRepoContext(ctx, app)
	promptContext = joinContexts(promptContext, buildHistoryContext(ctx, app))
	promptContext = maybeAppendMEMContext(ctx, app, diff, units, promptContext)

	return interactiveCommit(ctx, cmd, app, diff, promptContext, units)
}

// resolveProfileContent returns the AGENTS.md content for a cached profile.
// Returns empty string if no profile is set.
//
// A configured profile that is not cached logs a warning (not just debug):
// the content silently degrades to "", and in practice only hook mode reaches
// this branch — interactive mode fails earlier in the strict NewAppContext —
// so the warning surfaces the degradation to users instead of hiding it.
func resolveProfileContent(profileName string) string {
	if profileName == "" {
		return ""
	}
	initProfileManager()
	p, err := profileManager.Get(profileName)
	if err != nil {
		slog.Warn("configured profile not cached; proceeding without profile content",
			"profile", profileName,
			"hint", "git message profile save "+profileName)

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
			"First download it:  git message profile save %s\n"+
			"See all:            git message profile list --remote", profileName, profileName)
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

// getStagedDiffAndDeleted retrieves both the staged diff and the list of
// deleted file names from a single git subprocess call.
func getStagedDiffAndDeleted(ctx context.Context) (diff, deleted string, err error) {
	changes, err := git.GetStagedChanges(ctx)
	if err != nil {
		return "", "", fmt.Errorf("failed to get staged changes: %w", err)
	}

	diff = changes.Diff
	if len(changes.Deleted) > 0 {
		deleted = strings.Join(changes.Deleted, "\n") + "\n"
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
// Retrieval is hybrid: vector similarity over commit embeddings (when a query
// embedding can be computed), BM25 over diff text and commit messages, and
// entity-aware recall via MENTIONS edges. Results are fused with reciprocal
// rank fusion and re-ranked by recency. All errors are logged and silently
// discarded — HelixDB context is optional.
func maybeAppendMEMContext(
	ctx context.Context, app *AppContext, diff string,
	units []git.CodeUnit, existingContext string,
) string {
	db := app.HelixDB()
	if db == nil || !db.Enabled() || !db.IsAvailable(ctx) {
		slog.Debug("helixdb: skipping context retrieval, server unavailable")

		return existingContext
	}

	repoPath, err := app.RepoRoot(ctx)
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

	// Query embedding for vector recall. A failed embedding call is non-fatal:
	// retrieval falls back to BM25 and entity recall. The result is memoised
	// per invocation and reused by post-commit persistence, so the same diff
	// is embedded at most once.
	var queryVec []float32
	if vec, err := app.EmbedDiff(ctx, diff); err != nil {
		slog.Debug("helixdb: query embedding failed, falling back to BM25", "error", err)
	} else {
		queryVec = vec
	}

	// Branch is memoised per invocation; each query building on it reuses the
	// single subprocess spawn instead of paying one per query builder.
	branch := app.Branch(ctx)

	var groups []mem.RankedGroup

	// 1. Hybrid recall: vector + BM25 over diff text + BM25 over messages.
	query := mem.BuildHybridContextQuery(repoPath, branch, queryVec, diff, filePaths, maxContextRecords)
	var rawResp map[string]any
	if err := db.Exec(ctx, query, &rawResp); err != nil {
		slog.Debug("helixdb hybrid context query failed", "error", err)
	} else {
		groups = append(groups, mem.CollectContextGroups(mem.NewResponse(rawResp))...)
	}

	// 2. Entity-aware recall: find commits mentioning the same code elements.
	if len(codeElemKeys) > 0 {
		entityQ := mem.BuildEntityContextQuery(repoPath, branch, codeElemKeys, 3)
		var rawEntityResp map[string]any
		if err := db.Exec(ctx, entityQ, &rawEntityResp); err != nil {
			slog.Debug("helixdb entity context query failed", "error", err)
		} else {
			groups = append(groups, mem.CollectContextGroups(mem.NewResponse(rawEntityResp))...)
		}
	}

	records := mem.FuseContextRecords(groups, maxContextRecords)
	if len(records) == 0 {
		return existingContext
	}

	logRetrievedRecords(records, repoPath, groups)
	ctxStr := mem.FormatContextRecords(records)
	if existingContext != "" {
		return existingContext + "\n\n" + ctxStr
	}

	return ctxStr
}

// logRetrievedRecords logs a debug summary of what HelixDB recall produced.
func logRetrievedRecords(records []mem.CommitRecord, repoPath string, groups []mem.RankedGroup) {
	slog.Debug("helixdb: retrieved context records",
		"count", len(records),
		"repo", repoPath,
		"sources", contextGroupKeys(groups),
		"top", firstLine(records[0].Message),
	)
}

// contextGroupKeys returns the retrieval sources that produced ranked results.
func contextGroupKeys(groups []mem.RankedGroup) []string {
	keys := make([]string, 0, len(groups))
	for _, g := range groups {
		keys = append(keys, g.Key)
	}

	return keys
}

// firstLine returns the first line of s, or "" if s is empty.
func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")

	return line
}

// buildRepoContext returns a formatted string of repository file statistics,
// or empty string if the stats cannot be computed.
func buildRepoContext(ctx context.Context, app *AppContext) string {
	repoRoot, err := app.RepoRoot(ctx)
	if err != nil {
		slog.Debug("failed to get repo root for stats", "error", err)

		return ""
	}

	stats, err := detect.ComputeStats(repoRoot)
	if err != nil {
		slog.Debug("failed to compute repo stats", "error", err)

		return ""
	}

	return detect.FormatRepoContext(stats)
}

// joinContexts joins two non-empty context strings with a blank line separator.
// Returns whichever is non-empty if the other is empty, or empty if both are.
func joinContexts(a, b string) string {
	switch {
	case a == "" && b == "":
		return ""
	case a == "":
		return b
	case b == "":
		return a
	default:
		return a + "\n\n" + b
	}
}

// buildHistoryContext returns a formatted string of recent commit history, or
// empty string if --history is disabled or there are no recent commits.
// Errors from git are logged at debug level and silently discarded —
// history is optional context for the AI prompt and should never block generation.
func buildHistoryContext(ctx context.Context, app *AppContext) string {
	var history string
	if n := app.Config().HistoryValue(); n > 0 {
		var err error
		history, err = git.GetRecentCommits(ctx, n)
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
