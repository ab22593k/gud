package mem

import (
	"strings"
	"testing"
	"time"

	helix "github.com/helixdb/helix-db/sdks/go"
)

func TestBuildContextQuery_ValidDiff(t *testing.T) {
	q := BuildContextQuery("/home/user/repo", "main",
		[]string{"main.go", "helper.go"},
		"diff --git a/main.go b/main.go",
	)
	if q == nil {
		t.Fatal("expected non-nil query")
	}

	err := q.Validate()
	if err != nil {
		t.Errorf("query validation failed: %v", err)
	}
}

func TestBuildContextQuery_NoDiffText(t *testing.T) {
	q := BuildContextQuery("/home/user/repo", "main",
		[]string{"main.go"},
		"",
	)
	if q == nil {
		t.Fatal("expected non-nil query")
	}

	err := q.Validate()
	if err != nil {
		t.Errorf("query validation failed: %v", err)
	}
}

func TestBuildContextQuery_NoFiles(t *testing.T) {
	q := BuildContextQuery("/home/user/repo", "main",
		nil,
		"diff --git a/main.go b/main.go",
	)
	if q == nil {
		t.Fatal("expected non-nil query")
	}

	err := q.Validate()
	if err != nil {
		t.Errorf("query validation failed: %v", err)
	}
}

func TestBuildHybridContextQuery_Valid(t *testing.T) {
	q := BuildHybridContextQuery("/home/user/repo", "main",
		[]float32{0.1, 0.2, 0.3, 0.4},
		"diff --git a/main.go b/main.go",
		[]string{"main.go"},
		5,
	)
	if q == nil {
		t.Fatal("expected non-nil query")
	}

	err := q.Validate()
	if err != nil {
		t.Errorf("query validation failed: %v", err)
	}
}

func TestBuildHybridContextQuery_NoVector(t *testing.T) {
	q := BuildHybridContextQuery("/home/user/repo", "",
		nil,
		"diff --git a/main.go b/main.go",
		nil,
		5,
	)
	if q == nil {
		t.Fatal("expected non-nil query")
	}

	err := q.Validate()
	if err != nil {
		t.Errorf("query validation failed: %v", err)
	}
}

func TestBuildHybridContextQuery_BranchScoping(t *testing.T) {
	q := BuildHybridContextQuery("/home/user/repo", "feature-x",
		[]float32{0.1, 0.2},
		"diff text",
		[]string{"main.go"},
		5,
	)

	data, err := helix.MarshalRequest(q)
	if err != nil {
		t.Fatalf("MarshalRequest: %v", err)
	}

	js := string(data)
	if !strings.Contains(js, "feature-x") {
		t.Errorf("expected branch 'feature-x' in serialized query, got:\n%s", js)
	}
}

func TestBuildEntityContextQuery_Valid(t *testing.T) {
	q := BuildEntityContextQuery("/repo", "main",
		[]string{"/repo:main.go:ParseInput"},
		5,
	)
	if q == nil {
		t.Fatal("expected non-nil query")
	}

	err := q.Validate()
	if err != nil {
		t.Errorf("query validation failed: %v", err)
	}
}

func TestBuildEntityContextQuery_BranchScoping(t *testing.T) {
	t.Parallel()

	t.Run("branch scopes and includes legacy records", func(t *testing.T) {
		q := BuildEntityContextQuery("/repo", "feature-x",
			[]string{"/repo:main.go:ParseInput"},
			5,
		)

		data, err := helix.MarshalRequest(q)
		if err != nil {
			t.Fatalf("MarshalRequest: %v", err)
		}

		js := string(data)
		// "feature-x" only appears via the branch filter, and the empty
		// string pins the legacy-record OR (branch == "") branchFilter
		// semantics, mirroring the hybrid query.
		if !strings.Contains(js, "feature-x") || !strings.Contains(js, `""`) {
			t.Errorf("expected branch filter with legacy empty-branch OR in serialized query, got:\n%s", js)
		}
	})

	t.Run("empty branch disables scoping", func(t *testing.T) {
		q := BuildEntityContextQuery("/repo", "",
			[]string{"/repo:main.go:ParseInput"},
			5,
		)

		data, err := helix.MarshalRequest(q)
		if err != nil {
			t.Fatalf("MarshalRequest: %v", err)
		}

		if strings.Contains(string(data), "feature-x") {
			t.Errorf("unexpected branch filter with empty branch:\n%s", data)
		}
	})
}

func TestBuildMemoryContextQuery_Valid(t *testing.T) {
	q := BuildMemoryContextQuery("tenant-acme", "user-alice", "prefers Go", 10)
	if q == nil {
		t.Fatal("expected non-nil query")
	}

	err := q.Validate()
	if err != nil {
		t.Errorf("query validation failed: %v", err)
	}
}

func TestBuildCategoryMemoriesQuery_Valid(t *testing.T) {
	q := BuildCategoryMemoriesQuery("tenant-acme:feat", "tenant-acme", 10)
	if q == nil {
		t.Fatal("expected non-nil query")
	}

	err := q.Validate()
	if err != nil {
		t.Errorf("query validation failed: %v", err)
	}
}

func TestParseContextResults_EmptyResponse(t *testing.T) {
	resp := NewResponse(map[string]any{})

	records := ParseContextResults(resp)
	if len(records) != 0 {
		t.Errorf("expected 0 records, got %d", len(records))
	}
}

func TestParseContextResults_ValidResponse(t *testing.T) {
	resp := NewResponse(map[string]any{
		"by_diff": []any{
			map[string]any{
				"$id":       float64(1),
				"id":        "abc123",
				"message":   "fix: handle edge case",
				"author":    "dev@example.com",
				"timestamp": float64(1719648000000),
				"repo_path": "/repo",
				"branch":    "main",
				"diff_stat": "1 file changed",
			},
		},
	})

	records := ParseContextResults(resp)
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}

	if records[0].SHA != "abc123" {
		t.Errorf("expected SHA abc123, got %q", records[0].SHA)
	}
}

func TestParseContextResults_TwoSources(t *testing.T) {
	resp := NewResponse(map[string]any{
		"by_diff": []any{
			map[string]any{"$id": float64(1), "id": "a", "message": "a msg",
				"author": "", "timestamp": float64(0), "repo_path": "", "branch": ""},
		},
		"by_message": []any{
			map[string]any{"$id": float64(2), "id": "b", "message": "b msg",
				"author": "", "timestamp": float64(0), "repo_path": "", "branch": ""},
		},
	})

	records := ParseContextResults(resp)
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
}

func TestParseContextResults_WithVector(t *testing.T) {
	resp := NewResponse(map[string]any{
		"by_vector": []any{
			map[string]any{"$id": float64(1), "id": "vec1", "message": "vec result",
				"author": "", "timestamp": float64(0), "repo_path": "", "branch": ""},
		},
	})

	records := ParseContextResults(resp)
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}

	if records[0].SHA != "vec1" {
		t.Errorf("expected SHA vec1, got %q", records[0].SHA)
	}
}

func TestFormatContextRecords_Empty(t *testing.T) {
	result := FormatContextRecords(nil)
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestFormatContextRecords_NotEmpty(t *testing.T) {
	records := []CommitRecord{
		{SHA: "abc123", Message: "feat: add login", Author: "alice", RepoPath: "/repo", Branch: "main"},
		{SHA: "def456", Message: "fix: typo", Author: "bob", RepoPath: "/repo", Branch: "main"},
	}

	result := FormatContextRecords(records)
	if result == "" {
		t.Fatal("expected non-empty result")
	}

	if !strings.Contains(result, "abc123") {
		t.Errorf("expected abc123 in output, got %q", result)
	}

	if !strings.Contains(result, "feat: add login") {
		t.Errorf("expected 'feat: add login' in output, got %q", result)
	}
}

func TestBuildContextQuery_BranchFilter(t *testing.T) {
	t.Parallel()

	t.Run("branch scopes and includes legacy records", func(t *testing.T) {
		q := BuildContextQuery("/home/user/repo", "feature-x",
			[]string{"main.go"},
			"diff text",
		)

		data, err := helix.MarshalRequest(q)
		if err != nil {
			t.Fatalf("MarshalRequest: %v", err)
		}

		js := string(data)
		if !strings.Contains(js, "feature-x") {
			t.Errorf("expected branch 'feature-x' in serialized query, got:\n%s", js)
		}
	})

	t.Run("empty branch disables scoping", func(t *testing.T) {
		q := BuildContextQuery("/home/user/repo", "",
			[]string{"main.go"},
			"diff text",
		)

		data, err := helix.MarshalRequest(q)
		if err != nil {
			t.Fatalf("MarshalRequest: %v", err)
		}

		if strings.Contains(string(data), "feature-x") {
			t.Errorf("unexpected branch filter with empty branch:\n%s", data)
		}
	})
}

func TestCollectContextGroups_EmptyResponse(t *testing.T) {
	groups := CollectContextGroups(NewResponse(map[string]any{}))
	if len(groups) != 0 {
		t.Errorf("expected 0 groups, got %d", len(groups))
	}
}

func TestCollectContextGroups_AllSources(t *testing.T) {
	resp := NewResponse(map[string]any{
		"by_vector": []any{
			map[string]any{"$id": float64(1), "id": "v1", "message": "vec", "distance": float64(0.1)},
			map[string]any{"$id": float64(2), "id": "v2", "message": "vec2", "distance": float64(0.5)},
		},
		"by_diff": []any{
			map[string]any{"$id": float64(3), "id": "d1", "message": "diff", "distance": float64(0.2)},
		},
		"by_message": []any{
			map[string]any{"$id": float64(4), "id": "m1", "message": "msg"},
		},
		"commits": []any{
			map[string]any{"$id": float64(5), "id": "e1", "message": "entity"},
		},
	})

	groups := CollectContextGroups(resp)
	if len(groups) != 4 {
		t.Fatalf("expected 4 groups, got %d", len(groups))
	}

	if groups[0].Key != "by_vector" || len(groups[0].Items) != 2 {
		t.Errorf("by_vector group wrong: %+v", groups[0])
	}

	if groups[0].Items[0].Record.SHA != "v1" || groups[0].Items[0].Distance != 0.1 {
		t.Errorf("by_vector item wrong: %+v", groups[0].Items[0])
	}

	if groups[3].Key != "commits" || len(groups[3].Items) != 1 {
		t.Errorf("commits group wrong: %+v", groups[3])
	}
}

func TestCollectContextGroups_SkipsEmptySHA(t *testing.T) {
	resp := NewResponse(map[string]any{
		"by_diff": []any{
			map[string]any{"$id": float64(1), "id": "ok", "message": "fine"},
			map[string]any{"$id": float64(2), "id": "", "message": "skip me"},
		},
	})

	groups := CollectContextGroups(resp)
	if len(groups) != 1 || len(groups[0].Items) != 1 {
		t.Fatalf("expected 1 group with 1 item, got %+v", groups)
	}

	if groups[0].Items[0].Record.SHA != "ok" {
		t.Errorf("expected 'ok', got %q", groups[0].Items[0].Record.SHA)
	}
}

func TestFuseContextRecords_CrossSourceAgreementRanksFirst(t *testing.T) {
	groups := []RankedGroup{
		{Key: "by_vector", Items: []ScoredCommit{{Record: CommitRecord{SHA: "a"}}, {Record: CommitRecord{SHA: "b"}}}},
		{Key: "by_diff", Items: []ScoredCommit{{Record: CommitRecord{SHA: "b"}}, {Record: CommitRecord{SHA: "c"}}}},
		{Key: "by_message", Items: []ScoredCommit{{Record: CommitRecord{SHA: "a"}}}},
	}

	recs := FuseContextRecords(groups, 0)
	if len(recs) != 3 {
		t.Fatalf("expected 3 unique records, got %d", len(recs))
	}
	// "a" appears in two sources → highest RRF score.
	if recs[0].SHA != "a" {
		t.Errorf("expected 'a' first (2 sources), got %q", recs[0].SHA)
	}
}

func TestFuseContextRecords_DedupsAcrossSources(t *testing.T) {
	groups := []RankedGroup{
		{Key: "by_vector", Items: []ScoredCommit{{Record: CommitRecord{SHA: "dup"}}}},
		{Key: "by_diff", Items: []ScoredCommit{{Record: CommitRecord{SHA: "dup"}}}},
	}

	recs := FuseContextRecords(groups, 0)
	if len(recs) != 1 {
		t.Fatalf("expected 1 deduped record, got %d", len(recs))
	}

	if recs[0].SHA != "dup" {
		t.Errorf("expected 'dup', got %q", recs[0].SHA)
	}
}

func TestFuseContextRecords_EntityMatchesVoteAsTopRanked(t *testing.T) {
	// Entity hits vote as rank-1, tying a top-ranked vector hit; give the
	// entity hit a newer timestamp so the recency tie-break picks it.
	oldTime := time.Unix(1_000, 0)
	newTime := time.Unix(2_000, 0)
	groups := []RankedGroup{
		{Key: "by_vector", Items: []ScoredCommit{
			{Record: CommitRecord{SHA: "top1", Timestamp: oldTime}},
			{Record: CommitRecord{SHA: "top2", Timestamp: oldTime}},
		}},
		{Key: "commits", Items: []ScoredCommit{{Record: CommitRecord{SHA: "entity-hit", Timestamp: newTime}}}},
	}

	recs := FuseContextRecords(groups, 0)
	if len(recs) != 3 {
		t.Fatalf("expected 3 records, got %d", len(recs))
	}
	// Entity hit scores 1/61, matching a rank-1 source vote, and wins the
	// recency tie with the vector rank-1 hit.
	if recs[0].SHA != "entity-hit" {
		t.Errorf("expected entity hit first, got %q", recs[0].SHA)
	}

	if recs[2].SHA != "top2" {
		t.Errorf("expected rank-2 vector hit last, got %q", recs[2].SHA)
	}
}

func TestFuseContextRecords_RecencyTieBreak(t *testing.T) {
	older := time.Unix(1_000, 0)
	newer := time.Unix(2_000, 0)
	// Both commits are rank-1 in their own source, so scores tie exactly and
	// recency decides.
	groups := []RankedGroup{
		{Key: "by_vector", Items: []ScoredCommit{{Record: CommitRecord{SHA: "old", Timestamp: older}}}},
		{Key: "by_message", Items: []ScoredCommit{{Record: CommitRecord{SHA: "new", Timestamp: newer}}}},
	}

	recs := FuseContextRecords(groups, 0)
	if len(recs) != 2 {
		t.Fatalf("expected 2 records, got %d", len(recs))
	}

	if recs[0].SHA != "new" {
		t.Errorf("expected 'new' first on recency tie-break, got %q", recs[0].SHA)
	}
}

func TestFuseContextRecords_Limit(t *testing.T) {
	groups := []RankedGroup{
		{Key: "by_vector", Items: []ScoredCommit{
			{Record: CommitRecord{SHA: "a"}},
			{Record: CommitRecord{SHA: "b"}},
			{Record: CommitRecord{SHA: "c"}},
			{Record: CommitRecord{SHA: "d"}},
		}},
	}

	recs := FuseContextRecords(groups, 2)
	if len(recs) != 2 {
		t.Fatalf("expected 2 records with limit 2, got %d", len(recs))
	}
}

func TestFuseContextRecords_EmptyAndNil(t *testing.T) {
	if recs := FuseContextRecords(nil, 0); len(recs) != 0 {
		t.Errorf("expected 0 records for nil groups, got %d", len(recs))
	}

	if recs := FuseContextRecords([]RankedGroup{{Key: "by_diff", Items: nil}}, 0); len(recs) != 0 {
		t.Errorf("expected 0 records for empty group, got %d", len(recs))
	}
}
