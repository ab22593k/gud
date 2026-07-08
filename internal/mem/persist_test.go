package mem

import (
	"testing"
	"time"
)

func TestBuildPersistQuery_Valid(t *testing.T) {
	data := CommitData{
		SHA:       "abc123def456",
		Message:   "feat: add login",
		Author:    "Alice <alice@example.com>",
		Timestamp: time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC),
		RepoPath:  "/home/user/project",
		Branch:    "main",
		DiffHash:  "sha256:xyz",
		DiffStat:  "1 file changed",
		DiffText:  "diff --git a/main.go b/main.go",
		IsGudGenerated: true,
	}

	q := BuildPersistCommitQuery(data)
	if q == nil {
		t.Fatal("expected non-nil query")
	}
	err := q.Validate()
	if err != nil {
		t.Errorf("query validation failed: %v", err)
	}
}

func TestBuildPersistQuery_WithFiles(t *testing.T) {
	data := CommitData{
		SHA:       "def789",
		Message:   "refactor: extract auth",
		Author:    "Bob <bob@example.com>",
		Timestamp: time.Date(2026, 6, 28, 15, 0, 0, 0, time.UTC),
		RepoPath:  "/home/user/project",
		Branch:    "feat/auth",
		DiffHash:  "sha256:abc",
		DiffStat:  "3 files changed",
		DiffText:  "diff --git a/auth.go b/auth.go",
		IsGudGenerated: false,
		Files: []FileChange{
			{Path: "auth.go", ChangeType: "modified", LinesAdded: 10, LinesDeleted: 2},
			{Path: "auth_test.go", ChangeType: "added", LinesAdded: 30, LinesDeleted: 0},
		},
	}

	q := BuildPersistCommitQuery(data)
	if q == nil {
		t.Fatal("expected non-nil query")
	}
	err := q.Validate()
	if err != nil {
		t.Errorf("query validation failed: %v", err)
	}
}

func TestBuildPersistCommitQuery_WithCodeUnits(t *testing.T) {
	data := CommitData{
		SHA:       "aaa111",
		Message:   "fix: handle nil in parse",
		Author:    "dev@example.com",
		Timestamp: time.Now(),
		RepoPath:  "/repo",
		Branch:    "main",
		DiffHash:  "h1",
		DiffStat:  "1 file changed",
		DiffText:  "diff --git a/parse.go b/parse.go",
		CodeUnits: []CodeUnitRef{
			{Name: "ParseInput", Kind: "function", FilePath: "parse.go", ChangeType: "modified"},
		},
	}

	q := BuildPersistCommitQuery(data)
	if q == nil {
		t.Fatal("expected non-nil query")
	}
	err := q.Validate()
	if err != nil {
		t.Errorf("query validation failed: %v", err)
	}
}

func TestBuildPersistMemoryQuery_Valid(t *testing.T) {
	now := time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
	data := MemoryData{
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

	q := BuildPersistMemoryQuery(data)
	if q == nil {
		t.Fatal("expected non-nil query")
	}
	err := q.Validate()
	if err != nil {
		t.Errorf("query validation failed: %v", err)
	}
}

func TestBuildCategorizeMemoryQuery_Valid(t *testing.T) {
	q := BuildCategorizeMemoryQuery("mem001", "tenant-acme", "tenant-acme:feat")
	if q == nil {
		t.Fatal("expected non-nil query")
	}
	err := q.Validate()
	if err != nil {
		t.Errorf("query validation failed: %v", err)
	}
}

func TestBuildMentionEntityQuery_Valid(t *testing.T) {
	q := BuildMentionEntityQuery("mem001", "tenant-acme", "tenant-acme:ParseInput")
	if q == nil {
		t.Fatal("expected non-nil query")
	}
	err := q.Validate()
	if err != nil {
		t.Errorf("query validation failed: %v", err)
	}
}

func TestBuildUpdateMemoryQuery_Valid(t *testing.T) {
	now := time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
	newData := MemoryData{
		MemoryID:  "mem002",
		Content:   "User prefers Go for backend services (updated)",
		UserID:    "user-alice",
		Kind:      MemoryPreference,
		Salience:  0.9,
		CreatedAt: now,
		UpdatedAt: now,
	}

	q := BuildUpdateMemoryQuery("mem001", "tenant-acme", newData)
	if q == nil {
		t.Fatal("expected non-nil query")
	}
	err := q.Validate()
	if err != nil {
		t.Errorf("query validation failed: %v", err)
	}
}

func TestBuildSoftDeleteMemoryQuery_Valid(t *testing.T) {
	q := BuildSoftDeleteMemoryQuery("mem001", "tenant-acme")
	if q == nil {
		t.Fatal("expected non-nil query")
	}
	err := q.Validate()
	if err != nil {
		t.Errorf("query validation failed: %v", err)
	}
}

func TestFormatDiffStat_Empty(t *testing.T) {
	result := FormatDiffStat(nil)
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestFormatDiffStat_NotEmpty(t *testing.T) {
	files := []FileChange{
		{Path: "main.go", ChangeType: "modified", LinesAdded: 5, LinesDeleted: 3},
		{Path: "helper.go", ChangeType: "added", LinesAdded: 20, LinesDeleted: 0},
	}
	result := FormatDiffStat(files)
	if result == "" {
		t.Fatal("expected non-empty result")
	}
	if !contains(result, "main.go") {
		t.Errorf("expected main.go in output")
	}
	if !contains(result, "helper.go") {
		t.Errorf("expected helper.go in output")
	}
}
