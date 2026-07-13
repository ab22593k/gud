package mem

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
		IsLatest:       true,
		Embedding:      []float32{0.1, 0.2, 0.3},
	}

	props := c.ToProps()

	if len(props) == 0 {
		t.Fatal("expected non-empty props")
	}

	expectedKeys := map[string]bool{
		"id": false, "message": false, "author": false, "timestamp": false,
		"repo_path": false, "branch": false, "diff_hash": false, "diff_stat": false,
		"diff_text": false, "is_gud": false, "tenantId": false, "isLatest": false,
		"embedding": false,
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
}

func TestCommitData_ToProperties_NoEmbedding(t *testing.T) {
	now := time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
	c := CommitData{
		SHA:       "abc123",
		Message:   "fix: handle nil",
		Author:    "dev@example.com",
		Timestamp: now,
		RepoPath:  "/repo",
		Branch:    "main",
		DiffText:  "diff --git a/main.go b/main.go",
		IsLatest:  true,
	}

	props := c.ToProps()
	found := false
	for _, p := range props {
		if p.Name == "embedding" {
			found = true
		}
	}
	if found {
		t.Error("expected no embedding property when nil")
	}
}

func TestCommitData_ToProperties_WithLifecycle(t *testing.T) {
	now := time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
	del := now.Add(-24 * time.Hour)
	c := CommitData{
		SHA:       "xyz789",
		Message:   "old commit",
		Author:    "test@test.com",
		Timestamp: del,
		RepoPath:  "/repo",
		IsLatest:  false,
		DeletedAt: &del,
	}

	props := c.ToProps()
	foundDeleted := false
	for _, p := range props {
		if p.Name == "deletedAt" {
			foundDeleted = true
		}
	}
	if !foundDeleted {
		t.Error("expected deletedAt property")
	}
}

func TestMemoryData_ToProps(t *testing.T) {
	now := time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
	m := MemoryData{
		MemoryID:  "mem001",
		Content:   "User prefers Go for backend services",
		TenantID:  "tenant-acme",
		UserID:    "user-alice",
		Kind:      MemoryPreference,
		Salience:  0.85,
		IsLatest:  true,
		Embedding: []float32{0.1, 0.2, 0.3, 0.4},
		CreatedAt: now,
		UpdatedAt: now,
	}

	props := m.ToProps()
	expectedKeys := map[string]bool{
		"memoryId": false, "content": false, "tenantId": false, "userId": false,
		"kind": false, "salience": false, "isLatest": false, "createdAt": false,
		"updatedAt": false, "embedding": false,
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
}

func TestCategoryData_ToProps(t *testing.T) {
	cat := CategoryData{
		CategoryKey: "tenant-acme:feat",
		TenantID:    "tenant-acme",
		Name:        "feature",
		Description: "New feature commits",
	}

	props := cat.ToProps()
	if len(props) == 0 {
		t.Fatal("expected non-empty props")
	}
}

func TestEntityData_ToProps(t *testing.T) {
	ent := EntityData{
		EntityKey: "tenant-acme:NewServer",
		TenantID:  "tenant-acme",
		Name:      "NewServer",
		Kind:      "function",
	}

	props := ent.ToProps()
	if len(props) == 0 {
		t.Fatal("expected non-empty props")
	}
}

func TestCommitRecord_FromHelixNode(t *testing.T) {
	node := Node{data: map[string]any{
		"$id":       float64(42),
		"id":        "abc123",
		"message":   "fix: handle nil pointer",
		"author":    "Bob <bob@test.com>",
		"timestamp": float64(1719648000000),
		"repo_path": "/repo",
		"branch":    "main",
	}}

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
