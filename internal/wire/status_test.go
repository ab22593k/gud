package wire

import (
	"fmt"
	"strings"
	"testing"
)

func TestDeriveState(t *testing.T) {
	t.Parallel()

	commit := strings.Repeat("a", 40)
	exportHash := strings.Repeat("b", 64)

	entry := RegistryEntry{ResolvedCommit: commit, ExportHash: exportHash}

	cases := []struct {
		name       string
		localHash  string
		remoteSHA  string
		resolveErr error
		want       SyncState
	}{
		{name: "current", localHash: exportHash, remoteSHA: commit, want: StateCurrent},
		{name: "behind", localHash: exportHash, remoteSHA: strings.Repeat("c", 40), want: StateBehind},
		{
			name: "diverged dominates behind", localHash: strings.Repeat("d", 64),
			remoteSHA: strings.Repeat("c", 40), want: StateDiverged,
		},
		{name: "diverged when current upstream", localHash: strings.Repeat("d", 64), remoteSHA: commit, want: StateDiverged},
		{
			name: "unreachable on error", localHash: exportHash, remoteSHA: "",
			resolveErr: fmt.Errorf("dial: %w", ErrUpstream), want: StateUnreachable,
		},
		{name: "unreachable on empty sha", localHash: exportHash, want: StateUnreachable},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := deriveState(entry, tc.localHash, tc.remoteSHA, tc.resolveErr); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestDescribePinned proves data-model §4/D15: pinned entries (tag, commit)
// derive locally — current when the checkout matches the export, diverged
// on local edits — and can never be behind or unreachable.
func TestDescribePinned(t *testing.T) {
	t.Parallel()

	commit := strings.Repeat("a", 40)
	exportHash := strings.Repeat("b", 64)

	cases := []struct {
		name  string
		kind  RefKind
		entry RegistryEntry
		live  string
		want  SyncState
	}{
		{
			name:  "commit pin current",
			kind:  RefKindCommit,
			entry: RegistryEntry{Ref: commit, RefKind: RefKindCommit, ResolvedCommit: commit, ExportHash: exportHash},
			live:  exportHash,
			want:  StateCurrent,
		},
		{
			name:  "tag pin current",
			kind:  RefKindTag,
			entry: RegistryEntry{Ref: "v19.0", RefKind: RefKindTag, ResolvedCommit: commit, ExportHash: exportHash},
			live:  exportHash,
			want:  StateCurrent,
		},
		{
			name:  "commit pin diverged",
			kind:  RefKindCommit,
			entry: RegistryEntry{Ref: commit, RefKind: RefKindCommit, ResolvedCommit: commit, ExportHash: exportHash},
			live:  strings.Repeat("d", 64),
			want:  StateDiverged,
		},
		{
			name:  "tag pin diverged",
			kind:  RefKindTag,
			entry: RegistryEntry{Ref: "v19.0", RefKind: RefKindTag, ResolvedCommit: commit, ExportHash: exportHash},
			live:  strings.Repeat("d", 64),
			want:  StateDiverged,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// A resolving fake whose only commit differs from the entry's:
			// if describe touches the fetcher, a current checkout misreads
			// as behind — the pin must never resolve at all.
			f := &fakeFetcher{
				commits: map[string]string{"19.0": strings.Repeat("c", 40)},
				files:   map[string]map[string]string{strings.Repeat("c", 40): {}},
			}

			dir := t.TempDir()

			got := describePinned(dir, tc.entry, tc.live, f)

			if got != tc.want {
				t.Fatalf("state = %q, want %q", got, tc.want)
			}

			if f.resolves != 0 {
				t.Fatalf("Resolve called %d times for a %s pin", f.resolves, tc.kind)
			}
		})
	}
}
