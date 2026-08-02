package mem

import (
	"strings"
	"testing"

	"github.com/helixdb/helix-db/sdks/go"
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
	q := BuildHybridContextQuery("/home/user/repo",
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
	q := BuildHybridContextQuery("/home/user/repo",
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

func TestBuildEntityContextQuery_Valid(t *testing.T) {
	q := BuildEntityContextQuery("/repo",
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
			map[string]any{"$id": float64(1), "id": "a", "message": "a msg", "author": "", "timestamp": float64(0), "repo_path": "", "branch": ""},
		},
		"by_message": []any{
			map[string]any{"$id": float64(2), "id": "b", "message": "b msg", "author": "", "timestamp": float64(0), "repo_path": "", "branch": ""},
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
			map[string]any{"$id": float64(1), "id": "vec1", "message": "vec result", "author": "", "timestamp": float64(0), "repo_path": "", "branch": ""},
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
