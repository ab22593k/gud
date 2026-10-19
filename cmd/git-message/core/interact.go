package core

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gud/internal/config"
	"gud/internal/git"
	"gud/internal/request"
	"gud/internal/textwrap"
	"gud/internal/tui"

	"github.com/spf13/cobra"
)

const (
	actionCommit     = "commit"
	actionEdit       = "edit"
	actionRegenerate = "regenerate"
	actionAbort      = "abort"
)

// interactiveCommit runs the generate → review → commit loop. When prepared is
// non-empty (a git operation is in progress and git already drafted a message)
// the first pass presents it instead of generating a fresh standalone message.
func interactiveCommit(
	ctx context.Context,
	cmd *cobra.Command,
	app *AppContext,
	diff string,
	promptContext string,
	op git.Operation,
	prepared string,
) error {
	if prepared != "" {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(),
			"Git %s in progress — using git's prepared message (press [r] to regenerate).\n", op)
	}

	return reviewLoop(ctx, cmd, app, reviewRequest{
		diff:          diff,
		promptContext: promptContext,
		prepared:      prepared,
		// Without a client there is nothing to regenerate from; the review
		// loop still lets the user keep git's prepared message or edit it.
		canRegenerate: app.Client() != nil,
		commit:        commitFinalized,
	})
}

// reviewRequest is one generate → review → commit session. commit performs
// the terminal action (commit the message as-is, or amend a target commit);
// canRegenerate reports whether a client is available to regenerate with.
type reviewRequest struct {
	diff          string
	promptContext string
	prepared      string
	canRegenerate bool
	commit        func(ctx context.Context, out io.Writer, msg string) error
}

// reviewLoop drives the message review until the user commits, edits,
// regenerates, or aborts. It is shared by the ordinary commit flow and the
// amend flow; only the terminal commit action and the regenerate guard differ.
func reviewLoop(ctx context.Context, cmd *cobra.Command, app *AppContext, req reviewRequest) error {
	scanner := bufio.NewScanner(cmd.InOrStdin())
	out := cmd.OutOrStdout()
	prepared := req.prepared

	for {
		cfg := app.Config()
		client := app.Client()

		msg, preserved, findings, err := loopMessage(ctx, cmd.ErrOrStderr(), app, req.diff, req.promptContext, prepared)
		if err != nil {
			return err
		}

		// FR-018: a result served with checks still failing is presented
		// together with that fact, never as a clean success.
		presentFindings(out, findings)

		prepared = ""

		action, edited := reviewMessage(cmd, scanner, out, msg, cfg.WrapLine)
		if action == actionCommit && edited != "" {
			msg = edited
		}

		switch action {
		case actionCommit:
			return req.commit(ctx, out, msg)

		case actionEdit:
			edited, err := editMessage(msg)
			if err != nil {
				return fmt.Errorf("failed to edit message: %w", err)
			}
			// A preserved (git-drafted) message that the user then edits is
			// still not AI-generated, so no Assisted-by trailer is added.
			model := ""
			if !preserved && client != nil {
				model = client.ModelName()
			}

			edited, err = finalizeMessage(ctx, edited, cfg, model)
			if err != nil {
				return err
			}

			return req.commit(ctx, out, edited)

		case actionRegenerate:
			if !req.canRegenerate {
				_, _ = fmt.Fprintln(out, "Regeneration requires GOOGLE_API_KEY — aborting; git's prepared message is unchanged.")

				return nil
			}

			continue

		case actionAbort:
			_, _ = fmt.Fprintln(out, "Aborted.")

			return nil
		}
	}
}

// loopMessage produces the message for one review-loop iteration. On the
// first pass of an operation stop it returns git's prepared message with
// trailers assembled; otherwise it generates a fresh one. The second return
// marks the message as git's own (not AI-generated) so the edit path skips
// the Assisted-by trailer for it. The findings are the verification checks
// still failing after the bounded regeneration (nil when clean or when the
// message is git's own); the caller presents them next to the result.
func loopMessage(
	ctx context.Context,
	errW io.Writer,
	app *AppContext,
	diff string,
	promptContext string,
	prepared string,
) (msg string, preserved bool, findings []request.Finding, err error) {
	preserved = prepared != ""
	if !preserved {
		msg, findings, err = generateCommitMessage(ctx, errW, app, diff, promptContext)

		return msg, false, findings, err
	}

	msg, err = finalizeMessage(ctx, prepared, app.Config(), "")

	return msg, true, nil, err
}

// generateCommitMessage runs the model and returns the message with the
// Assisted-by and issue trailers applied, alongside the residual verification
// findings. It errors when no request client is available (no API key
// configured) so a preserved git message can be used without one.
func generateCommitMessage(
	ctx context.Context, errW io.Writer, app *AppContext, diff, promptContext string,
) (string, []request.Finding, error) {
	if app.Client() == nil {
		return "", nil, errors.New("failed to generate commit message: request client is unavailable (set GOOGLE_API_KEY)")
	}

	cfg := app.Config()
	mounts := app.AgentFiles(ctx)

	var findings []request.Finding

	msg, err := showProgress(ctx, errW, "Rolling in, obscuring the landscape of the codebase...", func() (string, error) {
		msg, f, err := app.Client().GenerateCommitMessageWithContent(ctx, diff, promptContext, cfg.DetailLevel,
			cfg.Hint, mounts, cfg.WrapLine)
		findings = f

		return msg, err
	})
	if err != nil {
		return "", nil, fmt.Errorf("failed to generate commit message: %w", err)
	}

	finalized, err := finalizeMessage(ctx, msg, cfg, app.Client().ModelName())
	if err != nil {
		return "", nil, err
	}

	return finalized, findings, nil
}

// presentFindings prints the FR-018 residual-findings notice: one line naming
// the verification checks still failing. A clean result prints nothing. It
// cannot fail — the notice informs, it never errors or changes the exit
// status.
func presentFindings(out io.Writer, findings []request.Finding) {
	if len(findings) == 0 {
		return
	}

	codes := make([]string, 0, len(findings))
	for _, f := range findings {
		codes = append(codes, f.Check)
	}

	_, _ = fmt.Fprintf(out, "Note: presented with verification still failing (%s).\n", strings.Join(codes, ", "))
}

// reviewMessage runs the commit-message review: the TUI when both stdin and
// stdout are terminals, the plain-text prompt otherwise. It returns the
// chosen action and any inline edit the TUI produced.
func reviewMessage(cmd *cobra.Command, scanner *bufio.Scanner, out io.Writer,
	msg string, wrapLine int) (action, edited string) {
	if isInteractive(cmd.InOrStdin(), out) {
		var err error

		action, edited, err = tui.RunCommitReview(msg, wrapLine)
		if err != nil {
			return actionAbort, ""
		}

		return action, edited
	}

	return promptAction(scanner, out), ""
}

// commitFinalized runs the git commit and reports success.
// Shared by the direct and edited commit paths.
func commitFinalized(ctx context.Context, out io.Writer, msg string) error {
	if _, err := git.Commit(ctx, msg); err != nil {
		return err
	}

	_, _ = fmt.Fprintln(out, "Committed successfully.")

	return nil
}

// promptAction reads a single-line action from the user and returns the
// normalized action name. It loops until a valid action is entered.
func promptAction(scanner *bufio.Scanner, out io.Writer) string {
	for {
		_, _ = fmt.Fprint(out, "? Continue  [y]es  [r]egenerate  [e]dit  [a]bort  (default: yes): ")

		if !scanner.Scan() {
			return actionAbort
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			return actionCommit
		}

		switch strings.ToLower(input) {
		case "y", "yes", "commit", "c":
			return actionCommit
		case "r", "regenerate":
			return actionRegenerate
		case "e", "edit":
			return actionEdit
		case "a", "abort", "q", "quit":
			return actionAbort
		}
	}
}

// assembleTrailers applies the Fixes trailers (one per issue, in flag order)
// and the optional Assisted-by trailer through git's own interpret-trailers
// parser, so ordering, deduplication, separators, and messages without a body
// all follow git's canonical rules instead of string heuristics. An empty
// model omits the Assisted-by trailer (e.g. for a preserved git message).
func assembleTrailers(ctx context.Context, msg string, issues []int, model string) (string, error) {
	var trailers []git.Trailer

	for _, n := range issues {
		if n > 0 {
			trailers = append(trailers, git.Trailer{Key: "Fixes", Value: fmt.Sprintf("#%d", n)})
		}
	}

	if model != "" {
		trailers = append(trailers, git.Trailer{Key: "Assisted-by", Value: model})
	}

	return git.AppendTrailers(ctx, msg, trailers)
}

// finalizeMessage is the single finalization choke point for every
// presentation path (FR-013): trailers are assembled first via git's own
// rules, then every line — subject, body, and trailers — is normalized to
// the configured width in display cells. Wrapping folds; it never truncates
// content or fails. An empty model omits the Assisted-by trailer (FR-016),
// e.g. for a git-authored message.
func finalizeMessage(ctx context.Context, msg string, cfg config.Config, model string) (string, error) {
	withTrailers, err := assembleTrailers(ctx, msg, cfg.Issues, model)
	if err != nil {
		return "", err
	}

	return textwrap.Wrap(withTrailers, cfg.WrapLine), nil
}

// editMessage opens the user's $EDITOR with the given message content,
// and returns the edited content.
func editMessage(msg string) (string, error) {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	dir, err := os.MkdirTemp("", "gud-commit-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	path := filepath.Join(dir, "COMMIT_EDITMSG")
	if err := os.WriteFile(path, []byte(msg), 0600); err != nil {
		return "", fmt.Errorf("failed to write temp file: %w", err)
	}

	fields := strings.Fields(editor)
	if len(fields) == 0 {
		return "", fmt.Errorf("editor: empty EDITOR value")
	}

	if strings.ContainsAny(fields[0], ";|&$`\\()<>\"'") {
		return "", fmt.Errorf("editor: EDITOR binary %q contains shell metacharacters", fields[0])
	}

	if _, err := exec.LookPath(fields[0]); err != nil {
		return "", fmt.Errorf("editor: EDITOR binary %q not found: %w", fields[0], err)
	}

	//nolint:noctx // interactive editor session must not be killed by cancellation
	editCmd := exec.Command(fields[0], append(fields[1:], path)...) //nolint:gosec
	editCmd.Stdin = os.Stdin
	editCmd.Stdout = os.Stdout
	editCmd.Stderr = os.Stderr

	if err := editCmd.Run(); err != nil {
		return "", fmt.Errorf("editor failed: %w", err)
	}

	edited, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return "", fmt.Errorf("failed to read edited file: %w", err)
	}

	return strings.TrimSpace(string(edited)), nil
}
