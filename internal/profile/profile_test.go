package profile

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

const slugTestAgent = "test-agent"
const slugExists = "exists"
const slugTest = "test"
const slugAstro = "astrophysicist"

func TestNewManager_CreatesCacheDir(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	m, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	expected := filepath.Join(tmpDir, ".config", "gud", "profiles")
	if m.cacheDir != expected {
		t.Errorf("cacheDir = %q, want %q", m.cacheDir, expected)
	}
	if _, err := os.Stat(expected); os.IsNotExist(err) {
		t.Errorf("cache dir should exist: %v", err)
	}
}

func TestSaveAndList(t *testing.T) {
	tmpDir := t.TempDir()
	m := &Manager{cacheDir: tmpDir}

	p := Profile{Slug: slugTestAgent, Profession: slugTestAgent, Content: "You are a test agent."}
	if err := m.Save(slugTestAgent, p); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	profiles, err := m.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("List() returned %d profiles, want 1", len(profiles))
	}
	if profiles[0].Slug != slugTestAgent {
		t.Errorf("profile slug = %q, want %q", profiles[0].Slug, slugTestAgent)
	}
}

func TestList_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	m := &Manager{cacheDir: tmpDir}

	profiles, err := m.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(profiles) != 0 {
		t.Errorf("List() returned %d profiles, want 0", len(profiles))
	}
}

func TestIsCached(t *testing.T) {
	tmpDir := t.TempDir()
	m := &Manager{cacheDir: tmpDir}

	if m.IsCached("missing") {
		t.Error("IsCached() = true for missing profile")
	}

	p := Profile{Slug: slugExists, Profession: slugExists, Content: "something"}
	if err := m.Save(slugExists, p); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if !m.IsCached(slugExists) {
		t.Error("IsCached() = false for cached profile")
	}
}

func TestGet(t *testing.T) {
	tmpDir := t.TempDir()
	m := &Manager{cacheDir: tmpDir}

	_, err := m.Get("missing")
	if err == nil {
		t.Fatal("Get() expected error for missing profile")
	}

	p := Profile{Slug: slugTest, Profession: slugTest, Content: "You are a test."}
	if err := m.Save(slugTest, p); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := m.Get(slugTest)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Content != "You are a test." {
		t.Errorf("content = %q, want %q", got.Content, "You are a test.")
	}
}

func TestRemove(t *testing.T) {
	tmpDir := t.TempDir()
	m := &Manager{cacheDir: tmpDir}

	if err := m.Remove("missing"); err == nil {
		t.Fatal("Remove() expected error for missing profile")
	}

	p := Profile{Slug: slugTest, Profession: slugTest, Content: "content"}
	if err := m.Save(slugTest, p); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if err := m.Remove(slugTest); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if m.IsCached(slugTest) {
		t.Error("profile still cached after Remove()")
	}
}

func TestFetchCatalog(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := RemoteCatalog{Agents: []RemoteAgent{
			{Profession: slugAstro, Summary: "Studies stars", WorkMode: "scientific"},
		}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	origURL := catalogURL
	catalogURL = server.URL
	t.Cleanup(func() { catalogURL = origURL })

	m := &Manager{}
	entries, err := m.FetchCatalog(context.Background())
	if err != nil {
		t.Fatalf("FetchCatalog() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	if entries[0].Slug != slugAstro {
		t.Errorf("slug = %q, want %q", entries[0].Slug, slugAstro)
	}
}

func TestFetchProfile_ValidatesSlug(t *testing.T) {
	m := &Manager{}
	_, err := m.FetchProfile(context.Background(), "../../etc/passwd")
	if err == nil {
		t.Fatal("FetchProfile() expected error for malicious slug")
	}
}

func TestFetchProfile_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("You are an astrophysicist."))
	}))
	defer server.Close()

	origBase := profileBaseURL
	profileBaseURL = server.URL + "/%s/AGENTS.md"
	t.Cleanup(func() { profileBaseURL = origBase })

	m := &Manager{}
	content, err := m.FetchProfile(context.Background(), slugAstro)
	if err != nil {
		t.Fatalf("FetchProfile() error = %v", err)
	}
	if content != "You are an astrophysicist." {
		t.Errorf("content = %q, want %q", content, "You are an astrophysicist.")
	}
}

func TestSlugify(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Astrophysicist", "astrophysicist"},
		{"Computer Scientist", "computer-scientist"},
		{"AI/ML Engineer", "ai-ml-engineer"},
		{"R&D Specialist", "randd-specialist"},
		{"DevOps & SRE", "devops-and-sre"},
		{"O'Brien", "obrien"},
	}
	for _, tt := range tests {
		got := slugify(tt.input)
		if got != tt.want {
			t.Errorf("slugify(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
