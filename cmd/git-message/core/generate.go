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
	"time"

	"gud/internal/git"
	"gud/internal/request"

	"github.com/spf13/cobra"
)

// runGenerate is the default action: generate a commit message from staged changes.
func runGenerate(cmd *cobra.Command, args []string) error {
	validateConfig()

	// Resolve API key and model from env if not provided via flag
	if cfg.APIKey == "" {
		cfg.APIKey = os.Getenv("GOOGLE_API_KEY")
	}
	if cfg.Model == "" {
		cfg.Model = os.Getenv("GEMINI_MODEL")
	}

	if cfg.APIKey == "" {
		return fmt.Errorf("API key is required. Set GOOGLE_API_KEY env or use --api-key flag")
	}

	ctx := context.Background()

	diff, err := git.GetStagedDiff(ctx)
	if err != nil {
		return fmt.Errorf("failed to get staged diff: %w", err)
	}

	if strings.TrimSpace(diff) == "" {
		return fmt.Errorf("no staged changes found. Use 'git add' to stage changes")
	}

	var commitHistory string
	if cfg.History > 0 {
		commitHistory = git.GetRecentCommits(ctx, cfg.History)
	}

	promptContext := cfg.Context
	if commitHistory != "" {
		if promptContext != "" {
			promptContext += "\n\n"
		}
		promptContext += "Recent commits:\n" + commitHistory
	}

	client, err := request.NewClient(cfg.APIKey, cfg.Model, cfg.Temperature)
	if err != nil {
		return fmt.Errorf("failed to create request client: %w", err)
	}

	return interactiveCommit(ctx, cmd, client, diff, promptContext)
}

// showProgress displays an animated spinner on stderr while fn executes.
// It returns fn's result values.
func showProgress[T any](msg string, fn func() (T, error)) (T, error) {
	type result struct {
		val T
		err error
	}
	resCh := make(chan result, 1)
	go func() {
		val, err := fn()
		resCh <- result{val, err}
	}()

	spinner := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	ticker := time.NewTicker(80 * time.Millisecond)
	defer ticker.Stop()

	i := 0
	for {
		select {
		case r := <-resCh:
			fmt.Fprintf(os.Stderr, "\r%s ✓\n", msg)
			return r.val, r.err
		case <-ticker.C:
			fmt.Fprintf(os.Stderr, "\r%s %s", spinner[i], msg)
			i = (i + 1) % len(spinner)
		}
	}
}

// interactiveCommit runs the generate → review → commit loop.
func interactiveCommit(ctx context.Context, cmd *cobra.Command, client *request.Client, diff, promptContext string) error {
	scanner := bufio.NewScanner(cmd.InOrStdin())
	out := cmd.OutOrStdout()

	for {
		msg, err := showProgress("Rolling in, obscuring the landscape of the codebase...", func() (string, error) {
			return client.GenerateCommitMessage(ctx, diff, promptContext, cfg.DetailLevel, cfg.Hint, cfg.Persona)
		})
		if err != nil {
			return fmt.Errorf("failed to generate commit message: %w", err)
		}

		fmt.Fprintln(out, "")
		fmt.Fprintln(out, msg)
		fmt.Fprintln(out, "")

		action := promptAction(scanner, out)
		switch action {
		case "commit":
			if err := git.Commit(ctx, msg); err != nil {
				return err
			}
			fmt.Fprintln(out, "Committed successfully.")
			return nil

		case "edit":
			edited, err := editMessage(msg)
			if err != nil {
				return fmt.Errorf("failed to edit message: %w", err)
			}
			if err := git.Commit(ctx, edited); err != nil {
				return err
			}
			fmt.Fprintln(out, "Committed successfully.")
			return nil

		case "regenerate":
			continue

		case "abort":
			fmt.Fprintln(out, "Aborted.")
			return nil
		}
	}
}

// promptAction reads a single-line action from the user and returns the
// normalized action name. It loops until a valid action is entered.
func promptAction(scanner *bufio.Scanner, out io.Writer) string {
	for {
		fmt.Fprint(out, "? Continue  [y]es  [r]egenerate  [e]dit  [a]bort  (default: yes): ")

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
	defer os.RemoveAll(dir)

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

// maxHistory is the maximum number of recent commits the --history flag can request.
// This prevents accidentally dumping hundreds of commits into the prompt and wasting tokens.
const maxHistory = 50

// validateConfig normalizes and validates the shared configuration values.
func validateConfig() {
	switch cfg.DetailLevel {
	case request.DetailMinimal, request.DetailStandard, request.DetailDetailed:
		// valid
	default:
		cfg.DetailLevel = request.DetailStandard
	}

	switch cfg.Persona {
	case request.PersonaEmbedded, request.PersonaConventional:
		// valid
	default:
		cfg.Persona = request.PersonaEmbedded
	}

	if cfg.History < 0 {
		cfg.History = 0
	} else if cfg.History > maxHistory {
		cfg.History = maxHistory
	}
}
