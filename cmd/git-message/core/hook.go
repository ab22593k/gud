package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gud/internal/git"
	"gud/internal/request"

	"github.com/spf13/cobra"
)

const hookCmdName = "hook"

var hookCmd = &cobra.Command{
	Use:   hookCmdName,
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

// hookTimeout is the maximum time a hook-mode generation is allowed to take.
// This prevents a stuck Gemini API call from blocking git commit indefinitely.
const hookTimeout = 2 * time.Minute

func runHookMode(cmd *cobra.Command, msgFile string) error {
	app, err := NewAppContext(cmd)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), hookTimeout)
	defer cancel()

	return runHookModeInternal(ctx, msgFile, app)
}

// runHookModeInternal generates a commit message and writes it to the message file.
func runHookModeInternal(ctx context.Context, msgFile string, app *AppContext) error {
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

	if err := app.InitClient(ctx); err != nil {
		return err
	}

	return generateAndWriteMsg(ctx, app, diff, msgFile)
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
// If the file already contains meaningful content (non-comment lines), it is
// left untouched — this prevents a prepare-commit-msg hook from overwriting a
// message provided by an interactive git-message commit or git commit -m.
func generateAndWriteMsg(ctx context.Context, app *AppContext, diff, msgFile string) error {
	msgFile = filepath.Clean(msgFile)
	existing, err := os.ReadFile(msgFile)
	if err == nil && hasMeaningfulContent(string(existing)) {
		return nil
	}

	cfg := app.Config()
	profileContent := resolveProfileContent(string(cfg.Profile))
	msg, err := app.Client().GenerateCommitMessageWithContent(
		ctx, diff, "", request.DetailLevel(cfg.DetailLevel),
		cfg.Hint, request.ProfileName(cfg.Profile), profileContent, cfg.WrapLine,
	)
	if err != nil {
		return fmt.Errorf("failed to generate commit message: %w", err)
	}

	msg = appendAssistedBy(msg, app.Client().ModelName())

	if err := os.WriteFile(filepath.Clean(msgFile), []byte(msg), 0600); err != nil {
		return fmt.Errorf("failed to write message file: %w", err)
	}

	return nil
}

// hasMeaningfulContent reports whether text contains any line that is not a
// git comment line (starting with #). The default COMMIT_EDITMSG template
// consists entirely of comments, so this distinguishes "user supplied a
// message" from "git created an empty template".
func hasMeaningfulContent(text string) bool {
	for line := range strings.SplitSeq(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			return true
		}
	}

	return false
}

func init() {
	hookInstallCmd.Flags().Bool("global", false, "Use global hooks directory")
	hookUninstallCmd.Flags().Bool("global", false, "Use global hooks directory")

	hookCmd.AddCommand(hookInstallCmd)
	hookCmd.AddCommand(hookUninstallCmd)
	hookCmd.AddCommand(hookRunCmd)
	rootCmd.AddCommand(hookCmd)
}
