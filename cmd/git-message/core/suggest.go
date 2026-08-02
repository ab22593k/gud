package core

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gud/internal/config"
	"gud/internal/config/dto"
	"gud/internal/detect"
	"gud/internal/profile"

	"github.com/spf13/cobra"
)

// skipMarker is the filename written to the working directory when the user
// chooses to skip the profile suggestion. Its presence suppresses future
// prompts. Both the skip marker and gud.json live in CWD so the mediator's
// CWDProvider can find them consistently.
const skipMarker = ".gud-skip"

// suggestProfileIfNeeded attempts to suggest a profile based on repo
// file statistics when no profile is configured. It runs interactively:
// fetches the remote catalog, ranks by keyword match, and prompts the
// user to select or skip.
//
// This is a no-op if any profile is already configured (via CLI, env, or
// config file), the repo root is unavailable, the skip marker exists,
// or stdin is not a terminal (non-interactive mode).
//
//nolint:funlen // suggestion flow is cohesive; breaking it up would add complexity.
func suggestProfileIfNeeded(ctx context.Context, cmd *cobra.Command, app *AppContext) error {
	out := cmd.OutOrStdout()
	in := cmd.InOrStdin()

	// Only prompt in interactive terminals
	file, ok := in.(*os.File)
	if !ok || !isTerminal(file) {
		return nil
	}

	// Get repo root and CWD — repo root for stats, CWD for config and skip marker
	repoRoot, err := app.RepoRoot(ctx)
	if err != nil {
		return fmt.Errorf("repo root: %w", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("cwd: %w", err)
	}

	// Skip if the user already dismissed the suggestion in this directory
	if hasSkipMarker(cwd) {
		return nil
	}

	// Compute repo stats
	stats, err := detect.ComputeStats(repoRoot)
	if err != nil {
		return fmt.Errorf("compute stats: %w", err)
	}
	if stats.TotalFiles == 0 {
		return nil
	}

	// Fetch remote catalog
	initProfileManager()
	entries, err := profileManager.FetchCatalog(ctx)
	if err != nil {
		return fmt.Errorf("fetch catalog: %w", err)
	}

	// Rank and suggest
	suggestions := detect.SuggestProfile(stats, entries)
	if len(suggestions) == 0 {
		return nil
	}

	// Show suggestion prompt
	_, _ = fmt.Fprint(out, detect.FormatSuggestionMessage(suggestions))

	// Read user selection
	scanner := bufio.NewScanner(in)
	if !scanner.Scan() {
		return nil
	}
	input := strings.TrimSpace(scanner.Text())

	switch {
	case input == "" || strings.EqualFold(input, "s") || strings.EqualFold(input, "skip"):
		writeSkipMarker(cwd)
		_, _ = fmt.Fprintln(out, "Skipped. To see all profiles: gud profile list --remote")

		return nil

	case strings.EqualFold(input, "a") || strings.EqualFold(input, "abort"):
		_, _ = fmt.Fprintln(out, "Aborted.")

		return nil

	default:
		idx, err := strconv.Atoi(input)
		if err != nil || idx < 1 || idx > len(suggestions) {
			_, _ = fmt.Fprintf(out, "Invalid selection %q. Skipping.\n", input)
			writeSkipMarker(cwd)

			return nil
		}

		selected := suggestions[idx-1]

		return applySelectedProfile(ctx, app, out, selected)
	}
}

// applySelectedProfile caches the profile, updates app config, and writes
// gud.json in the working directory so the selection persists across invocations.
func applySelectedProfile(ctx context.Context, app *AppContext, out io.Writer, entry profile.CatalogEntry) error {
	// Download and cache if not already cached
	if !profileManager.IsCached(entry.Slug) {
		_, _ = fmt.Fprintf(out, "Downloading profile %q...\n", entry.Slug)
		if err := downloadAndSaveProfile(ctx, entry.Slug, entry.Profession); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(out, "Profile %q saved.\n", entry.Slug)
	}

	// Write gud.json in CWD (where the mediator's CWDProvider looks for it).
	// This ensures the selection persists across invocations.
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get cwd: %w", err)
	}
	projCfg := config.Config{
		Profile: config.ProfileName(entry.Slug),
	}
	if err := writeProjectConfig(cwd, projCfg); err != nil {
		return fmt.Errorf("write gud.json: %w", err)
	}

	// Apply immediately for this invocation
	app.setProfile(config.ProfileName(entry.Slug))

	_, _ = fmt.Fprintf(out, "Profile %q selected. Run 'gud message' to generate a commit.\n", entry.Slug)

	return nil
}

// writeProjectConfig writes a config DTO to gud.json in the given directory.
// It merges with any existing file to preserve other settings.
//
// G117: marshaling APIKey is safe — writing to a local config file.
//
//nolint:gosec // G304: path is constructed from os.Getwd(), not user input.
func writeProjectConfig(dir string, cfg config.Config) error {
	path := filepath.Join(dir, "gud.json")

	// Read existing config if any
	var existing dto.ConfigDTO
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &existing)
	}

	// Merge with new config
	merged := existing.ToEntity().Merge(cfg)
	dto := dto.FromEntity(merged)

	data, err := json.MarshalIndent(dto, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	return os.WriteFile(path, data, 0600)
}

// hasSkipMarker returns true if the .gud-skip marker exists in the directory.
func hasSkipMarker(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, skipMarker))

	return err == nil
}

// writeSkipMarker creates the .gud-skip marker to suppress future prompts.
func writeSkipMarker(dir string) {
	_ = os.WriteFile(filepath.Join(dir, skipMarker), []byte("# gud profile suggestion skipped\n"), 0600)
}

// isTerminal reports whether f is a character device (terminal).
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}

	return fi.Mode()&os.ModeCharDevice != 0
}
