package mem

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

const testPort = "2232"

// TestMain is only responsible for the integration test binary.
// Unit tests (Test functions without the Integration prefix) run
// independently and are not affected by this file since Go test
// only calls TestMain once per package.

// startManagedContainer starts a HelixDB container via ContainerManager and returns cleanup.
func startManagedContainer(t *testing.T) (*ContainerManager, *DB) {
	t.Helper()

	mgr := NewContainerManager("gud-helixdb-int", testPort)

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()

	url, err := mgr.EnsureRunning(ctx)
	if err != nil {
		_ = mgr.Stop(context.Background()) // best-effort teardown

		t.Fatalf("EnsureRunning: %v", err)
	}

	db := NewDB(Options{BaseURL: url, Enabled: true})
	if !db.Enabled() {
		_ = mgr.Stop(context.Background()) // best-effort teardown

		t.Fatal("DB not enabled after EnsureRunning")
	}

	return mgr, db
}

func TestIntegration_EnsureSchema(t *testing.T) {
	if os.Getenv("RUN_HELIXDB_INTEGRATION") == "" {
		t.Skip("set RUN_HELIXDB_INTEGRATION=1 to run")
	}

	mgr, db := startManagedContainer(t)
	defer func() { _ = mgr.Stop(context.Background()) }() // best-effort teardown

	ctx := context.Background()
	if err := db.EnsureSchema(ctx); err != nil {
		t.Fatalf("EnsureSchema failed: %v", err)
	}

	t.Log("schema ensured successfully")
}

func TestIntegration_PersistAndQueryCommit(t *testing.T) {
	if os.Getenv("RUN_HELIXDB_INTEGRATION") == "" {
		t.Skip("set RUN_HELIXDB_INTEGRATION=1 to run")
	}

	mgr, db := startManagedContainer(t)
	defer func() { _ = mgr.Stop(context.Background()) }() // best-effort teardown

	ctx := context.Background()
	if err := db.EnsureSchema(ctx); err != nil {
		t.Fatalf("EnsureSchema failed: %v", err)
	}

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
	if os.Getenv("RUN_HELIXDB_INTEGRATION") == "" {
		t.Skip("set RUN_HELIXDB_INTEGRATION=1 to run")
	}

	mgr, db := startManagedContainer(t)
	defer func() { _ = mgr.Stop(context.Background()) }() // best-effort teardown

	ctx := context.Background()
	if err := db.EnsureSchema(ctx); err != nil {
		t.Fatalf("EnsureSchema failed: %v", err)
	}

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
	if os.Getenv("RUN_HELIXDB_INTEGRATION") == "" {
		t.Skip("set RUN_HELIXDB_INTEGRATION=1 to run")
	}

	mgr, db := startManagedContainer(t)
	defer func() { _ = mgr.Stop(context.Background()) }() // best-effort teardown

	ctx := context.Background()
	if err := db.EnsureSchema(ctx); err != nil {
		t.Fatalf("EnsureSchema failed: %v", err)
	}

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

const containerMgrTestPort = "16970"

func TestIntegration_ContainerManager(t *testing.T) {
	if os.Getenv("RUN_HELIXDB_INTEGRATION") == "" {
		t.Skip("set RUN_HELIXDB_INTEGRATION=1 to run")
	}

	ctx := context.Background()

	mgr := NewContainerManager("gud-helixdb-int-mgr", containerMgrTestPort)

	// EnsureRunning should start a new container.
	url, err := mgr.EnsureRunning(ctx)
	if err != nil {
		t.Fatalf("EnsureRunning failed: %v", err)
	}

	if url != "http://localhost:"+containerMgrTestPort {
		t.Errorf("expected url http://localhost:%s, got %s", testPort, url)
	}

	if !mgr.StartedByUs() {
		t.Error("expected StartedByUs to be true after EnsureRunning")
	}

	if !mgr.IsRunning(ctx) {
		t.Error("expected IsRunning to be true after EnsureRunning")
	}

	// Calling EnsureRunning again should be a no-op (container already running).
	url2, err := mgr.EnsureRunning(ctx)
	if err != nil {
		t.Fatalf("EnsureRunning (second call) failed: %v", err)
	}

	if url2 != url {
		t.Errorf("expected same url, got %s", url2)
	}

	// Stop should work and mark container as gone.
	if err := mgr.Stop(ctx); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	if mgr.StartedByUs() {
		t.Error("expected StartedByUs to be false after Stop")
	}

	if mgr.IsRunning(ctx) {
		t.Error("expected IsRunning to be false after Stop")
	}
}
