package core

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"gud/internal/git"
	"gud/internal/request"

	"github.com/spf13/cobra"
)

// runGenerate is the default action: generate a commit message from staged changes.
func runGenerate(cmd *cobra.Command, args []string) error {
	cfg := configFromCmd(cmd)

	if cfg.APIKey == "" {
		return errors.New("API key is required. Set the GOOGLE_API_KEY environment variable")
	}

	ctx := context.Background()

	diff, err := getStagedDiffOrError(ctx)
	if err != nil {
		return err
	}

	promptContext := buildHistoryContext(ctx, cfg)

	client, err := request.NewClient(ctx, cfg.APIKey, cfg.Model, cfg.Temperature)
	if err != nil {
		return fmt.Errorf("failed to create request client: %w", err)
	}

	return interactiveCommit(ctx, cmd, client, diff, promptContext, cfg)
}

// getStagedDiffOrError retrieves the staged diff and returns an error if none exists.
func getStagedDiffOrError(ctx context.Context) (string, error) {
	diff, err := git.GetStagedDiff(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get staged diff: %w", err)
	}
	if strings.TrimSpace(diff) == "" {
		return "", errors.New("no staged changes found. Use 'git add' to stage changes")
	}

	return diff, nil
}

// buildHistoryContext returns a formatted string of recent commit history, or
// empty string if --history is disabled or there are no recent commits.
// Errors from git are logged at debug level and silently discarded —
// history is optional context for the AI prompt and should never block generation.
func buildHistoryContext(ctx context.Context, cfg Config) string {
	var history string
	if cfg.History > 0 {
		var err error
		history, err = git.GetRecentCommits(ctx, cfg.History)
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

// maxHistory is the maximum number of recent commits the --history flag can request.
// This prevents accidentally dumping hundreds of commits into the prompt and wasting tokens.
const maxHistory = 50

// validateConfig normalizes and validates the shared configuration values.
func validateConfig(cfg Config) Config {
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

	return cfg
}
