package core

import (
	"context"
	"fmt"
	"os"
	"strings"

	"gud/internal/git"
	"gud/internal/request"

	"github.com/spf13/cobra"
)

// runGenerate is the default action: generate a commit message from staged changes.
func runGenerate(cmd *cobra.Command, args []string) error {
	validateConfig()
	resolveEnvConfig()

	if cfg.APIKey == "" {
		return fmt.Errorf("API key is required. Set the GOOGLE_API_KEY environment variable")
	}

	ctx := context.Background()

	diff, err := getStagedDiffOrError(ctx)
	if err != nil {
		return err
	}

	promptContext := buildHistoryContext(ctx)

	client, err := request.NewClient(cfg.APIKey, cfg.Model, cfg.Temperature)
	if err != nil {
		return fmt.Errorf("failed to create request client: %w", err)
	}

	return interactiveCommit(ctx, cmd, client, diff, promptContext)
}

// resolveEnvConfig reads API key and model from environment variables.
func resolveEnvConfig() {
	cfg.APIKey = os.Getenv("GOOGLE_API_KEY")
	if cfg.Model == "" {
		cfg.Model = os.Getenv("GEMINI_MODEL")
	}
}

// getStagedDiffOrError retrieves the staged diff and returns an error if none exists.
func getStagedDiffOrError(ctx context.Context) (string, error) {
	diff, err := git.GetStagedDiff(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get staged diff: %w", err)
	}
	if strings.TrimSpace(diff) == "" {
		return "", fmt.Errorf("no staged changes found. Use 'git add' to stage changes")
	}
	return diff, nil
}

// buildHistoryContext returns a formatted string of recent commit history, or
// empty string if --history is disabled or there are no recent commits.
func buildHistoryContext(ctx context.Context) string {
	var history string
	if cfg.History > 0 {
		history = git.GetRecentCommits(ctx, cfg.History)
	}
	if history != "" {
		history = "Recent commits:\n" + history
	}
	return history
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
