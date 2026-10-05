// Package wire fetches single subfolders from hosted git repositories,
// tracks them for later updates, and reports their sync state.
//
// A checkout is a plain directory holding exported files plus one colocated
// tracking record (.git-wire.json). Shared blobless git mirrors live under
// the wire cache directory. See specs/001-git-wire-fetch for the contracts.
package wire
