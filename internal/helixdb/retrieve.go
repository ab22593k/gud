package helixdb

import (
	"fmt"
	"strings"

	"github.com/helixdb/helix-db/sdks/go"
)

const maxContextRecords = 5

// BuildContextQuery constructs a HelixDB ReadQuery that searches for relevant
// past commits to use as context for the LLM. It searches two ways:
//  1. BM25 over diff_text — find commits with semantically similar diffs
//  2. BM25 over message — find commits mentioning the same files
func BuildContextQuery(repoPath, branch string, files []string, diffText string) helix.Request {
	b := helix.ReadQuery("commit_context")
	returns := []string{"by_diff"}

	// 1. BM25 search over stored diff_text for the same repo.
	diffSearch := helix.G()
	if diffText != "" {
		diffSearch = diffSearch.
			TextSearchNodes("Commit", "diff_text", shortenDiff(diffText), maxContextRecords)
	} else {
		diffSearch = diffSearch.NWithLabel("Commit")
	}

	b.VarAs("by_diff", diffSearch.
		Has("repo_path", repoPath).
		ValueMap("$id", "id", "message", "author", "timestamp", "repo_path", "branch", "diff_stat").
		Limit(maxContextRecords))

	// 2. BM25 search over commit messages using file names as signals.
	if len(files) > 0 {
		msgQuery := strings.Join(files, " ")
		b.VarAs("by_message", helix.G().
			TextSearchNodes("Commit", "message", msgQuery, maxContextRecords).
			Has("repo_path", repoPath).
			ValueMap("$id", "id", "message", "author", "timestamp", "repo_path", "branch", "diff_stat").
			Limit(maxContextRecords))
		returns = append(returns, "by_message")
	}

	return b.Returning(returns...)
}

// ParseContextResults extracts CommitRecords from a HelixDB ReadBatch response.
func ParseContextResults(resp map[string]any) []CommitRecord {
	seen := make(map[string]bool)
	var records []CommitRecord

	for _, key := range []string{"by_diff", "by_message"} {
		raw, ok := resp[key]
		if !ok {
			continue
		}
		items, ok := raw.([]any)
		if !ok {
			continue
		}
		for _, item := range items {
			node, ok := item.(map[string]any)
			if !ok {
				continue
			}
			record := CommitRecordFromHelixData(node)
			if record.SHA != "" && !seen[record.SHA] {
				seen[record.SHA] = true
				records = append(records, record)
			}
		}
	}

	return records
}

// FormatContextRecords formats a set of CommitRecords as a human-readable
// string for inclusion in the LLM prompt. Returns empty string if nil/empty.
func FormatContextRecords(records []CommitRecord) string {
	if len(records) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("Related commit history (from HelixDB memory):\n")
	for _, r := range records {
		_, _ = fmt.Fprintf(&b, "  %s %s by %s [%s@%s]\n",
			truncateSHA(r.SHA), r.Message, truncateAuthor(r.Author), r.Branch, r.RepoPath)
	}
	return b.String()
}

func truncateSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

func shortenDiff(diff string) string {
	if len(diff) <= 500 {
		return diff
	}
	return diff[:500]
}

func truncateAuthor(author string) string {
	if idx := strings.Index(author, "@"); idx > 0 {
		return author[:idx] + "@..."
	}
	if len(author) > 20 {
		return author[:20] + "..."
	}
	return author
}
