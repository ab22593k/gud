package wire

// Sync-state derivation for checkouts.

// SyncState is the derived freshness of a checkout against its source.
// States are derived on demand, never persisted.
type SyncState string

// Checkout sync states in derivation order.
const (
	// StateDiverged means local files differ from the last export.
	StateDiverged SyncState = "diverged"
	// StateUnreachable means the source cannot be resolved right now.
	StateUnreachable SyncState = "unreachable"
	// StateBehind means upstream moved past the recorded commit.
	StateBehind SyncState = "behind"
	// StateCurrent means local matches the recorded upstream commit.
	StateCurrent SyncState = "current"
)

// deriveState computes the sync state for an entry, a live content hash of
// the checkout, and a freshly resolved remote SHA. Local divergence
// dominates: edits must never be silently clobbered even when upstream
// also moved. An empty remote SHA counts as unreachable.
func deriveState(entry RegistryEntry, localHash, remoteSHA string, resolveErr error) SyncState {
	if localHash != entry.ExportHash {
		return StateDiverged
	}

	if resolveErr != nil || remoteSHA == "" {
		return StateUnreachable
	}

	if remoteSHA != entry.ResolvedCommit {
		return StateBehind
	}

	return StateCurrent
}
