package mem

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// startEmbeddedDB opens an isolated embedded database in a temp dir.
// Skips when native bindings are unavailable or integration is not enabled.
func startEmbeddedDB(t *testing.T) *DB {
	t.Helper()

	if os.Getenv("RUN_HELIXDB_INTEGRATION") == "" {
		t.Skip("set RUN_HELIXDB_INTEGRATION=1 to run")
	}

	db := NewDB(Options{DataDir: t.TempDir(), Database: "gud-test", Enabled: true})
	if !db.Enabled() {
		t.Skip("embedded native bindings unavailable")
	}

	t.Cleanup(func() { _ = db.Close() })

	if err := db.EnsureSchema(context.Background()); err != nil {
		_ = db.Close()

		t.Fatalf("EnsureSchema failed: %v", err)
	}

	return db
}

func TestIntegration_EnsureSchema(t *testing.T) {
	db := startEmbeddedDB(t)

	ctx := context.Background()
	if err := db.EnsureSchema(ctx); err != nil {
		t.Fatalf("EnsureSchema failed: %v", err)
	}

	t.Log("schema ensured successfully")
}

func TestIntegration_PersistAndQueryCommit(t *testing.T) {
	db := startEmbeddedDB(t)

	ctx := context.Background()

	commit := CommitData{
		SHA:            "abc123",
		Message:        "feat: add login endpoint",
		Author:         "dev@example.com",
		RepoPath:       "/test/repo",
		DiffText:       "@@ -1,5 +1,6 @@ func Login() {",
		Timestamp:      time.Now(),
		IsGudGenerated: true,
		Files: []FileChange{
			{Path: "auth/login.go", ChangeType: "added"},
		},
	}

	q := BuildPersistCommitQuery(commit)

	var persistResp map[string]any
	if err := db.Exec(ctx, q, &persistResp); err != nil {
		t.Fatalf("persist commit failed: %v", err)
	}

	t.Log("commit persisted successfully")

	summaryQ := BuildRepoSummaryQuery(commit.RepoPath)

	var rawSummaryResp map[string]any
	if err := db.Exec(ctx, summaryQ, &rawSummaryResp); err != nil {
		t.Fatalf("repo summary query failed: %v", err)
	}

	stats := ParseRepoSummary(NewResponse(rawSummaryResp))
	if stats.TotalCommits < 1 {
		t.Errorf("expected at least 1 commit, got %d", stats.TotalCommits)
	}

	output := FormatRepoSummary(stats)
	if !strings.Contains(output, "dev@example.com") {
		t.Errorf("expected author in output, got: %s", output)
	}

	t.Logf("summary:\n%s", output)
}

func TestIntegration_AuthorStats(t *testing.T) {
	db := startEmbeddedDB(t)

	ctx := context.Background()

	for _, c := range []CommitData{
		{
			SHA: "abc001", Message: "first", Author: "alice@example.com",
			RepoPath: "/test/repo", Timestamp: time.Now(), IsGudGenerated: true,
		},
		{
			SHA: "abc002", Message: "second", Author: "bob@example.com",
			RepoPath: "/test/repo", Timestamp: time.Now(), IsGudGenerated: true,
		},
	} {
		q := BuildPersistCommitQuery(c)
		if err := db.Exec(ctx, q, nil); err != nil {
			t.Fatalf("persist commit %s failed: %v", c.SHA, err)
		}
	}

	q := BuildAuthorStatsQuery("/test/repo")

	var rawResp map[string]any
	if err := db.Exec(ctx, q, &rawResp); err != nil {
		t.Fatalf("author stats query failed: %v", err)
	}

	stats := ParseAuthorStats(NewResponse(rawResp))
	if len(stats) < 2 {
		t.Errorf("expected at least 2 authors, got %d", len(stats))
	}

	output := FormatAuthorStats(stats)
	if !strings.Contains(output, "alice") || !strings.Contains(output, "bob") {
		t.Errorf("expected both authors in output, got: %s", output)
	}

	t.Logf("author stats:\n%s", output)
}

func TestIntegration_Trends(t *testing.T) {
	db := startEmbeddedDB(t)

	ctx := context.Background()

	now := time.Now()
	for _, c := range []CommitData{
		{
			SHA: "tr001", Message: "first today",
			Author: "dev@example.com", RepoPath: "/test/repo",
			Timestamp: now, IsGudGenerated: true,
		},
		{
			SHA: "tr002", Message: "second today",
			Author: "dev@example.com", RepoPath: "/test/repo",
			Timestamp: now.Add(1 * time.Second), IsGudGenerated: true,
		},
	} {
		q := BuildPersistCommitQuery(c)
		if err := db.Exec(ctx, q, nil); err != nil {
			t.Fatalf("persist commit %s failed: %v", c.SHA, err)
		}
	}

	q := BuildTrendsQuery("/test/repo")

	var rawResp map[string]any
	if err := db.Exec(ctx, q, &rawResp); err != nil {
		t.Fatalf("trends query failed: %v", err)
	}

	trends := ParseTrends(NewResponse(rawResp))
	if len(trends) == 0 {
		t.Fatal("expected at least 1 trend point")
	}

	output := FormatTrends(trends)

	expectedDate := now.Format("2006-01-02")
	if !strings.Contains(output, expectedDate) {
		t.Errorf("expected date %s in output, got: %s", expectedDate, output)
	}

	t.Logf("trends:\n%s", output)
}
