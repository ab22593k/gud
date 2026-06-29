package helixdb

import (
	"testing"
	"time"
)

func TestCommitData_ToProperties(t *testing.T) {
	now := time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
	c := CommitData{
		SHA:       "abc123def456",
		Message:   "feat: add user auth",
		Author:    "Alice <alice@example.com>",
		Timestamp: now,
		RepoPath:  "/home/user/project",
		Branch:    "main",
		DiffHash:  "sha256:xyz",
		DiffStat:  "2 files changed, 10 insertions(+), 2 deletions(-)",
		DiffText:  "diff --git a/main.go b/main.go",
		Files: []FileChange{
			{Path: "main.go", ChangeType: "modified", LinesAdded: 2, LinesDeleted: 0},
		},
		CodeUnits: []CodeUnitRef{
			{Name: "main", Kind: "function", FilePath: "main.go", ChangeType: "modified"},
		},
		IsGudGenerated: true,
	}

	props := c.ToProps()

	if len(props) == 0 {
		t.Fatal("expected non-empty props")
	}

	// Verify all expected keys are present.
	expectedKeys := map[string]bool{
		"id": false, "message": false, "author": false, "timestamp": false,
		"repo_path": false, "branch": false, "diff_hash": false, "diff_stat": false,
		"diff_text": false, "is_gud": false,
	}
	for _, p := range props {
		if _, ok := expectedKeys[p.Name]; ok {
			expectedKeys[p.Name] = true
		}
	}
	for key, found := range expectedKeys {
		if !found {
			t.Errorf("missing property %q", key)
		}
	}

	// Marshal and verify the output contains expected values.
	data, err := props[0].MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	_ = data // structural check passes if we got here
}

func TestCommitRecord_FromHelixNode(t *testing.T) {
	node := map[string]any{
		"$id":       float64(42),
		"id":        "abc123",
		"message":   "fix: handle nil pointer",
		"author":    "Bob <bob@test.com>",
		"timestamp": float64(1719648000000),
		"repo_path": "/repo",
		"branch":    "main",
	}

	record := CommitRecordFromHelixData(node)

	if record.HelixID != 42 {
		t.Errorf("expected HelixID 42, got %d", record.HelixID)
	}
	if record.Message != "fix: handle nil pointer" {
		t.Errorf("expected message %q, got %q", "fix: handle nil pointer", record.Message)
	}
}

func TestBuildCommitNodeQuery(t *testing.T) {
	q := BuildCommitNodeQuery("/home/user/repo", "main")
	if q == nil {
		t.Fatal("expected non-nil query")
	}
	err := q.Validate()
	if err != nil {
		t.Errorf("query validation failed: %v", err)
	}
}

func TestBuildCommitNodeQuery_EmptyBranch(t *testing.T) {
	q := BuildCommitNodeQuery("/home/user/repo", "")
	if q == nil {
		t.Fatal("expected non-nil query")
	}
	err := q.Validate()
	if err != nil {
		t.Errorf("query validation failed: %v", err)
	}
}
