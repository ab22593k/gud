package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"gud/internal/git"
	"gud/internal/request"
)

// runGenerate is the default action: generate a commit message from staged changes.
func runGenerate(cmd *cobra.Command, args []string) error {
	validateConfig()

	// Resolve API key and model from env if not provided via flag
	if cfg.APIKey == "" {
		cfg.APIKey = os.Getenv("GEMINI_API_KEY")
	}
	if cfg.Model == "" {
		cfg.Model = os.Getenv("GEMINI_MODEL")
	}

	if cfg.APIKey == "" {
		return fmt.Errorf("API key is required. Set GEMINI_API_KEY env or use --api-key flag")
	}

	diff, err := git.GetStagedDiff(context.Background())
	if err != nil {
		return fmt.Errorf("failed to get staged diff: %w", err)
	}

	if strings.TrimSpace(diff) == "" {
		return fmt.Errorf("no staged changes found. Use 'git add' to stage changes")
	}

	if cfg.DryRun {
		cmd.Println("=== Staged Diff ===")
		cmd.Println(diff)
		cmd.Println("=== End Diff ===")
		return nil
	}

	client, err := request.NewClient(cfg.APIKey, cfg.Model, cfg.Temperature)
	if err != nil {
		return fmt.Errorf("failed to create request client: %w", err)
	}

	msg, err := client.GenerateCommitMessage(context.Background(), diff, cfg.Context, cfg.DetailLevel, cfg.Hint, cfg.Persona)
	if err != nil {
		return fmt.Errorf("failed to generate commit message: %w", err)
	}

	cmd.Println(msg)
	return nil
}

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
}
