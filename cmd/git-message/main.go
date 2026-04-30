package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"gud/internal/git"
	"gud/internal/request"
)

const version = "0.1.0"

// Config holds CLI configuration.
type Config struct {
	DetailLevel   string
	Persona       string
	Hint          string
	Context       string
	APIKey        string
	InstallHook   bool
	UninstallHook bool
	HookMode      string
	DryRun        bool
	ShowVer       bool
	Global        bool
}

func main() {
	config := parseFlags()
	if config.ShowVer {
		fmt.Printf("git-message version %s\n", version)
		return
	}

	if err := run(config); err != nil {
		log.Fatalf("Error: %v", err)
	}
}

func parseFlags() *Config {
	config := &Config{}
	flag.StringVar(&config.DetailLevel, "detail-level", "standard", "Set the detail level (minimal, standard, detailed)")
	flag.StringVar(&config.Persona, "persona", "embedded", "Set output style (embedded, google)")
	flag.StringVar(&config.Hint, "hint", "", "Focus boundaries for the AI")
	flag.StringVar(&config.Context, "context", "", "Additional context for the commit message")
	flag.StringVar(&config.APIKey, "api-key", "", "Gemini API key (or use GEMINI_API_KEY env)")
	flag.BoolVar(&config.InstallHook, "install-hook", false, "Install git hook for automatic message generation")
	flag.BoolVar(&config.UninstallHook, "uninstall-hook", false, "Uninstall git hook")
	flag.StringVar(&config.HookMode, "hook-mode", "", "Hook mode - pass the message file path from git hook")
	flag.BoolVar(&config.DryRun, "dry-run", false, "Show message without committing")
	flag.BoolVar(&config.ShowVer, "version", false, "Show version")
	flag.BoolVar(&config.Global, "global", false, "Use global hooks directory")
	flag.Parse()

	// Validate detail-level
	if config.DetailLevel != "minimal" && config.DetailLevel != "standard" && config.DetailLevel != "detailed" {
		config.DetailLevel = "standard"
	}

	// Validate persona (default to embedded)
	if config.Persona != "embedded" && config.Persona != "google" {
		config.Persona = "embedded"
	}

	// Get API key from env if not provided
	if config.APIKey == "" {
		config.APIKey = os.Getenv("GEMINI_API_KEY")
	}

	return config
}

func run(config *Config) error {
	// Handle hook installation
	if config.InstallHook {
		return installHook(config.Global)
	}

	// Handle hook uninstallation
	if config.UninstallHook {
		return uninstallHook(config.Global)
	}

	// Handle hook mode (called from git hook)
	if config.HookMode != "" {
		return runHookMode(config)
	}

	// Normal mode or dry-run
	return generateAndShow(config)
}

func installHook(global bool) error {
	hookDir, err := git.GetHookDir(global)
	if err != nil {
		return fmt.Errorf("failed to get hook directory: %w", err)
	}

	if err := os.MkdirAll(hookDir, 0755); err != nil {
		return fmt.Errorf("failed to create hook directory: %w", err)
	}

	if err := git.InstallHook(hookDir, "prepare-commit-msg"); err != nil {
		return fmt.Errorf("failed to install hook: %w", err)
	}

	fmt.Printf("Hook installed to %s\n", filepath.Join(hookDir, "prepare-commit-msg"))
	return nil
}

func uninstallHook(global bool) error {
	hookDir, err := git.GetHookDir(global)
	if err != nil {
		return fmt.Errorf("failed to get hook directory: %w", err)
	}

	if err := git.UninstallHook(hookDir, "prepare-commit-msg"); err != nil {
		return fmt.Errorf("failed to uninstall hook: %w", err)
	}

	fmt.Println("Hook uninstalled")
	return nil
}

func runHookMode(config *Config) error {
	return git.RunHookMode(context.Background(), config.HookMode, config.APIKey, config.DetailLevel, config.Hint)
}

func generateAndShow(config *Config) error {
	// Get staged diff first (needed for both dry-run and normal mode)
	diff, err := git.GetStagedDiff(context.Background())
	if err != nil {
		return fmt.Errorf("failed to get staged diff: %w", err)
	}

	if strings.TrimSpace(diff) == "" {
		return fmt.Errorf("no staged changes found. Use 'git add' to stage changes")
	}

	// Dry run - just show the diff (no API key needed)
	if config.DryRun {
		fmt.Println("=== Staged Diff ===")
		fmt.Println(diff)
		fmt.Println("=== End Diff ===")
		return nil
	}

	// Normal mode - API key required
	if config.APIKey == "" {
		return fmt.Errorf("API key is required. Set GEMINI_API_KEY env or use --api-key flag")
	}

	// Generate message
	client, err := request.NewClient(config.APIKey)
	if err != nil {
		return fmt.Errorf("failed to create request client: %w", err)
	}

	msg, err := client.GenerateCommitMessage(context.Background(), diff, config.Context, config.DetailLevel, config.Hint, config.Persona)
	if err != nil {
		return fmt.Errorf("failed to generate commit message: %w", err)
	}

	fmt.Println(msg)
	return nil
}
