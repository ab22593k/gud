package helixdb

import (
	"fmt"
	"strings"

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

	return b.Returning("total", "by_author")
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
// files in a repo.
func BuildTopFilesQuery(repoPath string, limit int) helix.Request {
	b := helix.ReadQuery("top_files")

	b.VarAs("files", helix.G().
		NWithLabel("File").
		Has("repo_path", repoPath).
		In("MODIFIES").
		ValueMap("$id", "path").
		Limit(limit))

	return b.Returning("files")
}

// ParseRepoSummary parses the HelixDB response into a RepoStats.
func ParseRepoSummary(resp map[string]any) RepoStats {
	var stats RepoStats

	// Parse total count.
	if total, ok := resp["total"].(float64); ok {
		stats.TotalCommits = int(total)
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
