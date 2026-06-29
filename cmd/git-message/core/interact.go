package core

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gud/internal/git"
	"gud/internal/helixdb"
	"gud/internal/request"

	"github.com/spf13/cobra"
)

const (
	actionCommit     = "commit"
	actionEdit       = "edit"
	actionRegenerate = "regenerate"
	actionAbort      = "abort"
)

// interactiveCommit runs the generate → review → commit loop.
func interactiveCommit(ctx context.Context, cmd *cobra.Command, app *AppContext,
	diff, promptContext string, units []git.CodeUnit) error {
	scanner := bufio.NewScanner(cmd.InOrStdin())
	out := cmd.OutOrStdout()

	for {
		cfg := app.Config()
		client := app.Client()
		profileContent := resolveProfileContent(string(cfg.Profile))
		msg, err := showProgress("Rolling in, obscuring the landscape of the codebase...", func() (string, error) {
			return client.GenerateCommitMessageWithContent(ctx, diff, promptContext, request.DetailLevel(cfg.DetailLevel),
				cfg.Hint, request.ProfileName(cfg.Profile), profileContent, cfg.WrapLine)
		})
		if err != nil {
			return fmt.Errorf("failed to generate commit message: %w", err)
		}

		_, _ = fmt.Fprintln(out, "")
		_, _ = fmt.Fprintln(out, msg)
		_, _ = fmt.Fprintln(out, "")

		action := promptAction(scanner, out)
		switch action {
		case actionCommit:
			msg = appendAssistedBy(msg, client.ModelName())
			hash, err := git.Commit(ctx, msg)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(out, "Committed successfully.")
			persistToHelixDB(ctx, app, diff, hash, msg, units)

			return nil

		case actionEdit:
			edited, err := editMessage(msg)
			if err != nil {
				return fmt.Errorf("failed to edit message: %w", err)
			}
			edited = appendAssistedBy(edited, client.ModelName())
			hash, err := git.Commit(ctx, edited)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(out, "Committed successfully.")
			persistToHelixDB(ctx, app, diff, hash, edited, units)

			return nil

		case actionRegenerate:
			continue

		case actionAbort:
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

// appendAssistedBy appends an "Assisted-by: <model>" git trailer to the message.
// It ensures a blank line separator before the trailer, following git trailer conventions.
// It is idempotent: calling it multiple times with the same model name does nothing.
func appendAssistedBy(msg, modelName string) string {
	trailer := "Assisted-by: " + modelName

	msg = strings.TrimRight(msg, "\n")

	// Already has this trailer — no-op aside from trailing newline.
	if strings.HasSuffix(msg, trailer) {
		return msg + "\n"
	}

	return msg + "\n\n" + trailer + "\n"
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

	//nolint:gosec // editor comes from user's $EDITOR env var, running their own CLI
	editCmd := exec.Command("sh", "-c", editor+" "+strconv.Quote(path))
	editCmd.Stdin = os.Stdin
	editCmd.Stdout = os.Stdout
	editCmd.Stderr = os.Stderr

	if err := editCmd.Run(); err != nil {
		return "", fmt.Errorf("editor failed: %w", err)
	}

	//nolint:gosec // path is constructed from os.MkdirTemp, not user input
	edited, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read edited file: %w", err)
	}

	return strings.TrimSpace(string(edited)), nil
}

// persistToHelixDB persists the commit data to HelixDB after a successful commit.
// Errors are logged and silently discarded — HelixDB persistence is fire-and-forget.
func persistToHelixDB(ctx context.Context, app *AppContext, diff, hash, message string, units []git.CodeUnit) {
	db := app.HelixDB()
	if db == nil || !db.Enabled() || !db.IsAvailable(ctx) {
		return
	}

	repoPath, err := git.GetRepoRoot(ctx)
	if err != nil || repoPath == "" {
		slog.Debug("helixdb: failed to get repo root for persistence", "error", err)
		return
	}

	author := git.GetAuthor(ctx)

	var fileChanges []helixdb.FileChange
	for _, u := range units {
		existing := false
		for i := range fileChanges {
			if fileChanges[i].Path == u.FilePath {
				existing = true
				break
			}
		}
		if !existing {
			fileChanges = append(fileChanges, helixdb.FileChange{
				Path:       u.FilePath,
				ChangeType: u.ChangeType,
			})
		}
	}

	commit := helixdb.CommitData{
		SHA:            hash,
		RepoPath:       repoPath,
		Message:        message,
		DiffText:       diff,
		Author:         author,
		Timestamp:      time.Now(),
		Files:          fileChanges,
		IsGudGenerated: true,
	}

	query := helixdb.BuildPersistCommitQuery(commit)
	var resp map[string]any
	if err := db.Exec(ctx, query, &resp); err != nil {
		slog.Debug("helixdb: failed to persist commit", "error", err)
	}
}
