package mem

import (
	"fmt"
	"sort"
	"strings"
	"time"

	helix "github.com/helixdb/helix-db/sdks/go"
)

const maxContextRecords = 5

// BuildContextQuery constructs a HelixDB ReadQuery that searches for relevant
// past commits to use as context for the LLM. It searches two ways:
//  1. BM25 over diff_text — find commits with semantically similar diffs
//  2. BM25 over message — find commits mentioning the same files
//
// When branch is non-empty, results are scoped to that branch, including
// legacy records persisted without a branch (branch = "").
// Return-key names shared by the retrieval query builders and result parsers.
const (
	returnKeyByDiff    = "by_diff"
	returnKeyByMessage = "by_message"
	returnKeyByVector  = "by_vector"
	returnKeyCommits   = "commits"
)

func BuildContextQuery(tenantID, branch string, files []string, diffText string) helix.Request {
	b := helix.ReadQuery("commit_context")
	returns := []string{returnKeyByDiff}

	// 1. BM25 search over stored diff_text for the same tenant.
	diffSearch := helix.G()
	if diffText != "" {
		diffSearch = diffSearch.
			TextSearchNodes("Commit", "diff_text", shortenDiff(diffText), maxContextRecords, tenantID)
	} else {
		diffSearch = diffSearch.NWithLabel("Commit")
	}

	byDiff := diffSearch.
		Has("repo_path", tenantID).
		Where(helix.PredIsNull("deletedAt"))
	if branch != "" {
		byDiff = byDiff.Where(branchFilter(branch))
	}

	b.VarAs(returnKeyByDiff, byDiff.Project(
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

		byMessage := helix.G().
			TextSearchNodes("Commit", "message", msgQuery, maxContextRecords, tenantID).
			Has("repo_path", tenantID).
			Where(helix.PredIsNull("deletedAt"))
		if branch != "" {
			byMessage = byMessage.Where(branchFilter(branch))
		}

		b.VarAs(returnKeyByMessage, byMessage.Project(
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
		returns = append(returns, returnKeyByMessage)
	}

	return b.Returning(returns...)
}

// branchFilter scopes results to the given branch, including legacy records
// persisted without a branch (branch = ""). It must only be applied when
// branch is non-empty.
func branchFilter(branch string) helix.Predicate {
	return helix.PredOr(
		helix.PredEq("branch", branch),
		helix.PredEq("branch", ""),
	)
}

// BuildHybridContextQuery fuses vector + BM25 recall for commits that are
// semantically similar to the given diff text, across three ranked sources:
//
//  1. by_vector  — vector similarity to the query embedding (stored when the
//     commit was persisted with an embedding; model must match the index)
//  2. by_diff    — BM25 over stored diff_text
//  3. by_message — BM25 over commit messages using file names as signals
//
// All sources are scoped to the tenant, filtered for soft-deleted records, and
// when branch is non-empty scoped to that branch (including legacy records
// persisted without a branch). Sources with no usable input are omitted.
func BuildHybridContextQuery(
	tenantID, branch string, queryVector []float32, diffText string, files []string, limit int64,
) helix.Request {
	b := helix.ReadQuery("hybrid_commit_context")
	returns := []string{}

	// scope anchors every source to the tenant, drops soft-deleted records,
	// and optionally limits to the current branch.
	scope := func(t *helix.Traversal) *helix.Traversal {
		t = t.Has("repo_path", tenantID).Where(helix.PredIsNull("deletedAt"))
		if branch != "" {
			t = t.Where(branchFilter(branch))
		}

		return t
	}

	projectSearch := func(t *helix.Traversal) *helix.Traversal {
		return t.Project(
			helix.ProjectPropAs("$id", "$id"),
			helix.ProjectPropAs("id", "sha"),
			helix.ProjectPropAs("message", "message"),
			helix.ProjectPropAs("author", "author"),
			helix.ProjectPropAs("$distance", "distance"),
			helix.ProjectPropAs("repo_path", "repo_path"),
			helix.ProjectPropAs("branch", "branch"),
			helix.ProjectPropAs("diff_stat", "diff_stat"),
		).Limit(int(limit))
	}

	// 1. Vector search when an embedding query vector is provided.
	if len(queryVector) > 0 {
		b.VarAs(returnKeyByVector,
			projectSearch(scope(helix.G().
				VectorSearchNodes("Commit", "embedding", queryVector, int(limit), tenantID))))
		returns = append(returns, returnKeyByVector)
	}

	// 2. BM25 search over diff text.
	if diffText != "" {
		b.VarAs(returnKeyByDiff,
			projectSearch(scope(helix.G().
				TextSearchNodes("Commit", "diff_text", shortenDiff(diffText), int(limit), tenantID))))
		returns = append(returns, returnKeyByDiff)
	}

	// 3. BM25 search over commit messages using file names as signals.
	if len(files) > 0 {
		b.VarAs(returnKeyByMessage,
			projectSearch(scope(helix.G().
				TextSearchNodes("Commit", "message", strings.Join(files, " "), int(limit), tenantID))))
		returns = append(returns, returnKeyByMessage)
	}

	return b.Returning(returns...)
}

// BuildEntityContextQuery finds commits that mention specific code elements
// through MENTIONS edges, for entity-aware recall. When branch is non-empty,
// results are scoped to that branch, including legacy records persisted
// without a branch (branch = ""), matching BuildHybridContextQuery.
func BuildEntityContextQuery(tenantID, branch string, codeElementKeys []string, limit int) helix.Request {
	b := helix.ReadQuery("entity_commit_context")

	elemKeys := b.ParamArray("element_keys", toInterfaceSlice(codeElementKeys), helix.ParamTypeString())

	traversal := helix.G().
		NWithLabel("CodeElement").
		Where(helix.PredIsIn("elementKey", elemKeys)).
		Where(helix.PredEq("tenantId", tenantID)).
		In("MENTIONS").
		HasLabel("Commit").
		Where(helix.PredIsNull("deletedAt"))
	if branch != "" {
		traversal = traversal.Where(branchFilter(branch))
	}

	b.VarAs(returnKeyCommits,
		traversal.
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

	return b.Returning(returnKeyCommits)
}

// ScoredCommit pairs a retrieved commit with its relevance distance from the
// search source ($distance; smaller is more relevant for both vector and BM25).
type ScoredCommit struct {
	Record   CommitRecord
	Distance float64
}

// RankedGroup is one ordered retrieval source (by_vector, by_diff, by_message,
// or an entity-based result set). Items are in descending relevance order.
type RankedGroup struct {
	Key   string
	Items []ScoredCommit
}

// rrfK is the constant K in reciprocal-rank fusion. Standard RRF uses 60.
const rrfK = 60

// CollectContextGroups extracts per-source ranked result groups from a query
// response, preserving each source's ordering. Sources with no results are
// omitted and records without a SHA are dropped.
func CollectContextGroups(resp *Response) []RankedGroup {
	var groups []RankedGroup

	for _, key := range []string{returnKeyByVector, returnKeyByDiff, returnKeyByMessage, returnKeyCommits} {
		nodes := resp.Nodes(key)
		if len(nodes) == 0 {
			continue
		}

		items := make([]ScoredCommit, 0, len(nodes))
		for _, n := range nodes {
			rec := CommitRecordFromHelixData(n)
			if rec.SHA == "" {
				continue
			}

			items = append(items, ScoredCommit{Record: rec, Distance: n.Float64("distance")})
		}

		if len(items) > 0 {
			groups = append(groups, RankedGroup{Key: key, Items: items})
		}
	}

	return groups
}

// FuseContextRecords fuses ranked retrieval groups with reciprocal-rank fusion
// and re-ranks by recency. A commit present in multiple sources accumulates a
// vote per source (1/(K+rank)), so agreement across sources ranks above any
// single strong hit. Entity matches (Key == returnKeyCommits) are exact graph hits
// and vote as if top-ranked. When limit > 0 the result is capped.
func FuseContextRecords(groups []RankedGroup, limit int) []CommitRecord {
	type acc struct {
		rec       CommitRecord
		score     float64
		timestamp time.Time
	}

	scores := make(map[string]*acc)
	order := make([]string, 0, 8)

	for _, g := range groups {
		if len(g.Items) == 0 {
			continue
		}
		// Entity matches are precise: every hit votes as if it were top-ranked.
		entityGroup := g.Key == returnKeyCommits
		for i, item := range g.Items {
			sha := item.Record.SHA

			a := scores[sha]
			if a == nil {
				a = &acc{rec: item.Record, timestamp: item.Record.Timestamp}
				scores[sha] = a
				order = append(order, sha)
			}

			rank := float64(i + 1)
			if entityGroup {
				rank = 1
			}

			a.score += 1.0 / (rrfK + rank)
		}
	}

	recs := make([]CommitRecord, 0, len(order))
	for _, sha := range order {
		recs = append(recs, scores[sha].rec)
	}

	sort.SliceStable(recs, func(i, j int) bool {
		si, sj := scores[recs[i].SHA].score, scores[recs[j].SHA].score
		if si != sj {
			return si > sj
		}

		return recs[i].Timestamp.After(recs[j].Timestamp)
	})

	if limit > 0 && len(recs) > limit {
		recs = recs[:limit]
	}

	return recs
}

// ParseContextResults extracts CommitRecords from a HelixDB query response.
// Iterates over common result keys (by_diff, by_message, by_vector, commits)
// and deduplicates by SHA, preserving the fixed source-priority order. Prefer
// CollectContextGroups + FuseContextRecords for ranked hybrid retrieval.
func ParseContextResults(resp *Response) []CommitRecord {
	seen := make(map[string]bool)

	var records []CommitRecord

	for _, key := range []string{returnKeyByDiff, returnKeyByMessage, returnKeyByVector, returnKeyCommits} {
		for _, node := range resp.Nodes(key) {
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
