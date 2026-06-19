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
	RunE: func(cmd *cobra.Command, _ []string) error {
		global := mustGet(cmd, "global", cmd.Flags().GetBool)

		return runHookInstall(global)
	},
}

var hookUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Uninstall the git prepare-commit-msg hook",
	RunE: func(cmd *cobra.Command, _ []string) error {
		global := mustGet(cmd, "global", cmd.Flags().GetBool)

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

	if err := os.MkdirAll(hookDir, 0750); err != nil {
		return fmt.Errorf("failed to create hook directory: %w", err)
	}

	binaryPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	binaryPath, err = filepath.Abs(binaryPath)
	if err != nil {
		return fmt.Errorf("failed to resolve absolute path: %w", err)
	}

	if err := git.InstallHook(hookDir, git.PrepareCommitMsg, binaryPath); err != nil {
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

// HookConfig holds configuration for the hook mode's commit message generation.
type HookConfig struct {
	APIKey      string
	Model       string
	Temperature float64
	DetailLevel request.DetailLevel
	Hint        string
	Persona     request.PersonaName
	ACP         ACPProvider
}

// hookConfigFromConfig extracts a HookConfig from the shared CLI Config.
func hookConfigFromConfig(cfg Config) HookConfig {
	return HookConfig{
		APIKey:      cfg.APIKey,
		Model:       cfg.Model,
		Temperature: cfg.Temperature,
		DetailLevel: cfg.DetailLevel,
		Hint:        cfg.Hint,
		Persona:     cfg.Persona,
		ACP:         cfg.ACP,
	}
}

func runHookMode(cmd *cobra.Command, msgFile string) error {
	cfg := configFromCmd(cmd)
	hc := hookConfigFromConfig(cfg)

	ctx := context.Background()

	return runHookModeInternal(ctx, msgFile, hc)
}

// runHookModeInternal generates a commit message and writes it to the message file.
func runHookModeInternal(ctx context.Context, msgFile string, hc HookConfig) error {
	diff, err := getStagedDiffOrSkip(ctx)
	if err != nil {
		return fmt.Errorf("failed to get staged diff: %w", err)
	}
	if diff == "" {
		return nil
	}

	deleted, err := git.GetStagedDeletedFiles(ctx)
	if err != nil {
		return fmt.Errorf("failed to get deleted files: %w", err)
	}
	diff = appendDeletedContext(diff, deleted)

	client, err := request.NewClient(ctx, request.ClientConfig{
		APIKey:      hc.APIKey,
		Model:       hc.Model,
		Temperature: hc.Temperature,
		ACP:         string(hc.ACP),
	})
	if err != nil {
		return fmt.Errorf("failed to create request client: %w", err)
	}

	return generateAndWriteMsg(ctx, client, diff, msgFile, hc)
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
func generateAndWriteMsg(ctx context.Context, client *request.Client, diff, msgFile string, hc HookConfig) error {
	msg, err := client.GenerateCommitMessage(ctx, diff, "", hc.DetailLevel, hc.Hint, hc.Persona)
	if err != nil {
		return fmt.Errorf("failed to generate commit message: %w", err)
	}

	msg = appendAssistedBy(msg, client.ModelName())

	if err := os.WriteFile(msgFile, []byte(msg), 0600); err != nil {
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
