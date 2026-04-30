package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"gud/internal/git"
	"gud/internal/request"
)

const version = "0.1.0"

// Config holds CLI configuration.
type Config struct {
	DetailLevel   request.DetailLevel
	Persona       request.PersonaName
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
		slog.Error("fatal error", "error", err)
		os.Exit(1)
	}
}

func parseFlags() *Config {
	config := &Config{}

	var detailLevelStr, personaStr string
	flag.StringVar(&detailLevelStr, "detail-level", "standard", "Set the detail level (minimal, standard, detailed)")
	flag.StringVar(&personaStr, "persona", "embedded", "Set output style (embedded, google)")
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

	// Convert and validate detail-level
	config.DetailLevel = request.DetailLevel(detailLevelStr)
	if config.DetailLevel != request.DetailMinimal &&
		config.DetailLevel != request.DetailStandard &&
		config.DetailLevel != request.DetailDetailed {
		config.DetailLevel = request.DetailStandard
	}

	// Convert and validate persona
	config.Persona = request.PersonaName(personaStr)
	if config.Persona != request.PersonaEmbedded {
		config.Persona = request.PersonaEmbedded
	}

	// Get API key from env if not provided
	if config.APIKey == "" {
		config.APIKey = os.Getenv("GEMINI_API_KEY")
	}

	return config
}

func run(config *Config) error {
	if config.InstallHook {
		return installHook(config.Global)
	}

	if config.UninstallHook {
		return uninstallHook(config.Global)
	}

	if config.HookMode != "" {
		return runHookMode(config)
	}

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

	if err := git.InstallHook(hookDir, git.PrepareCommitMsg); err != nil {
		return fmt.Errorf("failed to install hook: %w", err)
	}

	fmt.Printf("Hook installed to %s\n", filepath.Join(hookDir, string(git.PrepareCommitMsg)))
	return nil
}

func uninstallHook(global bool) error {
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

func runHookMode(config *Config) error {
	return git.RunHookMode(context.Background(), config.HookMode, config.APIKey, config.DetailLevel, config.Hint, config.Persona)
}

func generateAndShow(config *Config) error {
	diff, err := git.GetStagedDiff(context.Background())
	if err != nil {
		return fmt.Errorf("failed to get staged diff: %w", err)
	}

	if strings.TrimSpace(diff) == "" {
		return fmt.Errorf("no staged changes found. Use 'git add' to stage changes")
	}

	if config.DryRun {
		fmt.Println("=== Staged Diff ===")
		fmt.Println(diff)
		fmt.Println("=== End Diff ===")
		return nil
	}

	if config.APIKey == "" {
		return fmt.Errorf("API key is required. Set GEMINI_API_KEY env or use --api-key flag")
	}

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
