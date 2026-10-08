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

	"gud/internal/git"
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
func interactiveCommit(ctx context.Context, cmd *cobra.Command, app *AppContext,
	diff, promptContext string, op git.Operation, prepared string) error {
	scanner := bufio.NewScanner(cmd.InOrStdin())
	out := cmd.OutOrStdout()

	if prepared != "" {
		_, _ = fmt.Fprintf(out, "Git %s in progress — using git's prepared message (press [r] to regenerate).\n", op)
	}

	for {
		cfg := app.Config()
		client := app.Client()

		msg, preserved, err := loopMessage(ctx, app, diff, promptContext, prepared)
		if err != nil {
			return err
		}

		prepared = ""

		action, edited := reviewMessage(cmd, scanner, out, msg, cfg.WrapLine)
		if action == actionCommit && edited != "" {
			msg = edited
		}

		switch action {
		case actionCommit:
			return commitFinalized(ctx, out, msg)

		case actionEdit:
			edited, err := editMessage(msg)
			if err != nil {
				return fmt.Errorf("failed to edit message: %w", err)
			}
			// A preserved (git-drafted) message that the user then edits is
			// still not AI-generated, so no Assisted-by trailer is added.
			model := ""
			if !preserved {
				model = client.ModelName()
			}

			edited, err = assembleTrailers(ctx, edited, cfg.Issues, model)
			if err != nil {
				return err
			}

			return commitFinalized(ctx, out, edited)

		case actionRegenerate:
			if client == nil {
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
// the Assisted-by trailer for it.
func loopMessage(
	ctx context.Context, app *AppContext, diff, promptContext, prepared string,
) (msg string, preserved bool, err error) {
	preserved = prepared != ""
	if !preserved {
		msg, err = generateCommitMessage(ctx, app, diff, promptContext)

		return msg, false, err
	}

	msg, err = assembleTrailers(ctx, prepared, app.Config().Issues, "")

	return msg, true, err
}

// generateCommitMessage runs the model and returns the message with the
// Assisted-by and issue trailers applied. It errors when no request client is
// available (no API key configured) so a preserved git message can be used
// without one.
func generateCommitMessage(ctx context.Context, app *AppContext, diff, promptContext string) (string, error) {
	if app.Client() == nil {
		return "", errors.New("failed to generate commit message: request client is unavailable (set GOOGLE_API_KEY)")
	}

	cfg := app.Config()
	mounts := app.AgentFiles(ctx)

	msg, err := showProgress(ctx, "Rolling in, obscuring the landscape of the codebase...", func() (string, error) {
		return app.Client().GenerateCommitMessageWithContent(ctx, diff, promptContext, cfg.DetailLevel,
			cfg.Hint, mounts, cfg.WrapLine)
	})
	if err != nil {
		return "", fmt.Errorf("failed to generate commit message: %w", err)
	}

	return assembleTrailers(ctx, msg, cfg.Issues, app.Client().ModelName())
}

// reviewMessage runs the commit-message review: the TUI in a terminal, the
// plain-text prompt otherwise. It returns the chosen action and any inline
// edit the TUI produced.
func reviewMessage(cmd *cobra.Command, scanner *bufio.Scanner, out io.Writer,
	msg string, wrapLine int) (action, edited string) {
	if file, ok := cmd.InOrStdin().(*os.File); ok && isTerminal(file) {
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

	//nolint:noctx // interactive editor session must not be killed by cancellation
	editCmd := exec.Command(editor /* user's $EDITOR */, path) //nolint:gosec
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
