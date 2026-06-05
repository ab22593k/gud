package core

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gud/internal/git"
	"gud/internal/request"

	"github.com/spf13/cobra"
)

// interactiveCommit runs the generate → review → commit loop.
func interactiveCommit(ctx context.Context, cmd *cobra.Command, client *request.Client, diff, promptContext string, cfg Config) error {
	scanner := bufio.NewScanner(cmd.InOrStdin())
	out := cmd.OutOrStdout()

	for {
		msg, err := showProgress("Rolling in, obscuring the landscape of the codebase...", func() (string, error) {
			return client.GenerateCommitMessage(ctx, diff, promptContext, cfg.DetailLevel, cfg.Hint, cfg.Persona)
		})
		if err != nil {
			return fmt.Errorf("failed to generate commit message: %w", err)
		}

		_, _ = fmt.Fprintln(out, "")
		_, _ = fmt.Fprintln(out, msg)
		_, _ = fmt.Fprintln(out, "")

		action := promptAction(scanner, out)
		switch action {
		case "commit":
			if err := git.Commit(ctx, msg); err != nil {
				return err
			}
			_, _ = fmt.Fprintln(out, "Committed successfully.")
			return nil

		case "edit":
			edited, err := editMessage(msg)
			if err != nil {
				return fmt.Errorf("failed to edit message: %w", err)
			}
			if err := git.Commit(ctx, edited); err != nil {
				return err
			}
			_, _ = fmt.Fprintln(out, "Committed successfully.")
			return nil

		case "regenerate":
			continue

		case "abort":
			_, _ = fmt.Fprintln(out, "Aborted.")
			return nil
		}
	}
}

// promptAction reads a single-line action from the user and returns the
// normalized action name. It loops until a valid action is entered.
func promptAction(scanner *bufio.Scanner, out io.Writer) string {
	for {
		_, _ = fmt.Fprint(out, "? Continue  [y]es  [r]egenerate  [e]dit  [a]bort  (default: yes): ")

		if !scanner.Scan() {
			return "abort"
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			return "commit"
		}

		switch strings.ToLower(input) {
		case "y", "yes", "commit", "c":
			return "commit"
		case "r", "regenerate":
			return "regenerate"
		case "e", "edit":
			return "edit"
		case "a", "abort", "q", "quit":
			return "abort"
		}
	}
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
	if err := os.WriteFile(path, []byte(msg), 0644); err != nil {
		return "", fmt.Errorf("failed to write temp file: %w", err)
	}

	editCmd := exec.Command(editor, path)
	editCmd.Stdin = os.Stdin
	editCmd.Stdout = os.Stdout
	editCmd.Stderr = os.Stderr

	if err := editCmd.Run(); err != nil {
		return "", fmt.Errorf("editor failed: %w", err)
	}

	edited, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read edited file: %w", err)
	}

	return strings.TrimSpace(string(edited)), nil
}
