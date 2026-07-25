package detect

import (
	"strings"
	"testing"

	"gud/internal/profile"
)

func TestSuggestProfile_NilStats(t *testing.T) {
	t.Parallel()

	got := SuggestProfile(nil, nil)
	if got != nil {
		t.Errorf("SuggestProfile(nil, nil) = %v, want nil", got)
	}
}

func TestSuggestProfile_EmptyRepo(t *testing.T) {
	t.Parallel()

	stats := &RepoStats{
		FilesByExtension: map[string]int{},
		TotalFiles:       0,
	}
	catalog := []profile.CatalogEntry{
		{Slug: "go-dev", Profession: "Go Developer", Summary: "Go development"},
	}

	got := SuggestProfile(stats, catalog)
	if got != nil {
		t.Errorf("SuggestProfile(empty) = %v, want nil", got)
	}
}

func TestSuggestProfile_EmptyCatalog(t *testing.T) {
	t.Parallel()

	stats := &RepoStats{
		FilesByExtension: map[string]int{".go": 5},
		TotalFiles:       5,
	}

	got := SuggestProfile(stats, nil)
	if got != nil {
		t.Errorf("SuggestProfile(nil catalog) = %v, want nil", got)
	}
}

func TestSuggestProfile_MatchByExtension(t *testing.T) {
	t.Parallel()

	stats := &RepoStats{
		FilesByExtension: map[string]int{".go": 10, ".proto": 3},
		TotalFiles:       13,
	}
	catalog := []profile.CatalogEntry{
		{Slug: "go-developer", Profession: "Go Developer", Summary: "Build Go applications"},
		{Slug: "rust-dev", Profession: "Rust Developer", Summary: "Systems programming in Rust"},
		{Slug: "python-dev", Profession: "Python Developer", Summary: "Python development"},
	}

	got := SuggestProfile(stats, catalog)

	if len(got) == 0 {
		t.Fatal("SuggestProfile = empty, want at least 1 suggestion")
	}

	// go-developer should match because ".go" -> "go" appears in profession and summary
	if got[0].Slug != "go-developer" {
		t.Errorf("top suggestion = %q, want %q", got[0].Slug, "go-developer")
	}
}

func TestSuggestProfile_Top3ByScore(t *testing.T) {
	t.Parallel()

	stats := &RepoStats{
		FilesByExtension: map[string]int{".go": 10, ".ts": 5},
		TotalFiles:       15,
	}
	catalog := []profile.CatalogEntry{
		{Slug: "go-dev", Profession: "Go Developer", Summary: "Go applications"},
		{Slug: "fullstack", Profession: "Fullstack Engineer", Summary: "TypeScript and Go"},
		{Slug: "ts-dev", Profession: "TypeScript Engineer", Summary: "TypeScript apps"},
		{Slug: "rust-dev", Profession: "Rust Developer", Summary: "Rust systems programming"},
	}

	got := SuggestProfile(stats, catalog)

	// Keywords: "go" and "ts"
	// Scoring:
	// - fullstack: contains "go" AND "ts" (in "typescript"? No — the letters t-y-p-e-s-c-r-i-p-t
	//   do NOT contain consecutive "ts"). Contains "go" = score 1.
	//   Actually, "typescript" does NOT contain "ts". Let me be precise:
	//   "typescript": t,y,p,e,s,c,r,i,p,t — no "ts"
	// - go-dev:    contains "go" = score 1
	// - ts-dev:    "typescript" (no "ts") = score 0
	// - rust-dev:  no match = score 0
	// Result: fullstack (score 1), go-dev (score 1) — both score 1, alphabetically by slug
	if len(got) != 2 {
		t.Errorf("SuggestProfile = %d results, want 2", len(got))
	}
	if len(got) >= 1 && got[0].Slug != "fullstack" {
		t.Errorf("suggestion[0] = %q, want %q", got[0].Slug, "fullstack")
	}
	if len(got) >= 2 && got[1].Slug != "go-dev" {
		t.Errorf("suggestion[1] = %q, want %q", got[1].Slug, "go-dev")
	}
}

func TestFormatSuggestionMessage_Empty(t *testing.T) {
	t.Parallel()

	if got := FormatSuggestionMessage(nil); got != "" {
		t.Errorf("FormatSuggestionMessage(nil) = %q, want ''", got)
	}
	if got := FormatSuggestionMessage([]profile.CatalogEntry{}); got != "" {
		t.Errorf("FormatSuggestionMessage(empty) = %q, want ''", got)
	}
}

func TestFormatSuggestionMessage_HappyPath(t *testing.T) {
	t.Parallel()

	suggestions := []profile.CatalogEntry{
		{Slug: "go-developer", Summary: "Build Go applications with best practices"},
		{Slug: "ts-developer", Summary: "TypeScript development patterns"},
	}

	got := FormatSuggestionMessage(suggestions)

	if got == "" {
		t.Fatal("FormatSuggestionMessage = empty, want non-empty")
	}

	if !strings.Contains(got, "go-developer") {
		t.Errorf("FormatSuggestionMessage missing 'go-developer', got:\n%s", got)
	}
	if !strings.Contains(got, "ts-developer") {
		t.Errorf("FormatSuggestionMessage missing 'ts-developer', got:\n%s", got)
	}
	if !strings.Contains(got, "Select a profile") {
		t.Errorf("FormatSuggestionMessage missing prompt, got:\n%s", got)
	}
}
