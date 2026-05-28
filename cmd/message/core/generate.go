package cli

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

	msg, err := client.GenerateCommitMessage(ctx, diff, promptContext, cfg.DetailLevel, cfg.Hint, cfg.Persona)
	if err != nil {
		return fmt.Errorf("failed to generate commit message: %w", err)
	}

	cmd.Println(msg)
	return nil
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
