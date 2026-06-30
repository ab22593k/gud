package helixdb

import (
	"fmt"
	"strings"

	"github.com/helixdb/helix-db/sdks/go"
)

const maxContextRecords = 5

// BuildContextQuery constructs a HelixDB ReadQuery that searches for relevant
// past commits to use as context for the LLM. It searches three ways:
//  1. BM25 over diff_text — find commits with semantically similar diffs
//  2. BM25 over message — find commits mentioning the same files
//  3. Vector search over embedding — find semantically similar commits (when
//     diffText is long enough to generate a meaningful embedding query)
func BuildContextQuery(tenantID, branch string, files []string, diffText string) helix.Request {
	b := helix.ReadQuery("commit_context")
	returns := []string{"by_diff"}

	// 1. BM25 search over stored diff_text for the same tenant.
	diffSearch := helix.G()
	if diffText != "" {
		diffSearch = diffSearch.
			TextSearchNodes("Commit", "diff_text", shortenDiff(diffText), maxContextRecords, tenantID)
	} else {
		diffSearch = diffSearch.NWithLabel("Commit")
	}

	b.VarAs("by_diff", diffSearch.
		Has("repo_path", tenantID).
		Where(helix.PredIsNull("deletedAt")).
		Project(
			helix.ProjectPropAs("$id", "$id"),
			helix.ProjectPropAs("id", "sha"),
			helix.ProjectPropAs("message", "message"),
			helix.ProjectPropAs("author", "author"),
			helix.ProjectPropAs("timestamp", "timestamp"),
			helix.ProjectPropAs("repo_path", "repo_path"),
			helix.ProjectPropAs("branch", "branch"),
			helix.ProjectPropAs("diff_stat", "diff_stat"),
		).
		Limit(maxContextRecords))

	// 2. BM25 search over commit messages using file names as signals.
	if len(files) > 0 {
		msgQuery := strings.Join(files, " ")
		b.VarAs("by_message", helix.G().
			TextSearchNodes("Commit", "message", msgQuery, maxContextRecords, tenantID).
			Has("repo_path", tenantID).
			Where(helix.PredIsNull("deletedAt")).
			Project(
				helix.ProjectPropAs("$id", "$id"),
				helix.ProjectPropAs("id", "sha"),
				helix.ProjectPropAs("message", "message"),
				helix.ProjectPropAs("author", "author"),
				helix.ProjectPropAs("timestamp", "timestamp"),
				helix.ProjectPropAs("repo_path", "repo_path"),
				helix.ProjectPropAs("branch", "branch"),
				helix.ProjectPropAs("diff_stat", "diff_stat"),
			).
			Limit(maxContextRecords))
		returns = append(returns, "by_message")
	}

	return b.Returning(returns...)
}

// BuildHybridContextQuery fuses vector + BM25 recall for commits that are
// semantically similar to the given diff text. This works only when commits
// have been stored with embeddings (text-embedding-3-small, 1536-dim F32).
// Falls back to BM25-only if diffText is empty.
func BuildHybridContextQuery(tenantID string, queryVector []float32, diffText string, files []string, limit int64) helix.Request {
	b := helix.ReadQuery("hybrid_commit_context")
	returns := []string{}

	// 1. Vector search when an embedding query vector is provided.
	if len(queryVector) > 0 {
		b.VarAs("by_vector",
			helix.G().
				VectorSearchNodes("Commit", "embedding", queryVector, int(limit), tenantID).
				Where(helix.PredIsNull("deletedAt")).
		Project(
				helix.ProjectPropAs("$id", "$id"),
				helix.ProjectPropAs("id", "sha"),
				helix.ProjectPropAs("message", "message"),
				helix.ProjectPropAs("author", "author"),
				helix.ProjectPropAs("$distance", "distance"),
				helix.ProjectPropAs("repo_path", "repo_path"),
				helix.ProjectPropAs("branch", "branch"),
				helix.ProjectPropAs("diff_stat", "diff_stat"),
			).
			Limit(int(limit)))
		returns = append(returns, "by_vector")
	}

	// 2. BM25 search over diff text.
	if diffText != "" {
		b.VarAs("by_diff",
			helix.G().
				TextSearchNodes("Commit", "diff_text", shortenDiff(diffText), int(limit), tenantID).
				Has("repo_path", tenantID).
				Where(helix.PredIsNull("deletedAt")).
				Project(
					helix.ProjectPropAs("$id", "$id"),
					helix.ProjectPropAs("id", "sha"),
					helix.ProjectPropAs("message", "message"),
					helix.ProjectPropAs("author", "author"),
					helix.ProjectPropAs("$distance", "distance"),
					helix.ProjectPropAs("repo_path", "repo_path"),
					helix.ProjectPropAs("branch", "branch"),
					helix.ProjectPropAs("diff_stat", "diff_stat"),
				).
				Limit(int(limit)))
		returns = append(returns, "by_diff")
	}

	return b.Returning(returns...)
}

// BuildEntityContextQuery finds commits that mention specific code elements
// through MENTIONS edges, for entity-aware recall.
func BuildEntityContextQuery(tenantID string, codeElementKeys []string, limit int) helix.Request {
	b := helix.ReadQuery("entity_commit_context")

	elemKeys := b.ParamArray("element_keys", toInterfaceSlice(codeElementKeys), helix.ParamTypeString())

	b.VarAs("commits",
		helix.G().
			NWithLabel("CodeElement").
			Where(helix.PredIsIn("elementKey", elemKeys)).
			Where(helix.PredEq("tenantId", tenantID)).
			In("MENTIONS").
			HasLabel("Commit").
			Where(helix.PredIsNull("deletedAt")).
			Project(
				helix.ProjectPropAs("$id", "$id"),
				helix.ProjectPropAs("id", "sha"),
				helix.ProjectPropAs("message", "message"),
				helix.ProjectPropAs("author", "author"),
				helix.ProjectPropAs("timestamp", "timestamp"),
				helix.ProjectPropAs("repo_path", "repo_path"),
				helix.ProjectPropAs("branch", "branch"),
				helix.ProjectPropAs("diff_stat", "diff_stat"),
			).
			Limit(limit))

	return b.Returning("commits")
}

// ParseContextResults extracts CommitRecords from a HelixDB ReadBatch response.
// Handles both flat arrays (e.g. TextSearch/VectorSearch results) and
// {"properties": [...]} wrapped results (e.g. plain ValueMap traversals).
func ParseContextResults(resp map[string]any) []CommitRecord {
	seen := make(map[string]bool)
	var records []CommitRecord

	for _, key := range []string{"by_diff", "by_message", "by_vector", "commits"} {
		items := extractResultItems(resp[key])
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

// extractResultItems normalises a HelixDB response value into a slice of items,
// handling both flat arrays and {"properties": [...]} wrappers.
func extractResultItems(raw any) []any {
	if items, ok := raw.([]any); ok {
		return items
	}
	if obj, ok := raw.(map[string]any); ok {
		if props, ok := obj["properties"].([]any); ok {
			return props
		}
	}
	return nil
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

// BuildMemoryContextQuery retrieves active (non-deleted, non-expired) Memory
// nodes for a given tenant and optional user scope, with BM25 text search.
func BuildMemoryContextQuery(tenantID, userID, query string, limit int) helix.Request {
	b := helix.ReadQuery("memory_context")

	scope := b.ParamString("tenant_id", tenantID)
	user := b.ParamString("user_id", userID)

	traversal := helix.G().
		NWithLabel("Memory").
		Where(helix.PredEq(DefaultTenantProperty, scope)).
		Where(helix.PredIsNull("deletedAt")).
		Where(helix.PredEq("isLatest", true))

	if userID != "" {
		traversal = traversal.Where(helix.PredEq("userId", user))
	}
	if query != "" {
		traversal = traversal.Where(helix.PredContains("content", query))
	}

	b.VarAs("memories",
		traversal.
			ValueMap("$id", "memoryId", "content", "kind", "salience", "userId", "createdAt").
			Limit(limit))

	return b.Returning("memories")
}

// BuildCategoryMemoriesQuery retrieves memories in a specific category.
func BuildCategoryMemoriesQuery(categoryKey, tenantID string, limit int) helix.Request {
	b := helix.ReadQuery("category_memories")

	catKey := b.ParamString("category_key", categoryKey)
	scope := b.ParamString("tenant_id", tenantID)

	b.VarAs("memories",
		helix.G().
			NWithLabel("Category").
			Where(helix.PredEq("categoryKey", catKey)).
			Where(helix.PredEq(DefaultTenantProperty, scope)).
			In("IN_CATEGORY").
			HasLabel("Memory").
			Where(helix.PredIsNull("deletedAt")).
			Where(helix.PredEq("isLatest", true)).
			ValueMap("$id", "memoryId", "content", "kind", "salience", "createdAt").
			Limit(limit))

	return b.Returning("memories")
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

func toInterfaceSlice(ss []string) []any {
	result := make([]any, len(ss))
	for i, s := range ss {
		result[i] = s
	}
	return result
}
