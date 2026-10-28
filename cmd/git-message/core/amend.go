package core

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"gud/internal/git"
	"gud/internal/obs"
	"gud/internal/request"

	"github.com/spf13/cobra"
)

// amendHeadRev is the default amend target for a bare --amend.
const amendHeadRev = "HEAD~1"

// amendFlag is the amend flag in --flag form, shared by registration,
// arg normalization, and tests.
const amendFlag = "--amend"

// amendTarget reads the --amend flag and positional args. The second return
// reports whether amend mode was requested. Forms supported:
// --amend (bare) → HEAD~1; --amend HEAD~2 (space) → HEAD~2 via positional;
// --amend=HEAD~2 → HEAD~2 (explicit value always wins, except an explicit
// =HEAD combined with a positional, which resolves the positional — the
// review loop always prints the resolved SHA before acting).
func amendTarget(cmd *cobra.Command, args []string) (string, bool) {
	flags := cmd.Flags()

	if !flags.Changed("amend") {
		return "", false
	}

	rev, err := flags.GetString("amend")
	if err != nil {
		return amendHeadRev, true
	}

	if rev == "" || (rev == amendHeadRev && len(args) > 0) {
		if len(args) > 0 {
			return args[0], true
		}

		return amendHeadRev, true
	}

	return rev, true
}

// isHeadCommit reports whether sha is the current HEAD. Any error means false.
func isHeadCommit(ctx context.Context, sha string) bool {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")

	var out bytes.Buffer

	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		return false
	}

	return strings.TrimSpace(out.String()) == sha
}

// amendJob is one resolved amend request: the target SHA, its patch, its old
// message, the repo-relative paths git reports for the target's own patch,
// and whether the target is HEAD (amended in place) or an older
// commit (reworded through a rebase).
type amendJob struct {
	sha    string
	diff   string
	paths  []string
	oldMsg string
	head   bool
}

// resolveAmendCommit validates rev and loads everything the review loop needs.
// The diff is the target commit's own patch, never a range.
func resolveAmendCommit(ctx context.Context, app *AppContext, rev string) (amendJob, error) {
	var job amendJob

	if op := app.Operation(ctx); op != git.OperationNone {
		return job, fmt.Errorf("cannot amend during an in-progress %s; finish it first", op)
	}

	// FR-029 (D8): a repository with no commits has no revision to amend.
	// The refusal sits before resolution because the default target is
	// HEAD~1: resolving it first would surface git's
	// "fatal: Needed a single revision" instead of the reason. A probe
	// error falls through to the existing resolution path rather than
	// asserting an absence the probe could not establish.
	has, err := git.HasCommits(ctx)
	if err == nil && !has {
		return job, errors.New("cannot amend: the repository has no commits yet")
	}

	sha, err := git.ResolveRevision(ctx, rev)
	if err != nil {
		return job, err
	}

	// FR-029: the root commit is refused for every amend target — HEAD as
	// well as any revision — before any diff or message work begins.
	root, err := git.IsRootCommit(ctx, sha)
	if err != nil {
		return job, err
	}

	if root {
		return job, fmt.Errorf("cannot amend %s: it is the root commit, which has no parent", rev)
	}

	if err := rejectUnamendableCommit(ctx, rev, sha); err != nil {
		return job, err
	}

	head := isHeadCommit(ctx, sha)

	if !head {
		if err := requireCleanTree(ctx, rev); err != nil {
			return job, err
		}
	}

	diff, err := git.GetCommitDiff(ctx, sha)
	if err != nil {
		return job, err
	}

	if strings.TrimSpace(diff) == "" {
		return job, fmt.Errorf("cannot amend %s: commit has no changes", rev)
	}

	oldMsg, err := git.GetCommitMessage(ctx, sha)
	if err != nil {
		return job, err
	}

	job = amendJob{
		sha:    sha,
		diff:   diff,
		paths:  resolveAmendPaths(ctx, app, sha),
		oldMsg: oldMsg,
		head:   head,
	}

	return job, nil
}

// resolveAmendPaths resolves the target commit's own change paths once for
// file-grounding (D9). The probe is optional evidence: on failure the
// diff-derived set applies and the amend proceeds.
func resolveAmendPaths(ctx context.Context, app *AppContext, sha string) []string {
	return resolveChangePaths(func() ([]string, error) { return app.commitChangePaths(ctx, sha) })
}

// rejectUnamendableCommit refuses merge commits for every amend target
// before any diff or message work begins.
func rejectUnamendableCommit(ctx context.Context, rev, sha string) error {
	merge, err := git.IsMergeCommit(ctx, sha)
	if err != nil {
		return err
	}

	if merge {
		return fmt.Errorf("cannot amend %s: merge commits are not supported", rev)
	}

	return nil
}

// requireCleanTree refuses a non-HEAD amend target when the working tree
// has staged or unstaged changes the reword would have to carry.
func requireCleanTree(ctx context.Context, rev string) error {
	clean, err := git.IsCleanTree(ctx)
	if err != nil {
		return err
	}

	if !clean {
		return fmt.Errorf("cannot amend %s: working tree has staged or unstaged changes", rev)
	}

	return nil
}

// runAmendFlow regenerates the message for exactly one commit and amends it.
func runAmendFlow(ctx context.Context, cmd *cobra.Command, app *AppContext, rev string) error {
	job, err := resolveAmendCommit(ctx, app, rev)
	if err != nil {
		return err
	}

	if err := app.InitClient(ctx); err != nil {
		return err
	}

	promptContext := buildPromptContextParallel(ctx, app, job.diff, git.OperationNone)
	obs.LogSizes(len(job.diff), len(promptContext))

	out := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(out, "Amending %s (old: %s)\n", shortSHA(job.sha), firstLine(job.oldMsg))

	scanner := bufio.NewScanner(cmd.InOrStdin())

	return amendReviewLoop(ctx, cmd, app, scanner, out, job, promptContext)
}

// amendReviewLoop runs the generate → review cycle for one amend job until the
// user commits, edits, regenerates, or aborts.
func amendReviewLoop(ctx context.Context, cmd *cobra.Command, app *AppContext,
	scanner *bufio.Scanner, out io.Writer, job amendJob, promptContext string,
) error {
	// D9: the target commit's records are resolved once in resolveAmendCommit
	// and reused across every regeneration of this review loop (FR-034).
	change := request.Change{Diff: job.diff, Paths: job.paths}

	for {
		msg, _, findings, err := loopMessage(ctx, cmd.ErrOrStderr(), app, change, promptContext, "")
		if err != nil {
			return err
		}

		presentFindings(out, findings)

		action, edited := reviewMessage(cmd, scanner, out, msg, app.Config().WrapLine)

		if action == actionCommit && edited != "" {
			msg = edited
		}

		switch action {
		case actionCommit:
			return applyAmendedMessage(ctx, out, msg, job.head, job.sha)
		case actionEdit:
			edited, err := editMessage(msg)
			if err != nil {
				return fmt.Errorf("failed to edit message: %w", err)
			}

			model := ""

			if c := app.Client(); c != nil {
				model = c.ModelName()
			}

			edited, err = finalizeMessage(ctx, edited, app.Config(), model)
			if err != nil {
				return err
			}

			return applyAmendedMessage(ctx, out, edited, job.head, job.sha)
		case actionRegenerate:
			continue
		case actionAbort:
			_, _ = fmt.Fprintln(out, "Aborted.")

			return nil
		}
	}
}

// applyAmendedMessage writes msg onto HEAD or rewrites an older commit.
func applyAmendedMessage(ctx context.Context, out io.Writer, msg string, head bool, sha string) error {
	if head {
		if _, err := git.AmendHead(ctx, msg); err != nil {
			return err
		}
	} else if err := git.RewordCommit(ctx, sha, msg); err != nil {
		return err
	}

	_, _ = fmt.Fprintln(out, "Amended successfully.")

	return nil
}

// shortSHA abbreviates a full SHA for display.
func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}

	return sha
}

// firstLine returns the first line of s.
func firstLine(s string) string {
	if before, _, ok := strings.Cut(s, "\n"); ok {
		return strings.TrimSpace(before)
	}

	return strings.TrimSpace(s)
}
