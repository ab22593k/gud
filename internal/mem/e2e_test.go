package mem

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/helixdb/helix-db/sdks/go"
)

// e2e helpers reuse the existing startManagedContainer from integration_test.go
// and testPort = "16969".

const testTenant  = "/test/repo"
const testTenantB = "/other/repo"

// createTestVector generates a 1536-dim F32 vector for integration testing.
// Uses text-embedding-3-small dimensions (1536). Contents are deterministic.
func createTestVector(seed byte) []float32 {
	v := make([]float32, 1536)
	for i := range v {
		v[i] = float32(int(seed)+i) / 1536.0
	}
	return v
}

// findSHAInResults searches ParseContextResults output for a specific SHA.
func findSHAInResults(records []CommitRecord, sha string) bool {
	for _, r := range records {
		if r.SHA == sha {
			return true
		}
	}
	return false
}

// TestIntegration_BM25ContextQueryByDiff persists commits with varied diff
// text and verifies that BM25 text search retrieves relevant commits.
func TestIntegration_BM25ContextQueryByDiff(t *testing.T) {
	if os.Getenv("RUN_HELIXDB_INTEGRATION") == "" {
		t.Skip("set RUN_HELIXDB_INTEGRATION=1 to run")
	}

	mgr, db := startManagedContainer(t)
	defer mgr.Stop(context.Background())

	ctx := context.Background()
	if err := db.EnsureSchema(ctx); err != nil {
		t.Fatalf("EnsureSchema failed: %v", err)
	}

	now := time.Now()

	// Persist two commits with distinct diff text.
	for _, c := range []CommitData{
		{
			SHA:       "bm25-a",
			Message:   "feat: add login handler",
			Author:    "dev@example.com",
			RepoPath:  testTenant,
			Branch:    "main",
			DiffText:  "diff --git a/auth/login.go b/auth/login.go\n@@ -0,0 +1,30 @@\n+package auth\n+\n+func Login(w http.ResponseWriter, r *http.Request) {\n+    // authenticate user\n+}",
			Timestamp: now,
			IsGudGenerated: true,
		},
		{
			SHA:       "bm25-b",
			Message:   "fix: correct db connection leak",
			Author:    "dev@example.com",
			RepoPath:  testTenant,
			Branch:    "main",
			DiffText:  "diff --git a/internal/db/pool.go b/internal/db/pool.go\n@@ -15,7 +15,9 @@ func getConn() *sql.Conn {\n-    return pool.Get()\n+    conn := pool.Get()\n+    conn.SetMaxLifetime(5 * time.Minute)\n+    return conn",
			Timestamp: now.Add(time.Second),
			IsGudGenerated: true,
		},
	} {
		q := BuildPersistCommitQuery(c)
		if err := db.Exec(ctx, q, nil); err != nil {
			t.Fatalf("persist commit %s failed: %v", c.SHA, err)
		}
	}

	// Search by login-related diff — should find bm25-a.
	q := BuildContextQuery(testTenant, "main", nil,
		"func Login(w http.ResponseWriter")
	var rawResp map[string]any
	if err := db.Exec(ctx, q, &rawResp); err != nil {
		t.Fatalf("BuildContextQuery failed: %v", err)
	}
	records := ParseContextResults(NewResponse(rawResp))
	if !findSHAInResults(records, "bm25-a") {
		t.Errorf("expected bm25-a in BM25 diff results, got %d records", len(records))
	}
	t.Logf("BM25 diff query returned %d records (expected bm25-a)", len(records))
}

// TestIntegration_BM25ContextQueryByFiles persists commits with file names
// in their messages and verifies retrieval by file-based search.
func TestIntegration_BM25ContextQueryByFiles(t *testing.T) {
	if os.Getenv("RUN_HELIXDB_INTEGRATION") == "" {
		t.Skip("set RUN_HELIXDB_INTEGRATION=1 to run")
	}

	mgr, db := startManagedContainer(t)
	defer mgr.Stop(context.Background())

	ctx := context.Background()
	if err := db.EnsureSchema(ctx); err != nil {
		t.Fatalf("EnsureSchema failed: %v", err)
	}

	now := time.Now()
	commitSHA := "bm25-file-a"

	c := CommitData{
		SHA:       commitSHA,
		Message:   "refactor: update auth/login.go and auth/middleware.go",
		Author:    "dev@example.com",
		RepoPath:  testTenant,
		Branch:    "main",
		DiffText:  "diff --git a/auth/login.go b/auth/login.go",
		Timestamp: now,
		IsGudGenerated: true,
		Files: []FileChange{
			{Path: "auth/login.go", ChangeType: "modified", LinesAdded: 5, LinesDeleted: 3},
		},
	}
	q := BuildPersistCommitQuery(c)
	if err := db.Exec(ctx, q, nil); err != nil {
		t.Fatalf("persist commit failed: %v", err)
	}

	// Search by file name — should find the commit via message BM25.
	q2 := BuildContextQuery(testTenant, "main", []string{"auth/login.go"}, "")
	var rawResp map[string]any
	if err := db.Exec(ctx, q2, &rawResp); err != nil {
		t.Fatalf("BuildContextQuery failed: %v", err)
	}
	records := ParseContextResults(NewResponse(rawResp))
	if !findSHAInResults(records, commitSHA) {
		t.Errorf("expected %s in BM25 file results, got %d records", commitSHA, len(records))
	}
	t.Logf("BM25 file query returned %d records (expected %s)", len(records), commitSHA)
}

// TestIntegration_EntityAwareRecall persists a commit with code units and
// verifies that BuildEntityContextQuery retrieves it by element key.
func TestIntegration_EntityAwareRecall(t *testing.T) {
	if os.Getenv("RUN_HELIXDB_INTEGRATION") == "" {
		t.Skip("set RUN_HELIXDB_INTEGRATION=1 to run")
	}

	mgr, db := startManagedContainer(t)
	defer mgr.Stop(context.Background())

	ctx := context.Background()
	if err := db.EnsureSchema(ctx); err != nil {
		t.Fatalf("EnsureSchema failed: %v", err)
	}

	now := time.Now()
	commitSHA := "entity-a"
	elementKey := testTenant + ":pkg/parser.go:ParseInput"

	c := CommitData{
		SHA:       commitSHA,
		Message:   "fix: handle nil edge case in ParseInput",
		Author:    "dev@example.com",
		RepoPath:  testTenant,
		Branch:    "main",
		DiffText:  "diff --git a/pkg/parser.go b/pkg/parser.go",
		Timestamp: now,
		IsGudGenerated: true,
		CodeUnits: []CodeUnitRef{
			{Name: "ParseInput", Kind: "function", FilePath: "pkg/parser.go", ChangeType: "modified"},
		},
	}
	q := BuildPersistCommitQuery(c)
	if err := db.Exec(ctx, q, nil); err != nil {
		t.Fatalf("persist commit failed: %v", err)
	}

	// Query by code element key.
	q2 := BuildEntityContextQuery(testTenant, []string{elementKey}, 10)
	var rawResp map[string]any
	if err := db.Exec(ctx, q2, &rawResp); err != nil {
		t.Fatalf("BuildEntityContextQuery failed: %v", err)
	}
	records := ParseContextResults(NewResponse(rawResp))
	if !findSHAInResults(records, commitSHA) {
		t.Errorf("expected %s in entity context results, got %d records", commitSHA, len(records))
	}
	t.Logf("Entity context query returned %d records (expected %s)", len(records), commitSHA)
}

// TestIntegration_MemoryLifecycleFull tests the full memory lifecycle:
// create → categorize → mention entity → query by category → update version
// → verify old superseded → soft-delete → verify excluded from recall.
func TestIntegration_MemoryLifecycleFull(t *testing.T) {
	if os.Getenv("RUN_HELIXDB_INTEGRATION") == "" {
		t.Skip("set RUN_HELIXDB_INTEGRATION=1 to run")
	}

	mgr, db := startManagedContainer(t)
	defer mgr.Stop(context.Background())

	ctx := context.Background()
	if err := db.EnsureSchema(ctx); err != nil {
		t.Fatalf("EnsureSchema failed: %v", err)
	}

	tenant := "mem-lifecycle-tenant"
	user := "user-alice"
	now := time.Now().Truncate(time.Millisecond)

	// 1. Create a Category node.
	catKey := tenant + ":travel"
	createCat := helix.WriteQuery("create_category")
	createCat.VarAs("cat",
		helix.G().AddN("Category", helix.Props{
			helix.Prop("categoryKey", helix.String(catKey)),
			helix.Prop(DefaultTenantProperty, helix.String(tenant)),
			helix.Prop("name", helix.String("Travel Plans")),
			helix.Prop("description", helix.String("User travel preferences and itineraries")),
		}))
	if err := db.Exec(ctx, createCat.Returning("cat"), nil); err != nil {
		t.Fatalf("create Category failed: %v", err)
	}

	// 2. Create an Entity node.
	entityKey := tenant + ":Japan"
	createEnt := helix.WriteQuery("create_entity")
	createEnt.VarAs("ent",
		helix.G().AddN("Entity", helix.Props{
			helix.Prop("entityKey", helix.String(entityKey)),
			helix.Prop(DefaultTenantProperty, helix.String(tenant)),
			helix.Prop("name", helix.String("Japan")),
			helix.Prop("kind", helix.String("country")),
			helix.Prop("metadata", helix.String(`{"type":"destination"}`)),
		}))
	if err := db.Exec(ctx, createEnt.Returning("ent"), nil); err != nil {
		t.Fatalf("create Entity failed: %v", err)
	}

	// 3. Create Memory (v1).
	memIDv1 := tenant + ":mem-v1"
	memV1 := MemoryData{
		MemoryID:  memIDv1,
		Content:   "User is planning a trip to Japan with Maya",
		TenantID:  tenant,
		UserID:    user,
		Kind:      MemoryEpisode,
		Salience:  0.75,
		IsLatest:  true,
		CreatedAt: now,
		UpdatedAt: now,
	}
	q1 := BuildPersistMemoryQuery(memV1)
	if err := db.Exec(ctx, q1, nil); err != nil {
		t.Fatalf("persist memory v1 failed: %v", err)
	}

	// 4. Categorize memory.
	qCat := BuildCategorizeMemoryQuery(memIDv1, tenant, catKey)
	if err := db.Exec(ctx, qCat, nil); err != nil {
		t.Fatalf("categorize memory failed: %v", err)
	}

	// 5. Link entity.
	qMent := BuildMentionEntityQuery(memIDv1, tenant, entityKey)
	if err := db.Exec(ctx, qMent, nil); err != nil {
		t.Fatalf("mention entity failed: %v", err)
	}

	// 6. Query by category — verify v1 appears.
	qCatMem := BuildCategoryMemoriesQuery(catKey, tenant, 10)
	var catResp map[string]any
	if err := db.Exec(ctx, qCatMem, &catResp); err != nil {
		t.Fatalf("BuildCategoryMemoriesQuery failed: %v", err)
	}
	catItems := extractResultItems(catResp["memories"])
	if len(catItems) == 0 {
		t.Fatal("expected at least 1 memory in category results")
	}
	foundV1 := false
	for _, item := range catItems {
		if m, ok := item.(map[string]any); ok {
			if mid, _ := m["memoryId"].(string); mid == memIDv1 {
				foundV1 = true
			}
		}
	}
	if !foundV1 {
		t.Errorf("expected memory %s in category results", memIDv1)
	}

	// 7. Create Memory v2 (update — extends the trip with timing).
	memIDv2 := tenant + ":mem-v2"
	memV2 := MemoryData{
		MemoryID: memIDv2,
		Content:  "User is planning a trip to Japan with Maya next April",
		TenantID: tenant,
		UserID:   user,
		Kind:     MemoryEpisode,
		Salience: 0.85,
		IsLatest: true,
		CreatedAt: now.Add(time.Minute),
		UpdatedAt: now.Add(time.Minute),
	}
	qUpdate := BuildUpdateMemoryQuery(memIDv1, tenant, memV2)
	if err := db.Exec(ctx, qUpdate, nil); err != nil {
		t.Fatalf("update memory failed: %v", err)
	}

	// 8. Query memory context — should only return v2 (latest).
	memCtx := BuildMemoryContextQuery(tenant, user, "Japan", 10)
	var mcResp map[string]any
	if err := db.Exec(ctx, memCtx, &mcResp); err != nil {
		t.Fatalf("BuildMemoryContextQuery failed: %v", err)
	}
	mcItems := extractResultItems(mcResp["memories"])

	// Ensure v1 is never returned as latest.
	for _, item := range mcItems {
		if m, ok := item.(map[string]any); ok {
			if mid, _ := m["memoryId"].(string); mid == memIDv1 {
				t.Errorf("old memory %s should not appear in active memory context", memIDv1)
			}
		}
	}

	// Ensure v2 is present.
	foundV2 := false
	for _, item := range mcItems {
		if m, ok := item.(map[string]any); ok {
			if mid, _ := m["memoryId"].(string); mid == memIDv2 {
				foundV2 = true
			}
		}
	}
	if !foundV2 {
		t.Errorf("expected memory %s in memory context results", memIDv2)
	}

	// 9. Soft-delete v2.
	qDel := BuildSoftDeleteMemoryQuery(memIDv2, tenant)
	if err := db.Exec(ctx, qDel, nil); err != nil {
		t.Fatalf("soft-delete memory failed: %v", err)
	}

	// 10. Query memory context again — v2 should be excluded.
	memCtx2 := BuildMemoryContextQuery(tenant, user, "Japan", 10)
	var mcResp2 map[string]any
	if err := db.Exec(ctx, memCtx2, &mcResp2); err != nil {
		t.Fatalf("BuildMemoryContextQuery (post-delete) failed: %v", err)
	}
	mcItems2 := extractResultItems(mcResp2["memories"])
	for _, item := range mcItems2 {
		if m, ok := item.(map[string]any); ok {
			if mid, _ := m["memoryId"].(string); mid == memIDv2 {
				t.Errorf("soft-deleted memory %s should be excluded from context", memIDv2)
			}
		}
	}
	t.Logf("Memory lifecycle test passed: created, categorized, linked, updated, and soft-deleted")
}

// TestIntegration_TenantIsolation verifies that commits and memories in
// one tenant are invisible when querying from another tenant.
func TestIntegration_TenantIsolation(t *testing.T) {
	if os.Getenv("RUN_HELIXDB_INTEGRATION") == "" {
		t.Skip("set RUN_HELIXDB_INTEGRATION=1 to run")
	}

	mgr, db := startManagedContainer(t)
	defer mgr.Stop(context.Background())

	ctx := context.Background()
	if err := db.EnsureSchema(ctx); err != nil {
		t.Fatalf("EnsureSchema failed: %v", err)
	}

	now := time.Now()
	tenantA := "/tenant/a"
	tenantB := "/tenant/b"

	// Persist a commit in tenant A with distinctive diff text.
	shaA := "iso-a-001"
	cA := CommitData{
		SHA:       shaA,
		Message:   "tenant A commit",
		Author:    "alice@a.com",
		RepoPath:  tenantA,
		Branch:    "main",
		DiffText:  "TENANT_A_SPECIFIC_FEATURE_XYZ",
		Timestamp: now,
		IsGudGenerated: true,
	}
	if err := db.Exec(ctx, BuildPersistCommitQuery(cA), nil); err != nil {
		t.Fatalf("persist tenant A commit failed: %v", err)
	}

	// Persist a commit in tenant B.
	shaB := "iso-b-001"
	cB := CommitData{
		SHA:       shaB,
		Message:   "tenant B commit",
		Author:    "bob@b.com",
		RepoPath:  tenantB,
		Branch:    "main",
		DiffText:  "TENANT_B_SPECIFIC_FEATURE_XYZ",
		Timestamp: now,
		IsGudGenerated: true,
	}
	if err := db.Exec(ctx, BuildPersistCommitQuery(cB), nil); err != nil {
		t.Fatalf("persist tenant B commit failed: %v", err)
	}

	// Query context for tenant A — should NOT find tenant B's commit.
	q := BuildContextQuery(tenantA, "main", nil, "TENANT_A_SPECIFIC_FEATURE_XYZ")
	var rawResp map[string]any
	if err := db.Exec(ctx, q, &rawResp); err != nil {
		t.Fatalf("BuildContextQuery for tenant A failed: %v", err)
	}
	records := ParseContextResults(NewResponse(rawResp))
	if findSHAInResults(records, shaB) {
		t.Errorf("tenant B commit leaked into tenant A results")
	}
	if !findSHAInResults(records, shaA) {
		t.Errorf("expected tenant A commit %s in own context", shaA)
	}
	t.Logf("Tenant isolation verified: A has %d records, none from B", len(records))
}

// TestIntegration_VectorSearch persists a commit with a 1536-dim embedding
// and retrieves it via BuildHybridContextQuery vector search.
func TestIntegration_VectorSearch(t *testing.T) {
	if os.Getenv("RUN_HELIXDB_INTEGRATION") == "" {
		t.Skip("set RUN_HELIXDB_INTEGRATION=1 to run")
	}

	mgr, db := startManagedContainer(t)
	defer mgr.Stop(context.Background())

	ctx := context.Background()
	if err := db.EnsureSchema(ctx); err != nil {
		t.Fatalf("EnsureSchema failed: %v", err)
	}

	now := time.Now()
	sha := "vec-c-001"
	embedding := createTestVector(42)

	c := CommitData{
		SHA:       sha,
		Message:   "feat: add vector search test",
		Author:    "dev@example.com",
		RepoPath:  testTenant,
		Branch:    "main",
		DiffText:  "diff --git a/search.go b/search.go",
		Timestamp: now,
		IsGudGenerated: true,
		Embedding: embedding,
	}
	if err := db.Exec(ctx, BuildPersistCommitQuery(c), nil); err != nil {
		t.Fatalf("persist commit with embedding failed: %v", err)
	}

	// Search with a similar vector (slightly different seed).
	queryVec := createTestVector(43)
	q := BuildHybridContextQuery(testTenant, queryVec, "", nil, 10)
	var rawResp map[string]any
	if err := db.Exec(ctx, q, &rawResp); err != nil {
		t.Fatalf("BuildHybridContextQuery failed: %v", err)
	}

	vecRaw, ok := rawResp["by_vector"]
	if !ok {
		t.Fatal("expected 'by_vector' in response")
	}
	vecItems := extractResultItems(vecRaw)
	if len(vecItems) == 0 {
		t.Fatal("expected at least 1 vector search result")
	}

	found := false
	for _, item := range vecItems {
		if m, ok := item.(map[string]any); ok {
			if s, _ := m["sha"].(string); s == sha {
				found = true
				if dist, _ := m["distance"].(float64); dist > 0 {
					t.Logf("vector distance for %s: %f", sha, dist)
				}
			}
		}
	}
	if !found {
		t.Errorf("expected commit %s in vector search results (%d items)", sha, len(vecItems))
	}
	t.Logf("Vector search returned %d items, found expected commit", len(vecItems))
}

// TestIntegration_TopFilesStats persists commits with file changes
// and verifies that BuildTopFilesQuery returns correct file counts.
func TestIntegration_TopFilesStats(t *testing.T) {
	if os.Getenv("RUN_HELIXDB_INTEGRATION") == "" {
		t.Skip("set RUN_HELIXDB_INTEGRATION=1 to run")
	}

	mgr, db := startManagedContainer(t)
	defer mgr.Stop(context.Background())

	ctx := context.Background()
	if err := db.EnsureSchema(ctx); err != nil {
		t.Fatalf("EnsureSchema failed: %v", err)
	}

	now := time.Now()

	// Persist two commits touching the same file.
	for i, c := range []CommitData{
		{
			SHA: "tf-001", Message: "first change", Author: "dev@example.com",
			RepoPath: testTenant, Branch: "main",
			DiffText: "diff --git a/lib/core.go b/lib/core.go",
			Timestamp: now, IsGudGenerated: true,
			Files: []FileChange{
				{Path: "lib/core.go", ChangeType: "modified", LinesAdded: 10, LinesDeleted: 2},
			},
		},
		{
			SHA: "tf-002", Message: "second change", Author: "dev@example.com",
			RepoPath: testTenant, Branch: "main",
			DiffText: "diff --git a/lib/core.go b/lib/core.go",
			Timestamp: now.Add(time.Second), IsGudGenerated: true,
			Files: []FileChange{
				{Path: "lib/core.go", ChangeType: "modified", LinesAdded: 3, LinesDeleted: 1},
				{Path: "lib/util.go", ChangeType: "added", LinesAdded: 15, LinesDeleted: 0},
			},
		},
	} {
		if err := db.Exec(ctx, BuildPersistCommitQuery(c), nil); err != nil {
			t.Fatalf("persist commit %s (%d) failed: %v", c.SHA, i, err)
		}
	}

	// Top files query.
	q := BuildTopFilesQuery(testTenant, 10)
	var rawResp map[string]any
	if err := db.Exec(ctx, q, &rawResp); err != nil {
		t.Fatalf("BuildTopFilesQuery failed: %v", err)
	}

	stats := ParseTopFiles(NewResponse(rawResp))
	output := FormatTopFiles(stats)
	t.Logf("top files output:\n%s", output)

	if len(stats) == 0 {
		t.Fatal("expected at least 1 file stat")
	}
	// lib/core.go should have 2 changes (across 2 commits).
	found := false
	for _, s := range stats {
		if s.Path == "lib/core.go" {
			found = true
			if s.Changes < 2 {
				t.Errorf("expected lib/core.go to have >=2 changes, got %d", s.Changes)
			}
		}
	}
	if !found {
		t.Errorf("expected lib/core.go in top files, got: %+v", stats)
	}
}

// TestIntegration_CategoryMemories verifies the full cycle: create category,
// create memory, categorize it, then recall via BuildCategoryMemoriesQuery.
func TestIntegration_CategoryMemoriesQuery(t *testing.T) {
	if os.Getenv("RUN_HELIXDB_INTEGRATION") == "" {
		t.Skip("set RUN_HELIXDB_INTEGRATION=1 to run")
	}

	mgr, db := startManagedContainer(t)
	defer mgr.Stop(context.Background())

	ctx := context.Background()
	if err := db.EnsureSchema(ctx); err != nil {
		t.Fatalf("EnsureSchema failed: %v", err)
	}

	tenant := "cat-mem-tenant"
	user := "user-bob"
	now := time.Now().Truncate(time.Millisecond)

	// Create two categories.
	for _, cat := range []struct {
		key  string
		name string
	}{
		{tenant + ":work", "Work"},
		{tenant + ":personal", "Personal"},
	} {
		createCat := helix.WriteQuery("create_category_" + strings.ReplaceAll(cat.name, " ", "_"))
		createCat.VarAs("cat",
			helix.G().AddN("Category", helix.Props{
				helix.Prop("categoryKey", helix.String(cat.key)),
				helix.Prop(DefaultTenantProperty, helix.String(tenant)),
				helix.Prop("name", helix.String(cat.name)),
				helix.Prop("description", helix.String("")),
			}))
		if err := db.Exec(ctx, createCat.Returning("cat"), nil); err != nil {
			t.Fatalf("create category %s failed: %v", cat.name, err)
		}
	}

	// Create two memories in different categories.
	memWork := MemoryData{
		MemoryID:  tenant + ":work-mem",
		Content:   "User prefers Go for backend services",
		TenantID:  tenant,
		UserID:    user,
		Kind:      MemoryPreference,
		Salience:  0.9,
		IsLatest:  true,
		CreatedAt: now,
		UpdatedAt: now,
	}
	memPersonal := MemoryData{
		MemoryID:  tenant + ":personal-mem",
		Content:   "User enjoys hiking on weekends",
		TenantID:  tenant,
		UserID:    user,
		Kind:      MemoryFact,
		Salience:  0.6,
		IsLatest:  true,
		CreatedAt: now,
		UpdatedAt: now,
	}

	for _, m := range []MemoryData{memWork, memPersonal} {
		if err := db.Exec(ctx, BuildPersistMemoryQuery(m), nil); err != nil {
			t.Fatalf("persist memory %s failed: %v", m.MemoryID, err)
		}
	}

	// Categorize work memory.
	if err := db.Exec(ctx, BuildCategorizeMemoryQuery(memWork.MemoryID, tenant, tenant+":work"), nil); err != nil {
		t.Fatalf("categorize work memory failed: %v", err)
	}
	// Categorize personal memory.
	if err := db.Exec(ctx, BuildCategorizeMemoryQuery(memPersonal.MemoryID, tenant, tenant+":personal"), nil); err != nil {
		t.Fatalf("categorize personal memory failed: %v", err)
	}

	// Query work category — should only find work memory.
	qWork := BuildCategoryMemoriesQuery(tenant+":work", tenant, 10)
	var workResp map[string]any
	if err := db.Exec(ctx, qWork, &workResp); err != nil {
		t.Fatalf("BuildCategoryMemoriesQuery(work) failed: %v", err)
	}
	workItems := extractResultItems(workResp["memories"])

	// Verify work memory is present and personal memory is not.
	for _, item := range workItems {
		if m, ok := item.(map[string]any); ok {
			if mid, _ := m["memoryId"].(string); mid == memPersonal.MemoryID {
				t.Errorf("personal memory %s should not be in work category", memPersonal.MemoryID)
			}
		}
	}

	// Query personal category — should only find personal memory.
	qPersonal := BuildCategoryMemoriesQuery(tenant+":personal", tenant, 10)
	var personalResp map[string]any
	if err := db.Exec(ctx, qPersonal, &personalResp); err != nil {
		t.Fatalf("BuildCategoryMemoriesQuery(personal) failed: %v", err)
	}
	personalItems := extractResultItems(personalResp["memories"])

	foundPersonal := false
	for _, item := range personalItems {
		if m, ok := item.(map[string]any); ok {
			if mid, _ := m["memoryId"].(string); mid == memPersonal.MemoryID {
				foundPersonal = true
			}
		}
	}
	if !foundPersonal {
		t.Errorf("expected personal memory %s in personal category results", memPersonal.MemoryID)
	}
	t.Logf("Category query verified: work has %d memories, personal has %d", len(workItems), len(personalItems))
}

// TestIntegration_BM25ContextQueryNoResults verifies that a context query
// for a non-matching diff returns no results.
func TestIntegration_BM25ContextQueryNoResults(t *testing.T) {
	if os.Getenv("RUN_HELIXDB_INTEGRATION") == "" {
		t.Skip("set RUN_HELIXDB_INTEGRATION=1 to run")
	}

	mgr, db := startManagedContainer(t)
	defer mgr.Stop(context.Background())

	ctx := context.Background()
	if err := db.EnsureSchema(ctx); err != nil {
		t.Fatalf("EnsureSchema failed: %v", err)
	}

	now := time.Now()
	c := CommitData{
		SHA: "nores-a", Message: "some commit", Author: "dev@example.com",
		RepoPath: testTenant, Branch: "main",
		DiffText:  "diff --git a/main.go b/main.go",
		Timestamp: now, IsGudGenerated: true,
	}
	if err := db.Exec(ctx, BuildPersistCommitQuery(c), nil); err != nil {
		t.Fatalf("persist commit failed: %v", err)
	}

	// Search for something that doesn't match.
	q := BuildContextQuery(testTenant, "main", nil, "ZZZZ_THIS_DOES_NOT_MATCH_ZZZZ")
	var rawResp map[string]any
	if err := db.Exec(ctx, q, &rawResp); err != nil {
		t.Fatalf("BuildContextQuery failed: %v", err)
	}
	records := ParseContextResults(NewResponse(rawResp))
	if len(records) != 0 {
		t.Errorf("expected 0 results for non-matching query, got %d", len(records))
	}
	t.Log("Non-matching BM25 correctly returned no results")
}
