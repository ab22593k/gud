package mem

import (
	"slices"
	"strings"
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

func TestBuildTrendsQuery(t *testing.T) {
	q := BuildTrendsQuery("/home/user/repo")
	if q == nil {
		t.Fatal("expected non-nil query")
	}
	if err := q.Validate(); err != nil {
		t.Errorf("query validation failed: %v", err)
	}
}

func TestParseAuthorStats(t *testing.T) {
	tests := []struct {
		name string
		raw  map[string]any
		want []AuthorStat
	}{
		{
			name: "counts commits per author",
			raw: map[string]any{
				"by_author": []any{
					map[string]any{"author": "alice@example.com"},
					map[string]any{"author": "alice@example.com"},
					map[string]any{"author": "bob@example.com"},
				},
			},
			want: []AuthorStat{{"alice@example.com", 2}, {"bob@example.com", 1}},
		},
		{
			name: "falls back to authors_by_array key",
			raw: map[string]any{
				"authors_by_array": []any{
					map[string]any{"author": "carol@example.com"},
				},
			},
			want: []AuthorStat{{"carol@example.com", 1}},
		},
		{
			name: "skips entries with empty author",
			raw: map[string]any{
				"by_author": []any{
					map[string]any{"author": ""},
					map[string]any{"author": "dave@example.com"},
				},
			},
			want: []AuthorStat{{"dave@example.com", 1}},
		},
		{
			name: "no data returns nil",
			raw:  map[string]any{},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseAuthorStats(NewResponse(tt.raw))
			if len(got) != len(tt.want) {
				t.Fatalf("ParseAuthorStats() = %v, want %v", got, tt.want)
			}
			for _, g := range got {
				if !slices.Contains(tt.want, g) {
					t.Errorf("unexpected AuthorStat %v", g)
				}
			}
		})
	}
}

func TestParseTopFiles(t *testing.T) {
	resp := NewResponse(map[string]any{
		"files": []any{
			map[string]any{"path": "main.go"},
			map[string]any{"path": "main.go"},
			map[string]any{"path": "util.go"},
			map[string]any{"path": ""}, // skipped
		},
	})
	got := ParseTopFiles(resp)
	want := map[string]int{"main.go": 2, "util.go": 1}
	if len(got) != len(want) {
		t.Fatalf("ParseTopFiles() = %v, want counts %v", got, want)
	}
	for _, f := range got {
		if want[f.Path] != f.Changes {
			t.Errorf("%s: changes = %d, want %d", f.Path, f.Changes, want[f.Path])
		}
	}

	if got := ParseTopFiles(NewResponse(map[string]any{})); got != nil {
		t.Errorf("empty response: ParseTopFiles() = %v, want nil", got)
	}
}

func TestParseTrends(t *testing.T) {
	// 2026-06-29 12:00 UTC = 1782734400000 ms since epoch.
	day := float64(1782734400000)
	tests := []struct {
		name   string
		key    string
		stamps []float64
	}{
		{name: "commits_by_day primary key", key: "commits_by_day", stamps: []float64{day, day, day + 86400000}},
		{name: "commits fallback key", key: "commits", stamps: []float64{day}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodes := make([]any, 0, len(tt.stamps))
			for _, s := range tt.stamps {
				nodes = append(nodes, map[string]any{"timestamp": s})
			}
			got := ParseTrends(NewResponse(map[string]any{tt.key: nodes}))
			total := 0
			for _, p := range got {
				total += p.Count
			}
			if total != len(tt.stamps) {
				t.Errorf("total trend count = %d, want %d (%v)", total, len(tt.stamps), got)
			}
		})
	}

	// Invalid timestamps are skipped.
	resp := NewResponse(map[string]any{
		"commits": []any{
			map[string]any{"timestamp": float64(0)}, // zero -> invalid
			map[string]any{"timestamp": day},
		},
	})
	if got := ParseTrends(resp); len(got) != 1 || got[0].Count != 1 {
		t.Errorf("ParseTrends() with invalid stamp = %v, want single point", got)
	}

	if got := ParseTrends(NewResponse(map[string]any{})); got != nil {
		t.Errorf("empty response: ParseTrends() = %v, want nil", got)
	}
}

func TestParseTimestamp(t *testing.T) {
	if _, ok := parseTimestamp(0); ok {
		t.Error("parseTimestamp(0) ok = true, want false")
	}
	ts, ok := parseTimestamp(1782734400000)
	if !ok {
		t.Fatal("parseTimestamp(valid) ok = false, want true")
	}
	if got := ts.UTC().Format("2006-01-02"); got != "2026-06-29" {
		t.Errorf("parsed date = %s, want 2026-06-29", got)
	}
}

func TestFormatAuthorStats(t *testing.T) {
	if got := FormatAuthorStats(nil); got != "No author data found.\n" {
		t.Errorf("FormatAuthorStats(nil) = %q", got)
	}

	got := FormatAuthorStats([]AuthorStat{{Email: "a@x.com", Count: 3}, {Email: "b@x.com", Count: 1}})
	for _, want := range []string{"Commits by author:", "a@x.com  3 commits  (75%)", "b@x.com  1 commits  (25%)"} {
		if !strings.Contains(got, want) {
			t.Errorf("FormatAuthorStats() missing %q in\n%s", want, got)
		}
	}
}

func TestFormatTopFiles(t *testing.T) {
	if got := FormatTopFiles(nil); got != "No file data found.\n" {
		t.Errorf("FormatTopFiles(nil) = %q", got)
	}

	got := FormatTopFiles([]FileStat{{Path: "main.go", Changes: 7}})
	if !strings.Contains(got, "main.go  7 changes") {
		t.Errorf("FormatTopFiles() = %q, want file line", got)
	}
}

func TestFormatTrends(t *testing.T) {
	if got := FormatTrends(nil); got != "No trend data found.\n" {
		t.Errorf("FormatTrends(nil) = %q", got)
	}

	got := FormatTrends([]TrendPoint{{Date: "2026-06-29", Count: 4}})
	if !strings.Contains(got, "2026-06-29  4 commits") {
		t.Errorf("FormatTrends() = %q, want trend line", got)
	}
}

func TestParseRepoSummary(t *testing.T) {
	resp := NewResponse(map[string]any{
		"total": map[string]any{"count": float64(3)},
		"by_author": []any{
			map[string]any{"author": "a@x.com"},
			map[string]any{"author": "a@x.com"},
			map[string]any{"author": "b@x.com"},
		},
	})
	stats := ParseRepoSummary(resp)
	if stats.TotalCommits != 3 {
		t.Errorf("TotalCommits = %d, want 3", stats.TotalCommits)
	}
	if len(stats.AuthorStats) != 2 {
		t.Errorf("AuthorStats = %v, want 2 entries", stats.AuthorStats)
	}

	// Count-based total missing -> falls back to counting author nodes.
	fallback := NewResponse(map[string]any{
		"by_author": []any{
			map[string]any{"author": "a@x.com"},
			map[string]any{"author": "b@x.com"},
		},
	})
	if got := ParseRepoSummary(fallback); got.TotalCommits != 2 {
		t.Errorf("fallback TotalCommits = %d, want 2", got.TotalCommits)
	}
}
