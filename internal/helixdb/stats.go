package helixdb

import (
	"fmt"
	"strings"
	"time"

	"github.com/helixdb/helix-db/sdks/go"
)

// AuthorStat aggregates commit counts per author.
type AuthorStat struct {
	Email string
	Count int
}

// FileStat aggregates change counts per file.
type FileStat struct {
	Path    string
	Changes int
}

// RepoStats is the result of a repo summary query.
type RepoStats struct {
	TotalCommits int
	AuthorStats  []AuthorStat
	FileStats    []FileStat
}

// TrendPoint represents commit activity in a time bucket.
type TrendPoint struct {
	Date   string // "2026-06-29" or "2026-W26"
	Count  int
	IsWeek bool
}

// BuildRepoSummaryQuery constructs a ReadQuery that counts commits by author
// and by file for a given repo.
func BuildRepoSummaryQuery(repoPath string) helix.Request {
	b := helix.ReadQuery("repo_summary")

	b.VarAs("total", helix.G().
		NWithLabel("Commit").
		Has("repo_path", repoPath).
		Count())

	b.VarAs("by_author", helix.G().
		NWithLabel("Commit").
		Has("repo_path", repoPath).
		ValueMap("$id", "author"))

	b.VarAs("files", helix.G().
		NWithLabel("Commit").
		Has("repo_path", repoPath).
		Out("MODIFIES").
		ValueMap("$id", "path").
		Limit(100))

	return b.Returning("total", "by_author", "files")
}

// BuildAuthorStatsQuery returns a query that groups commits by author for a repo.
func BuildAuthorStatsQuery(repoPath string) helix.Request {
	b := helix.ReadQuery("author_stats")

	b.VarAs("by_author", helix.G().
		NWithLabel("Commit").
		Has("repo_path", repoPath).
		ValueMap("$id", "author", "message"))

	return b.Returning("by_author")
}

// BuildTopFilesQuery returns a query that finds the most frequently changed
// files in a repo by traversing from Commit → MODIFIES → File.
func BuildTopFilesQuery(repoPath string, limit int) helix.Request {
	b := helix.ReadQuery("top_files")

	b.VarAs("files", helix.G().
		NWithLabel("Commit").
		Has("repo_path", repoPath).
		Out("MODIFIES").
		ValueMap("$id", "path").
		Limit(limit))

	return b.Returning("files")
}

// BuildTrendsQuery returns a query that fetches all commits for a repo
// with their timestamps, for trend analysis.
func BuildTrendsQuery(repoPath string) helix.Request {
	b := helix.ReadQuery("trends")

	b.VarAs("commits", helix.G().
		NWithLabel("Commit").
		Has("repo_path", repoPath).
		ValueMap("$id", "timestamp", "author"))

	return b.Returning("commits")
}

// extractProperties extracts the "properties" array from a response key.
// HelixDB returns ValueMap results as {"key": {"properties": [...]}}.
func extractProperties(resp map[string]any, key string) []map[string]any {
	obj, ok := resp[key].(map[string]any)
	if !ok {
		return nil
	}
	raw, ok := obj["properties"].([]any)
	if !ok {
		return nil
	}
	result := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if m, ok := item.(map[string]any); ok {
			result = append(result, m)
		}
	}
	return result
}

// extractCount extracts an integer count from a response key.
// HelixDB returns Count results as {"key": {"count": N}}.
func extractCount(resp map[string]any, key string) int {
	obj, ok := resp[key].(map[string]any)
	if !ok {
		return 0
	}
	count, _ := obj["count"].(float64)
	return int(count)
}

// ParseAuthorStats extracts author commit counts from a HelixDB response.
func ParseAuthorStats(resp map[string]any) []AuthorStat {
	entries := extractProperties(resp, "by_author")
	if len(entries) == 0 {
		// Also try "authors_by_array" as an alternative key
		entries = extractProperties(resp, "authors_by_array")
	}
	if len(entries) == 0 {
		return nil
	}

	emailCount := make(map[string]int)
	for _, m := range entries {
		author, _ := m["author"].(string)
		if author == "" {
			continue
		}
		emailCount[author]++
	}

	var stats []AuthorStat
	for email, count := range emailCount {
		stats = append(stats, AuthorStat{Email: email, Count: count})
	}

	return stats
}

// ParseTopFiles extracts file change counts from a HelixDB response.
// The response includes ValueMap results from the File → MODIFIES → Commit traversal.
func ParseTopFiles(resp map[string]any) []FileStat {
	entries := extractProperties(resp, "files")
	if len(entries) == 0 {
		return nil
	}

	pathCount := make(map[string]int)
	for _, m := range entries {
		path, _ := m["path"].(string)
		if path == "" {
			continue
		}
		pathCount[path]++
	}

	var stats []FileStat
	for path, count := range pathCount {
		stats = append(stats, FileStat{Path: path, Changes: count})
	}

	return stats
}

// ParseTrends extracts daily trends from a HelixDB response.
func ParseTrends(resp map[string]any) []TrendPoint {
	entries := extractProperties(resp, "commits_by_day")
	if len(entries) == 0 {
		entries = extractProperties(resp, "commits")
	}
	if len(entries) == 0 {
		return nil
	}

	dayCount := make(map[string]int)
	for _, m := range entries {
		t, ok := parseTimestamp(m["timestamp"])
		if !ok {
			continue
		}
		date := t.Format("2006-01-02")
		dayCount[date]++
	}

	var trends []TrendPoint
	for date, count := range dayCount {
		trends = append(trends, TrendPoint{Date: date, Count: count})
	}

	return trends
}

// parseTimestamp attempts to parse a timestamp from various HelixDB response formats.
func parseTimestamp(v any) (time.Time, bool) {
	switch val := v.(type) {
	case string:
		t, err := time.Parse(time.RFC3339, val)
		if err == nil {
			return t, true
		}
	case float64:
		return time.UnixMilli(int64(val)), true
	}
	return time.Time{}, false
}

// FormatAuthorStats renders AuthorStat list as human-readable text.
func FormatAuthorStats(stats []AuthorStat) string {
	if len(stats) == 0 {
		return "No author data found.\n"
	}

	var b strings.Builder
	b.WriteString("Commits by author:\n")
	total := 0
	for _, s := range stats {
		total += s.Count
	}
	for _, s := range stats {
		pct := 0
		if total > 0 {
			pct = s.Count * 100 / total
		}
		_, _ = fmt.Fprintf(&b, "  %s  %d commits  (%d%%)\n", s.Email, s.Count, pct)
	}
	return b.String()
}

// FormatTopFiles renders FileStat list as human-readable text.
func FormatTopFiles(stats []FileStat) string {
	if len(stats) == 0 {
		return "No file data found.\n"
	}

	var b strings.Builder
	b.WriteString("Most changed files:\n")
	for _, s := range stats {
		_, _ = fmt.Fprintf(&b, "  %s  %d changes\n", s.Path, s.Changes)
	}
	return b.String()
}

// FormatTrends renders TrendPoint list as human-readable text.
func FormatTrends(trends []TrendPoint) string {
	if len(trends) == 0 {
		return "No trend data found.\n"
	}

	var b strings.Builder
	b.WriteString("Commit activity by day:\n")
	for _, t := range trends {
		_, _ = fmt.Fprintf(&b, "  %s  %d commits\n", t.Date, t.Count)
	}
	return b.String()
}

// ParseRepoSummary parses the HelixDB response into a RepoStats.
func ParseRepoSummary(resp map[string]any) RepoStats {
	var stats RepoStats

	stats.TotalCommits = extractCount(resp, "total")

	stats.AuthorStats = ParseAuthorStats(resp)
	stats.FileStats = ParseTopFiles(resp)

	// If top-level parsing found nothing but there are raw entries, try
	// grouping from the author count approach.
	if stats.TotalCommits == 0 {
		stats.TotalCommits = len(extractProperties(resp, "by_author"))
	}
	if stats.TotalCommits == 0 {
		stats.TotalCommits = len(extractProperties(resp, "authors_by_array"))
	}

	return stats
}

// FormatRepoSummary renders RepoStats as human-readable text.
func FormatRepoSummary(stats RepoStats) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Repository stats: %d commits indexed\n\n", stats.TotalCommits))

	if len(stats.AuthorStats) > 0 {
		b.WriteString("By author:\n")
		for _, a := range stats.AuthorStats {
			pct := 0
			if stats.TotalCommits > 0 {
				pct = a.Count * 100 / stats.TotalCommits
			}
			_, _ = fmt.Fprintf(&b, "  %s  %d commits  (%d%%)\n", a.Email, a.Count, pct)
		}
		b.WriteString("\n")
	}

	if len(stats.FileStats) > 0 {
		b.WriteString("Top files:\n")
		for _, f := range stats.FileStats {
			_, _ = fmt.Fprintf(&b, "  %s  %d changes\n", f.Path, f.Changes)
		}
	}

	return b.String()
}
