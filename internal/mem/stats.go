package mem

import (
	"fmt"
	"strings"
	"time"

	helix "github.com/helixdb/helix-db/sdks/go"
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

// ParseAuthorStats extracts author commit counts from a HelixDB response.
func ParseAuthorStats(resp *Response) []AuthorStat {
	nodes := resp.Nodes("by_author")
	if len(nodes) == 0 {
		nodes = resp.Nodes("authors_by_array")
	}

	if len(nodes) == 0 {
		return nil
	}

	emailCount := make(map[string]int)

	for _, n := range nodes {
		author := n.String("author")
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
func ParseTopFiles(resp *Response) []FileStat {
	nodes := resp.Nodes("files")
	if len(nodes) == 0 {
		return nil
	}

	pathCount := make(map[string]int)

	for _, n := range nodes {
		path := n.String("path")
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
func ParseTrends(resp *Response) []TrendPoint {
	nodes := resp.Nodes("commits_by_day")
	if len(nodes) == 0 {
		nodes = resp.Nodes("commits")
	}

	if len(nodes) == 0 {
		return nil
	}

	dayCount := make(map[string]int)

	for _, n := range nodes {
		t, ok := parseTimestamp(n.Float64("timestamp"))
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

// parseTimestamp converts a HelixDB timestamp float64 (Unix milliseconds)
// into a time.Time.
func parseTimestamp(v float64) (time.Time, bool) {
	if v == 0 {
		return time.Time{}, false
	}

	return time.UnixMilli(int64(v)), true
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
func ParseRepoSummary(resp *Response) RepoStats {
	var stats RepoStats

	stats.TotalCommits = resp.Count("total")

	stats.AuthorStats = ParseAuthorStats(resp)
	stats.FileStats = ParseTopFiles(resp)

	// If count-based parsing found nothing, fall back to counting node entries.
	if stats.TotalCommits == 0 {
		stats.TotalCommits = len(resp.Nodes("by_author"))
	}

	if stats.TotalCommits == 0 {
		stats.TotalCommits = len(resp.Nodes("authors_by_array"))
	}

	return stats
}

// FormatRepoSummary renders RepoStats as human-readable text.
func FormatRepoSummary(stats RepoStats) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Repository stats: %d commits indexed\n\n", stats.TotalCommits)

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
