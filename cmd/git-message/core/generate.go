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

	// Detect the in-progress git operation (merge, cherry-pick, revert,
	// rebase, squash, fixup) before checking staged changes: during an
	// operation stop the message may come from git's prepared message
	// instead of the staged diff, and some stops (e.g. a reword or a clean
	// squash message edit) have no staged changes at all.
	op := app.Operation(ctx)

	// Check for staged changes before any interactive flow. A user with
	// nothing staged should get the "no staged changes" error immediately,
	// not a HelixDB probe or a profile suggestion that may write
	// .gud-skip/gud.json. InitHelixDB/InitClient run after this check.
	diff, err := getStagedDiffOrError(ctx)
	if err != nil {
		if op == git.OperationNone {
			return err
		}
		// A git operation stop without staged changes is expected (e.g. a
		// reword stop): the commit message is git's prepared one, not
		// derived from a diff.
		slog.Debug("no staged changes while a git operation is in progress; using prepared message", "operation", op)
		diff = ""
	}

	if err := app.InitHelixDB(ctx); err != nil {
		slog.Debug("helixdb init failed, proceeding without", "error", err)
	}

	if err := app.InitClient(ctx); err != nil {
		// During an operation stop the message is git's prepared one, so an
		// API key is only needed if the user asks to regenerate. Completing
		// a merge or squash must not fail just because no key is configured.
		if op != git.OperationNone {
			slog.Debug("no request client while a git operation is in progress; regeneration will be unavailable", "error", err)
		} else {
			return err
		}
	}

	// Suggest a profile if none is configured (first invocation in this repo).
	// Skipped during an operation stop: completing a merge must not be
	// interrupted by a first-run profile prompt.
	if app.Config().Profile == "" && op == git.OperationNone {
		if err := suggestProfileIfNeeded(ctx, cmd, app); err != nil {
			slog.Debug("profile suggestion skipped", "error", err)
		}
	}

	units := git.ExtractCodeUnits(diff)

	promptContext := buildRepoContext(ctx, app)
	promptContext = joinContexts(promptContext, buildHistoryContext(ctx, app, diff))
	promptContext = joinContexts(promptContext, buildOperationContext(op))

	// When git is mid-operation it has already prepared the message that
	// preserves (merge, cherry-pick, revert, rebase, fixup) or combines
	// (squash) prior intent. Present that message instead of generating a
	// normal standalone one; the review loop can still regenerate with the
	// operation context above.
	prepared := ""
	if op != git.OperationNone {
		prepared = git.PreparedMessage(ctx, op)
	}

	return interactiveCommit(ctx, cmd, app, diff, promptContext, units, op, prepared)
}

// buildOperationContext returns a prompt fragment describing the in-progress
// git operation, or "" for an ordinary commit. It lets a regenerated message
// (or any generation that happens mid-operation) respect what git is asking
// for instead of producing a standalone message out of context.
func buildOperationContext(op git.Operation) string {
	switch op {
	case git.OperationMerge:
		return "Git operation in progress: merge. This commit completes an in-progress merge; " +
			"it should summarise the merge (for example \"Merge branch 'X' into Y\") " +
			"and any conflict resolution."
	case git.OperationCherryPick:
		return "Git operation in progress: cherry-pick. This commit re-applies an existing commit; " +
			"preserve the original commit's subject and intent."
	case git.OperationRevert:
		return "Git operation in progress: revert. This commit reverts an earlier change; " +
			"the message should identify the commit being reverted."
	case git.OperationRebase:
		return "Git operation in progress: rebase. This commit continues a rebase by re-applying " +
			"a commit; preserve the original commit's message and intent."
	case git.OperationSquash:
		return "Git operation in progress: squash. This commit combines several commits into one; " +
			"the message should merge the subjects and intent of the combined commits."
	case git.OperationFixup:
		return "Git operation in progress: fixup. This commit folds changes into an earlier commit; " +
			"preserve the target commit's message and intent."
	}

	return ""
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

// buildHistoryContext returns commit-history context for the prompt, or empty
// string if --history is disabled or no history is available.
//
// History is graph-relative when possible: with a configured upstream it lists
// commits on the current topic since divergence (merge-base .. HEAD), limited
// to the staged paths when a diff is available; without an upstream it falls
// back to the n most recent commits. Errors are logged at debug level and
// silently discarded — history is optional context and should never block
// generation.
func buildHistoryContext(ctx context.Context, app *AppContext, diff string) string {
	n := app.Config().HistoryValue()
	if n <= 0 {
		return ""
	}

	if upstream := git.GetUpstreamBranch(ctx); upstream != "" {
		paths := git.ExtractChangedPaths(diff)
		history, err := git.GetTopicHistory(ctx, upstream, n, paths)
		if err == nil && strings.TrimSpace(history) != "" {
			label := fmt.Sprintf("Commits on %s since diverging from %s:", app.Branch(ctx), upstream)
			if len(paths) > 0 {
				label += " (staged files)"
			}

			return label + "\n" + strings.TrimRight(history, "\n")
		}
		// No divergence yet or upstream unreachable — fall back to recent
		// commits below.
	}

	history, err := git.GetRecentCommits(ctx, n)
	if err != nil {
		slog.Debug("failed to get recent commits, proceeding without history", "error", err)

		return ""
	}
	if strings.TrimSpace(history) == "" {
		return ""
	}

	return "Recent commits:\n" + strings.TrimRight(history, "\n")
}
