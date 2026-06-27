package core

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"gud/internal/config"
	"gud/internal/config/provider"
	"gud/internal/git"
	"gud/internal/request"

	"github.com/spf13/cobra"
)

// maxHistory is the maximum number of recent commits the --history flag can request.
// This prevents accidentally dumping hundreds of commits into the prompt and wasting tokens.
const maxHistory = git.MaxRecentCommits

// Config priority (highest to lowest):
//
//  1. CLI flags (configFromCmd)
//  2. Environment variables (configFromEnv)
//  3. CWD config file (./gud.json)
//  4. XDG config file (~/.config/gud/config.json)
//  5. Sensible defaults (DefaultConfig)
//
// loadMergedConfig layers all sources with the above priority, then validates.
func loadMergedConfig(cliCfg config.Config) config.Config {
	result := config.DefaultConfig()
	result = result.Merge(loadConfigFile()) // XDG config
	result = result.Merge(loadCWDConfig())  // CWD config
	result = result.Merge(configFromEnv())  // env vars
	result = result.Merge(cliCfg)           // CLI flags

	return result.Validate()
}

// loadConfigFile attempts to load configuration from the XDG JSON path.
// Returns zero-value Config if the file doesn't exist or can't be read.
func loadConfigFile() config.Config {
	path, err := provider.DefaultConfigPath()
	if err != nil {
		return config.Config{}
	}

	return loadConfigFrom(path)
}

// loadCWDConfig attempts to load configuration from ./gud.json in the
// current working directory. Returns zero-value Config if not found.
func loadCWDConfig() config.Config {
	cwd, err := os.Getwd()
	if err != nil {
		return config.Config{}
	}

	return loadConfigFrom(filepath.Join(cwd, "gud.json"))
}

// loadConfigFrom reads a config JSON file at the given path.
// Returns zero-value Config (silently) if the file doesn't exist.
func loadConfigFrom(path string) config.Config {
	p := provider.NewFileProvider(path)
	cfg, err := p.Load()
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Debug("failed to load config file", "path", path, "error", err)
		}

		return config.Config{}
	}

	slog.Debug("loaded config file", "path", path)

	return cfg
}

// runGenerate is the default action: generate a commit message from staged changes.
func runGenerate(cmd *cobra.Command, _ []string) error {
	cliCfg := configFromCmd(cmd)
	cfg := loadMergedConfig(cliCfg)

	if cfg.APIKey == "" {
		slog.Debug("no API key configured; opencode provider does not require one")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := requireProfile(string(cfg.Profile)); err != nil {
		return err
	}

	diff, err := getStagedDiffOrError(ctx)
	if err != nil {
		return err
	}

	promptContext := buildHistoryContext(ctx, cfg)

	client, err := request.NewClient(ctx, request.ClientConfig{
		APIKey:      cfg.APIKey,
		Model:       cfg.Model,
		Temperature: cfg.Temperature,
		ACP:         string(cfg.ACP),
	})
	if err != nil {
		return fmt.Errorf("failed to create request client: %w", err)
	}

	return interactiveCommit(ctx, cmd, client, diff, promptContext, cfg)
}

// resolveProfileContent returns the AGENTS.md content for a cached profile.
// Returns empty string if no profile is set.
func resolveProfileContent(profileName string) string {
	if profileName == "" {
		return ""
	}
	initProfileManager()
	p, err := profileManager.Get(profileName)
	if err != nil {
		slog.Debug("profile not found in cache", "profile", profileName)

		return ""
	}

	return p.Content
}

// requireProfile checks that the given profile is cached (or empty).
// If set but not found, it tells the user to download it first.
func requireProfile(profileName string) error {
	if profileName == "" {
		return nil
	}
	initProfileManager()
	_, err := profileManager.Get(profileName)
	if err != nil {
		return fmt.Errorf("profile %q not found.\n\n"+
			"First download it:  gud profile save %s\n"+
			"See all:            gud profile list --remote", profileName, profileName)
	}

	return nil
}

// getStagedDiffOrError retrieves the staged diff and returns an error if none exists.
func getStagedDiffOrError(ctx context.Context) (string, error) {
	diff, deleted, err := getStagedDiffAndDeleted(ctx)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(diff) == "" {
		return "", errors.New("no staged changes found. Use 'git add' to stage changes")
	}

	return appendDeletedContext(diff, deleted), nil
}

// getStagedDiffAndDeleted retrieves both the staged diff (excluding deleted content)
// and the list of deleted file names.
func getStagedDiffAndDeleted(ctx context.Context) (diff, deleted string, err error) {
	diff, err = git.GetStagedDiff(ctx)
	if err != nil {
		return "", "", fmt.Errorf("failed to get staged diff: %w", err)
	}

	deleted, err = git.GetStagedDeletedFiles(ctx)
	if err != nil {
		return "", "", fmt.Errorf("failed to get deleted files: %w", err)
	}

	return diff, deleted, nil
}

// appendDeletedContext appends a note about deleted files to the diff if any exist.
func appendDeletedContext(diff, deleted string) string {
	if strings.TrimSpace(deleted) == "" {
		return diff
	}

	// Sanitize each filename line: trim whitespace, filter empty lines.
	// This prevents injection of fake diff context via malicious filenames
	// containing embedded newlines, while preserving the one-per-line format.
	lines := strings.Split(deleted, "\n")
	var clean []string
	for _, line := range lines {
		if line = strings.TrimSpace(line); line != "" {
			clean = append(clean, line)
		}
	}

	var b strings.Builder
	b.WriteString(diff)
	b.WriteString("\n\nDeleted files:\n")
	for i, line := range clean {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(line)
	}
	b.WriteString("\n")

	return b.String()
}

// buildHistoryContext returns a formatted string of recent commit history, or
// empty string if --history is disabled or there are no recent commits.
// Errors from git are logged at debug level and silently discarded —
// history is optional context for the AI prompt and should never block generation.
func buildHistoryContext(ctx context.Context, cfg config.Config) string {
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
