// Package detect computes file-extension statistics for a repository
// and provides formatting utilities for AI prompt context injection.
// It has zero internal dependencies beyond the standard library.
package detect

import (
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

// ComputeStats walks the repo root directory and counts files by extension.
// It skips the .git directory and any unreadable paths silently.
// Returns an empty RepoStats with zero values if the repo root is unreadable.
func ComputeStats(repoRoot string) (*RepoStats, error) {
	stats := &RepoStats{
		FilesByExtension: make(map[string]int),
	}

	err := filepath.WalkDir(repoRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable paths
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		if d.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if ext == "" {
			ext = "(no extension)"
		}
		stats.FilesByExtension[ext]++
		stats.TotalFiles++

		return nil
	})
	if err != nil {
		return &RepoStats{FilesByExtension: make(map[string]int)}, err
	}

	return stats, nil
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
