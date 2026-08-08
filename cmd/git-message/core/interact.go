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
	"strings"
	"time"

	"gud/internal/git"
	"gud/internal/mem"
	"gud/internal/request"
	"gud/internal/tui"

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
		msg, err := showProgress(ctx, "Rolling in, obscuring the landscape of the codebase...", func() (string, error) {
			return client.GenerateCommitMessageWithContent(ctx, diff, promptContext, request.DetailLevel(cfg.DetailLevel),
				cfg.Hint, request.ProfileName(cfg.Profile), profileContent, cfg.WrapLine)
		})
		if err != nil {
			return fmt.Errorf("failed to generate commit message: %w", err)
		}

		msg = appendAssistedBy(msg, client.ModelName())
		msg = appendIssues(msg, cfg.Issues)

		// Use TUI in terminal mode, fall back to text prompt otherwise
		var action string
		if file, ok := cmd.InOrStdin().(*os.File); ok && isTerminal(file) {
			var edited string
			action, edited, err = tui.RunCommitReview(msg, cfg.WrapLine)
			if err != nil {
				action = actionAbort
			}
			// If the user edited inline, use the TUI's version.
			if action == actionCommit && edited != "" {
				msg = edited
			}
		} else {
			action = promptAction(scanner, out)
		}
		switch action {
		case actionCommit:
			return commitFinalized(ctx, app, out, diff, msg, units)

		case actionEdit:
			edited, err := editMessage(msg)
			if err != nil {
				return fmt.Errorf("failed to edit message: %w", err)
			}
			edited = appendAssistedBy(edited, client.ModelName())
			edited = appendIssues(edited, cfg.Issues)

			return commitFinalized(ctx, app, out, diff, edited, units)

		case actionRegenerate:
			continue

		case actionAbort:
			_, _ = fmt.Fprintln(out, "Aborted.")

			return nil
		}
	}
}

// commitFinalized runs the git commit, reports success, and persists the
// commit to HelixDB. Shared by the direct and edited commit paths.
func commitFinalized(ctx context.Context, app *AppContext, out io.Writer,
	diff, msg string, units []git.CodeUnit) error {
	hash, err := git.Commit(ctx, msg)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintln(out, "Committed successfully.")
	persistToHelixDB(ctx, app, diff, hash, msg, units)

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

// appendIssues inserts one "Fixes: <issue>" git trailer per issue number just
// before the trailing "Assisted-by:" trailer, or after the body if that trailer
// is absent. It is a no-op for an empty list and idempotent: a "Fixes: #N"
// trailer already present is not duplicated.
func appendIssues(msg string, issues []int) string {
	if len(issues) == 0 {
		return msg
	}
	msg = strings.TrimRight(msg, "\n")

	// Collect the missing trailers, in flag order.
	block := make([]string, 0, len(issues))
	for _, n := range issues {
		if n <= 0 {
			continue
		}
		trailer := fmt.Sprintf("Fixes: #%d", n)
		if strings.HasSuffix(msg, trailer) || strings.Contains(msg, "\n"+trailer+"\n") {
			continue
		}
		block = append(block, trailer)
	}
	if len(block) == 0 {
		return msg + "\n"
	}

	trailers := strings.Join(block, "\n")

	// Insert just before the trailing "Assisted-by:" trailer. A blank line
	// separates the last "Fixes:" trailer from "Assisted-by:". When Fixes
	// trailers already precede it (e.g. from an editor pass), insert after
	// them instead of replacing their separator.
	const sep = "\n\nAssisted-by: "
	if idx := strings.LastIndex(msg, sep); idx >= 0 {
		return msg[:idx] + "\n\n" + trailers + "\n\n" + strings.TrimPrefix(msg[idx:], "\n\n") + "\n"
	}
	const lineSep = "\nAssisted-by: "
	if idx := strings.LastIndex(msg, lineSep); idx >= 0 {
		return msg[:idx] + "\n" + trailers + "\n\n" + strings.TrimPrefix(msg[idx:], "\n") + "\n"
	}

	return msg + "\n\n" + trailers + "\n"
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

	//nolint:gosec
	editCmd := exec.Command(editor /* user's $EDITOR */, path)
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

// toFileChanges deduplicates git.CodeUnit entries by FilePath and converts
// them to mem.FileChange for HelixDB persistence.
func toFileChanges(units []git.CodeUnit) []mem.FileChange {
	var fileChanges []mem.FileChange
	for _, u := range units {
		existing := false
		for i := range fileChanges {
			if fileChanges[i].Path == u.FilePath {
				existing = true

				break
			}
		}
		if !existing {
			fileChanges = append(fileChanges, mem.FileChange{
				Path:       u.FilePath,
				ChangeType: u.ChangeType,
			})
		}
	}

	return fileChanges
}

// toCodeUnitRefs maps parsed git code units to mem.CodeUnitRef for HelixDB
// persistence. They back the entity-aware recall path (MENTIONS edges).
func toCodeUnitRefs(units []git.CodeUnit) []mem.CodeUnitRef {
	refs := make([]mem.CodeUnitRef, 0, len(units))
	for _, u := range units {
		refs = append(refs, mem.CodeUnitRef{
			Name:       u.Name,
			Kind:       u.Kind,
			FilePath:   u.FilePath,
			ChangeType: u.ChangeType,
		})
	}

	return refs
}

// persistToHelixDB persists the commit data to HelixDB after a successful commit.
// Errors are logged and silently discarded — HelixDB persistence is fire-and-forget.
func persistToHelixDB(ctx context.Context, app *AppContext, diff, hash, message string, units []git.CodeUnit) {
	db := app.HelixDB()
	if db == nil || !db.Enabled() || !db.IsAvailable(ctx) {
		return
	}

	repoPath, err := app.RepoRoot(ctx)
	if err != nil || repoPath == "" {
		slog.Debug("helixdb: failed to get repo root for persistence", "error", err)

		return
	}

	author := git.GetAuthor(ctx)
	branch := git.GetBranch(ctx)

	fileChanges := toFileChanges(units)
	codeUnits := toCodeUnitRefs(units)

	commit := mem.CommitData{
		SHA:            hash,
		RepoPath:       repoPath,
		Branch:         branch,
		Message:        message,
		DiffText:       diff,
		Author:         author,
		Timestamp:      time.Now(),
		Files:          fileChanges,
		CodeUnits:      codeUnits,
		IsGudGenerated: true,
	}

	// Embed the diff so the hybrid (vector + BM25) recall path can find this
	// commit semantically. A failed embedding is non-fatal: the commit is
	// still persisted and remains findable via BM25 and MENTIONS edges.
	if client := app.Client(); client != nil {
		if vec, err := client.EmbedText(ctx, diff); err != nil {
			slog.Debug("helixdb: embedding failed, persisting without", "error", err)
		} else {
			commit.Embedding = vec
		}
	}

	query := mem.BuildPersistCommitQuery(commit)
	var rawResp map[string]any
	if err := db.Exec(ctx, query, &rawResp); err != nil {
		slog.Debug("helixdb: failed to persist commit", "error", err)
	}
}
