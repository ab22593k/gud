package mem

import (
	"encoding/json"
	"strings"
	"testing"

	helix "github.com/helixdb/helix-db/sdks/go"
)

// assertHasLimit fails when the marshaled query has no limit step.
func assertHasLimit(t *testing.T, q helix.Request, want int) {
	t.Helper()

	raw, err := helix.MarshalRequest(q)
	if err != nil {
		t.Fatalf("MarshalRequest()=%v, want nil", err)
	}

	if !strings.Contains(string(raw), `"limit"`) {
		t.Fatalf("query %s has no limit step, want capped at %d", string(raw), want)
	}

	var decoded struct {
		Query map[string]struct {
			Entries []struct {
				Query struct {
					Name string         `json:"name"`
					Root map[string]any `json:"root"`
				} `json:"query"`
			} `json:"entries"`
		} `json:"query"`
	}

	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal()=%v, want nil", err)
	}

	found := false

	for _, entries := range decoded.Query {
		for _, e := range entries.Entries {
			if _, ok := e.Query.Root["limit"]; ok {
				found = true
			}
		}
	}

	if !found {
		t.Errorf("no entry with limit root, want cap %d", want)
	}
}

func TestStatsQueries_AreBounded(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		query helix.Request
	}{
		{name: "repo summary by_author", query: BuildRepoSummaryQuery("/repo")},
		{name: "author stats", query: BuildAuthorStatsQuery("/repo")},
		{name: "trends", query: BuildTrendsQuery("/repo")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if err := tt.query.Validate(); err != nil {
				t.Fatalf("Validate()=%v, want nil", err)
			}

			assertHasLimit(t, tt.query, maxStatsCommits)
		})
	}
}
