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
