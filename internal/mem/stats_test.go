package mem

import (
	"testing"
)

func TestBuildRepoSummaryQuery(t *testing.T) {
	q := BuildRepoSummaryQuery("/home/user/repo")
	if q == nil {
		t.Fatal("expected non-nil query")
	}
	err := q.Validate()
	if err != nil {
		t.Errorf("query validation failed: %v", err)
	}
}

func TestBuildAuthorStatsQuery(t *testing.T) {
	q := BuildAuthorStatsQuery("/home/user/repo")
	if q == nil {
		t.Fatal("expected non-nil query")
	}
	err := q.Validate()
	if err != nil {
		t.Errorf("query validation failed: %v", err)
	}
}

func TestBuildTopFilesQuery(t *testing.T) {
	q := BuildTopFilesQuery("/home/user/repo", 10)
	if q == nil {
		t.Fatal("expected non-nil query")
	}
	err := q.Validate()
	if err != nil {
		t.Errorf("query validation failed: %v", err)
	}
}

func TestFormatRepoSummary_Empty(t *testing.T) {
	result := FormatRepoSummary(RepoStats{})
	if result == "" {
		t.Fatal("expected non-empty summary header")
	}
}

func TestFormatRepoSummary_NotEmpty(t *testing.T) {
	stats := RepoStats{
		TotalCommits: 42,
		AuthorStats: []AuthorStat{
			{Email: "alice@example.com", Count: 24},
			{Email: "bob@example.com", Count: 18},
		},
		FileStats: []FileStat{
			{Path: "main.go", Changes: 12},
			{Path: "auth.go", Changes: 8},
		},
	}
	result := FormatRepoSummary(stats)
	if !contains(result, "42") {
		t.Errorf("expected '42' in output")
	}
	if !contains(result, "alice") {
		t.Errorf("expected 'alice' in output")
	}
	if !contains(result, "main.go") {
		t.Errorf("expected 'main.go' in output")
	}
}
