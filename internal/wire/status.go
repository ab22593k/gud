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

// derivePinned computes a pin's state from the local content hash only:
// current when the checkout matches the recorded export, diverged on any
// local edit. Pins float nowhere, so behind and unreachable are impossible.
func derivePinned(entry RegistryEntry, localHash string) SyncState {
	if localHash != entry.ExportHash {
		return StateDiverged
	}

	return StateCurrent
}

// describePinned derives a pinned entry's state without touching the
// fetcher. The fetcher argument exists only so tests can seal the
// no-resolution contract by counting Resolve calls.
func describePinned(_ string, entry RegistryEntry, live string, _ Fetcher) SyncState {
	return derivePinned(entry, live)
}

// deriveState computes the sync state for an entry, a live content hash of
// the checkout, and a freshly resolved remote SHA. Local divergence
// dominates: edits must never be silently clobbered even when upstream
// also moved. An empty remote SHA counts as unreachable.
func deriveState(
	entry RegistryEntry,
	localHash string,
	remoteSHA string,
	resolveErr error,
) SyncState {
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
