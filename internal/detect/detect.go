// Package detect computes file-extension statistics for a repository
// and provides formatting utilities for AI prompt context injection.
// It has zero internal dependencies beyond the standard library.
package detect

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

// RepoStats captures a file-extension statistics summary of a repository.
type RepoStats struct {
	FilesByExtension map[string]int `json:"files_by_extension"`
	TotalFiles       int            `json:"total_files"`
}

// ComputeStatsWithContext walks repoRoot counting files by extension. It aborts
// early on ctx cancellation and stops counting after MaxFilesForStats files.
// The .git skip, .gitignore pruning, unreadable-path tolerance, and
// MaxFilesForStats cap are all defined here so there is a single walk to reason
// about.
func ComputeStatsWithContext(ctx context.Context, repoRoot string) (*RepoStats, error) {
	stats := &RepoStats{FilesByExtension: make(map[string]int)}
	matcher := loadGitignore(repoRoot)

	err := filepath.WalkDir(repoRoot, func(p string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if err != nil {
			return nil //nolint:nilerr // documented: unreadable paths are skipped silently
		}

		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}

			if p != repoRoot && matcher.ignored(relPath(repoRoot, p)) {
				return filepath.SkipDir
			}

			return nil
		}

		if stats.TotalFiles >= MaxFilesForStats {
			return filepath.SkipAll
		}

		ext := strings.ToLower(filepath.Ext(p))
		if ext == "" {
			ext = noExtensionKey
		}

		stats.FilesByExtension[ext]++
		stats.TotalFiles++

		return nil
	})
	if err != nil {
		return stats, err
	}

	return stats, nil
}

// relPath returns the slash-separated path of p relative to root ("." for
// the root itself, "" when the relative path cannot be computed).
func relPath(root, p string) string {
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return ""
	}

	return filepath.ToSlash(rel)
}

// FormatRepoContext returns a human-readable summary of repo statistics
// suitable for injection into the AI prompt context. Returns empty string
// if stats is nil or has no files.
func FormatRepoContext(stats *RepoStats) string {
	if stats == nil || stats.TotalFiles == 0 {
		return ""
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Repository: %d files across %d extensions\n",
		stats.TotalFiles, len(stats.FilesByExtension))

	type extCount struct {
		ext   string
		count int
	}

	sorted := make([]extCount, 0, len(stats.FilesByExtension))
	for ext, count := range stats.FilesByExtension {
		sorted = append(sorted, extCount{ext, count})
	}

	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].count != sorted[j].count {
			return sorted[i].count > sorted[j].count
		}

		return sorted[i].ext < sorted[j].ext
	})

	for _, ec := range sorted {
		pct := float64(ec.count) / float64(stats.TotalFiles) * 100
		fmt.Fprintf(&sb, "  %-6s %3d  (%3.0f%%)\n", ec.ext, ec.count, pct)
	}

	return sb.String()
}
