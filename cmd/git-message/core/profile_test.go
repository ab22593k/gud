package core

import (
	"bytes"
	"strings"
	"testing"

	"gud/internal/profile"
)

const (
	testBiology = "biology"
	testPhysics = "physics"
)

func TestCategorizeByWorkMode(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name  string
		input []profile.CatalogEntry
		want  []category
	}

	tests := []testCase{
		{
			name:  "nil input returns empty slice",
			input: nil,
			want:  []category{},
		},
		{
			name:  "empty input returns empty slice",
			input: []profile.CatalogEntry{},
			want:  []category{},
		},
		{
			name: "single entry",
			input: []profile.CatalogEntry{
				{WorkMode: testPhysics, Profession: testAstrophysicist},
			},
			want: []category{
				{name: testPhysics, count: 1},
			},
		},
		{
			name: "multiple entries same work mode",
			input: []profile.CatalogEntry{
				{WorkMode: testPhysics, Profession: testAstrophysicist},
				{WorkMode: testPhysics, Profession: "cosmologist"},
				{WorkMode: testPhysics, Profession: "quantum physicist"},
			},
			want: []category{
				{name: testPhysics, count: 3},
			},
		},
		{
			name: "entries grouped by work mode and sorted by name",
			input: []profile.CatalogEntry{
				{WorkMode: testBiology, Profession: "molecular biologist"},
				{WorkMode: testPhysics, Profession: testAstrophysicist},
				{WorkMode: "chemistry", Profession: "organic chemist"},
				{WorkMode: testBiology, Profession: "geneticist"},
			},
			want: []category{
				{name: testBiology, count: 2},
				{name: "chemistry", count: 1},
				{name: testPhysics, count: 1},
			},
		},
		{
			name: "single element in each work mode",
			input: []profile.CatalogEntry{
				{WorkMode: "z", Profession: "last"},
				{WorkMode: "a", Profession: "first"},
				{WorkMode: "m", Profession: "middle"},
			},
			want: []category{
				{name: "a", count: 1},
				{name: "m", count: 1},
				{name: "z", count: 1},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := categorizeByWorkMode(tt.input)

			if len(got) != len(tt.want) {
				t.Fatalf("categorizeByWorkMode() returned %d categories, want %d\ngot:  %+v\nwant: %+v",
					len(got), len(tt.want), got, tt.want)
			}

			for i := range got {
				if got[i].name != tt.want[i].name || got[i].count != tt.want[i].count {
					t.Errorf("categorizeByWorkMode()[%d] = {name:%q, count:%d}, want {name:%q, count:%d}",
						i, got[i].name, got[i].count, tt.want[i].name, tt.want[i].count)
				}
			}
		})
	}
}

// profileSummaryHint is the trailing instruction line printed by
// printProfileSummary after the category summary.
const profileSummaryHint = "Use 'git message --profile <slug>' or 'git message profile save <slug>' " +
	"with one of the slugs below.\n"

// TestPrintProfileSummary verifies the category summary header, per-category
// count lines, and instruction line written to the output writer.
func TestPrintProfileSummary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		total int
		cats  []category
		want  string
	}{
		{
			name:  "no profiles and no categories",
			total: 0,
			cats:  nil,
			want: "\nFound 0 profiles in 0 categories:\n" +
				"\n" +
				"\n" +
				profileSummaryHint +
				"\n",
		},
		{
			name:  "single category",
			total: 3,
			cats:  []category{{name: testPhysics, count: 3}},
			want: "\nFound 3 profiles in 1 categories:\n" +
				"\n" +
				"  physics (3 profiles)\n" +
				"\n" +
				profileSummaryHint +
				"\n",
		},
		{
			name:  "multiple categories with total independent of category count",
			total: 4,
			cats: []category{
				{name: testBiology, count: 2},
				{name: "chemistry", count: 1},
				{name: testPhysics, count: 1},
			},
			want: "\nFound 4 profiles in 3 categories:\n" +
				"\n" +
				"  biology (2 profiles)\n" +
				"  chemistry (1 profiles)\n" +
				"  physics (1 profiles)\n" +
				"\n" +
				profileSummaryHint +
				"\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			printProfileSummary(&buf, tt.total, tt.cats)

			if got := buf.String(); got != tt.want {
				t.Errorf("printProfileSummary(total=%d, cats=%d):\n  got:  %q\n  want: %q",
					tt.total, len(tt.cats), got, tt.want)
			}
		})
	}
}

// TestFindProfession verifies slug lookup over the remote catalog: exact
// match, miss, empty catalog, and first-match-wins on duplicate slugs
// (documenting the linear-scan contract).
func TestFindProfession(t *testing.T) {
	t.Parallel()

	entries := []profile.CatalogEntry{
		{Slug: "astro", Profession: "Astrophysicist"},
		{Slug: "bio", Profession: "Molecular Biologist"},
		{Slug: "astro", Profession: "First Astro Wins"},
	}

	tests := []struct {
		slug    string
		entries []profile.CatalogEntry
		want    string
	}{
		{slug: "astro", entries: entries, want: "Astrophysicist"},
		{slug: "bio", entries: entries, want: "Molecular Biologist"},
		{slug: "missing", entries: entries, want: ""},
		{slug: "astro", entries: nil, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.slug, func(t *testing.T) {
			t.Parallel()

			got := findProfession(tt.slug, tt.entries)
			if got != tt.want {
				t.Errorf("findProfession(%q) = %q, want %q", tt.slug, got, tt.want)
			}
		})
	}

	if got := findProfession("astro", entries); got != "Astrophysicist" {
		t.Errorf("duplicate slugs: first entry must win, got %q", got)
	}
}

// TestWriteCatalogEntry verifies the single-entry rendering contract:
// a four-space indent, the slug left-aligned in a 50-column field, and the
// summary truncated to 70 characters via truncate.
func TestWriteCatalogEntry(t *testing.T) {
	t.Parallel()

	const slug = "astro"

	// summaryAt builds a summary of exactly n 'x' characters plus an
	// optional suffix, so boundary cases derive from the contract constant
	// instead of hand-counted literals.
	summaryAt := func(n int, suffix string) string {
		return strings.Repeat("x", n) + suffix
	}

	at70 := summaryAt(70, "")
	at69 := summaryAt(69, "")

	tests := []struct {
		name        string
		slug        string
		summary     string
		wantSummary string
	}{
		{name: "short summary unchanged", slug: slug, summary: "stars", wantSummary: "stars"},
		{name: "summary at 70 boundary unchanged", slug: slug, summary: at70, wantSummary: at70},
		{name: "summary over 70 truncated with ellipsis", slug: slug, summary: at70 + "y", wantSummary: at70 + "..."},
		{name: "70 chars returned as-is incl. space", slug: slug, summary: at69 + " ", wantSummary: at69 + " "},
		{name: "space at cut point trimmed before ellipsis", slug: slug, summary: at69 + " y", wantSummary: at69 + "..."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			writeCatalogEntry(&buf, profile.CatalogEntry{Slug: tt.slug, Summary: tt.summary})

			want := "    " + tt.slug + strings.Repeat(" ", 50-len(tt.slug)) + " " + tt.wantSummary + "\n"
			if got := buf.String(); got != want {
				t.Errorf("writeCatalogEntry():\n  got:  %q\n  want: %q", got, want)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	t.Parallel()

	const (
		hello      = "hello"
		helloWorld = "hello world"
	)

	tests := []struct {
		name   string
		s      string
		maxLen int
		want   string
	}{
		{name: "shorter than max returns as-is", s: hello, maxLen: 10, want: hello},
		{name: "equal to max returns as-is", s: hello, maxLen: 5, want: hello},
		{name: "longer than max appends ellipsis", s: helloWorld, maxLen: 5, want: "hello..."},
		{name: "empty string returns empty", s: "", maxLen: 10, want: ""},
		{name: "maxLen of zero with content", s: hello, maxLen: 0, want: "..."},
		{name: "trailing space at boundary trimmed", s: helloWorld, maxLen: 6, want: "hello..."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := truncate(tt.s, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.s, tt.maxLen, got, tt.want)
			}
		})
	}
}
