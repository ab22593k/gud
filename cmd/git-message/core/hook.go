package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gud/internal/git"
	"gud/internal/request"

	"github.com/spf13/cobra"
)

var hookCmd = &cobra.Command{
	Use:   "hook",
	Short: "Manage git hooks for automatic commit message generation",
	Long: `Manage git hooks that automatically generate commit messages
when you run 'git commit'.

Install a prepare-commit-msg hook to generate commit messages automatically
from your staged changes.`,
}

var hookInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install the git prepare-commit-msg hook",
	RunE: func(cmd *cobra.Command, args []string) error {
		global, _ := cmd.Flags().GetBool("global")
		return runHookInstall(global)
	},
}

var hookUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Uninstall the git prepare-commit-msg hook",
	RunE: func(cmd *cobra.Command, args []string) error {
		global, _ := cmd.Flags().GetBool("global")
		return runHookUninstall(global)
	},
}

var hookRunCmd = &cobra.Command{
	Use:   "run <msg-file>",
	Short: "Generate a commit message and write it to the message file",
	Long: `Run the hook mode: generates a commit message from staged changes
and writes it to the specified message file.

This is used internally by the git hook and should not be called directly.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runHookMode(cmd, args[0])
	},
}

func runHookInstall(global bool) error {
	hookDir, err := git.GetHookDir(global)
	if err != nil {
		return fmt.Errorf("failed to get hook directory: %w", err)
	}

	if err := os.MkdirAll(hookDir, 0755); err != nil {
		return fmt.Errorf("failed to create hook directory: %w", err)
	}

	if err := git.InstallHook(hookDir, git.PrepareCommitMsg); err != nil {
		return fmt.Errorf("failed to install hook: %w", err)
	}

	fmt.Printf("Hook installed to %s\n", filepath.Join(hookDir, string(git.PrepareCommitMsg)))
	return nil
}

func runHookUninstall(global bool) error {
	hookDir, err := git.GetHookDir(global)
	if err != nil {
		return fmt.Errorf("failed to get hook directory: %w", err)
	}

	if err := git.UninstallHook(hookDir, git.PrepareCommitMsg); err != nil {
		return fmt.Errorf("failed to uninstall hook: %w", err)
	}

	fmt.Println("Hook uninstalled")
	return nil
}

func runHookMode(cmd *cobra.Command, msgFile string) error {
	cfg := configFromCmd(cmd)

	ctx := context.Background()
	return runHookModeInternal(ctx, msgFile, cfg.APIKey, cfg.Model, cfg.Temperature, cfg.DetailLevel, cfg.Hint, cfg.Persona)
}

// runHookModeInternal generates a commit message and writes it to the message file.
func runHookModeInternal(ctx context.Context, msgFile, apiKey, model string, temperature float64, detailLevel request.DetailLevel, hint string, persona request.PersonaName) error {
	diff, err := getStagedDiffOrSkip(ctx)
	if err != nil {
		return fmt.Errorf("failed to get staged diff: %w", err)
	}
	if diff == "" {
		return nil
	}

	client, err := request.NewClient(ctx, apiKey, model, temperature)
	if err != nil {
		return fmt.Errorf("failed to create request client: %w", err)
	}

	return generateAndWriteMsg(client, ctx, diff, detailLevel, hint, persona, msgFile)
}

// getStagedDiffOrSkip returns the staged diff, or an empty string if there are no staged changes.
func getStagedDiffOrSkip(ctx context.Context) (string, error) {
	diff, err := git.GetStagedDiff(ctx)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(diff) == "" {
		return "", nil
	}
	return diff, nil
}

// generateAndWriteMsg generates a commit message and writes it to the message file.
func generateAndWriteMsg(client *request.Client, ctx context.Context, diff string, detailLevel request.DetailLevel, hint string, persona request.PersonaName, msgFile string) error {
	msg, err := client.GenerateCommitMessage(ctx, diff, "", detailLevel, hint, persona)
	if err != nil {
		return fmt.Errorf("failed to generate commit message: %w", err)
	}

	if err := os.WriteFile(msgFile, []byte(msg), 0644); err != nil {
		return fmt.Errorf("failed to write message file: %w", err)
	}
	return nil
}

func init() {
	hookInstallCmd.Flags().Bool("global", false, "Use global hooks directory")
	hookUninstallCmd.Flags().Bool("global", false, "Use global hooks directory")

	hookCmd.AddCommand(hookInstallCmd)
	hookCmd.AddCommand(hookUninstallCmd)
	hookCmd.AddCommand(hookRunCmd)
	rootCmd.AddCommand(hookCmd)
}
