package profile

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateSlug(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		slug    string
		wantErr bool
	}{
		{name: "simple", slug: "astrophysicist", wantErr: false},
		{name: "dashes and underscores", slug: "ai-ml_engineer-2", wantErr: false},
		{name: "parent traversal", slug: "../secret", wantErr: true},
		{name: "nested traversal", slug: "a/../../etc/passwd", wantErr: true},
		{name: "absolute path", slug: "/etc/passwd", wantErr: true},
		{name: "dot slug", slug: "..", wantErr: true},
		{name: "empty", slug: "", wantErr: true},
		{name: "leading dash", slug: "-agent", wantErr: true},
		{name: "separator only", slug: "-", wantErr: true},
		{name: "dot in name", slug: "my.agent", wantErr: true},
		{name: "space", slug: "my agent", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateSlug(tt.slug)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSlug(%q) error = %v, wantErr %v", tt.slug, err, tt.wantErr)
			}
		})
	}
}

func TestCache_RejectsTraversalSlugs(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	cacheDir := filepath.Join(base, "profiles")

	if err := os.MkdirAll(cacheDir, 0o750); err != nil {
		t.Fatal(err)
	}

	m := NewManagerWithDir(cacheDir)
	traversal := "../escape"

	if err := m.Save(traversal, Profile{Content: "x"}); err == nil {
		t.Error("Save() expected error for traversal slug")
	}

	if _, err := m.Get(traversal); err == nil {
		t.Error("Get() expected error for traversal slug")
	}

	if err := m.Remove(traversal); err == nil {
		t.Error("Remove() expected error for traversal slug")
	}

	if m.IsCached(traversal) {
		t.Error("IsCached() = true for traversal slug")
	}

	if _, err := os.Stat(filepath.Join(base, "escape.json")); !os.IsNotExist(err) {
		t.Errorf("traversal write escaped cache dir: stat err = %v", err)
	}
}

func TestCache_RoundTripValidSlug(t *testing.T) {
	t.Parallel()

	m := NewManagerWithDir(t.TempDir())

	if err := m.Save("valid-slug_1", Profile{Profession: "Tester", Content: "hello"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	if !m.IsCached("valid-slug_1") {
		t.Fatal("IsCached() = false after Save()")
	}

	got, err := m.Get("valid-slug_1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if got.Content != "hello" {
		t.Errorf("Content = %q, want %q", got.Content, "hello")
	}

	if err := m.Remove("valid-slug_1"); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
}

func TestSlugify_ConfinedOutput(t *testing.T) {
	t.Parallel()

	hostile := []string{
		"../../../../home/victim/.config/gud/hooks/prepare-commit-msg",
		"..",
		"...",
		"a/b\\c",
		"---",
	}

	for _, in := range hostile {
		got := slugify(in)
		if strings.Contains(got, "/") || strings.Contains(got, "\\") || strings.Contains(got, "..") {
			t.Errorf("slugify(%q) = %q: carries a path separator or parent step", in, got)
		}

		if got != "" && !validSlug.MatchString(got) {
			t.Errorf("slugify(%q) = %q: non-empty output must match validSlug", in, got)
		}
	}
}

func TestFetchCatalog_SkipsInvalidSlugs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := RemoteCatalog{Agents: []RemoteAgent{
			{Profession: "Astrophysicist", Summary: "Studies stars", WorkMode: "scientific"},
			{Profession: "...", Summary: "Unsanitizeable", WorkMode: "scientific"},
			{Profession: "---", Summary: "Dashes only", WorkMode: "scientific"},
		}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	origURL := catalogURL
	catalogURL = server.URL

	t.Cleanup(func() { catalogURL = origURL })

	entries, err := (&Manager{}).FetchCatalog(context.Background())
	if err != nil {
		t.Fatalf("FetchCatalog() error = %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("FetchCatalog() returned %d entries, want 1 (invalid slugs skipped)", len(entries))
	}

	if entries[0].Slug != "astrophysicist" {
		t.Errorf("Slug = %q, want %q", entries[0].Slug, "astrophysicist")
	}
}
