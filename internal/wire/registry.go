package wire

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Run-level tracking registry: one file per run directory mapping fetched
// targets to their sources (research D12). Fetched folders hold contents
// only; every domain function takes an explicit registry path so tests stay
// hermetic and never depend on process cwd.

// registryVersion is the only registry envelope version v1 reads. It lives
// on the envelope, never per entry.
const registryVersion = 1

// registryFileName is the run-level registry file. HashDir and snapshotDir
// exclude it as bookkeeping so a fetch with `-t .` stays self-contained.
const registryFileName = ".git-wire.json"

// maxEntryBytes bounds each serialized registry entry (SC-006 budget).
const maxEntryBytes = 10 * 1024

// RegistryEntry is one tracked target's source association: the sole input
// the update operation needs. Field shapes match the retired per-checkout
// record so fixtures migrate by re-keying.
type RegistryEntry struct {
	SourceURL      string `json:"source_url"`
	Host           string `json:"host"`
	Owner          string `json:"owner"`
	Repo           string `json:"repo"`
	Ref            string `json:"ref"`
	Subpath        string `json:"subpath"`
	ResolvedCommit string `json:"resolved_commit"`
	ExportHash     string `json:"export_hash"`
	FetchedAt      string `json:"fetched_at"`
	UpdatedAt      string `json:"updated_at"`
}

// Registry is the versioned envelope holding one entry per fetched target.
type Registry struct {
	Version int                      `json:"version"`
	Entries map[string]RegistryEntry `json:"entries"`
}

// EntryFor builds the initial entry for a fresh fetch.
func EntryFor(source SourceRef, commit, exportHash string, now time.Time) RegistryEntry {
	stamp := now.UTC().Format(time.RFC3339)

	return RegistryEntry{
		SourceURL:      source.SourceURL,
		Host:           source.Host,
		Owner:          source.Owner,
		Repo:           source.Repo,
		Ref:            source.Ref,
		Subpath:        source.Subpath,
		ResolvedCommit: commit,
		ExportHash:     exportHash,
		FetchedAt:      stamp,
		UpdatedAt:      stamp,
	}
}

// Source rebuilds the source identity from an entry, preserving the
// verbatim fetch URL. Entries store resolved refs, so update re-resolves
// the same upstream without re-parsing user input.
func (e RegistryEntry) Source() SourceRef {
	return SourceRef{
		Host:      e.Host,
		Owner:     e.Owner,
		Repo:      e.Repo,
		Ref:       e.Ref,
		Subpath:   e.Subpath,
		SourceURL: e.SourceURL,
	}
}

// RegistryPath returns the registry file path for a run directory.
func RegistryPath(runDir string) string {
	return filepath.Join(runDir, registryFileName)
}

// Keys returns the entry keys in sorted order.
func (r Registry) Keys() []string {
	keys := make([]string, 0, len(r.Entries))
	for k := range r.Entries {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	return keys
}

// Lookup returns the entry for key, normalizing before compare. A missing
// key reports ErrNotACheckout; a malformed key reports ErrInvalidRecord.
func (r Registry) Lookup(key string) (RegistryEntry, error) {
	normal, err := normalizeKey(key)
	if err != nil {
		return RegistryEntry{}, err
	}

	entry, ok := r.Entries[normal]
	if !ok {
		return RegistryEntry{}, fmt.Errorf("no entry for %s: %w", normal, ErrNotACheckout)
	}

	return entry, nil
}

// Upsert stores entry under key (normalized first), creating the entries
// map when nil. The caller persists with SaveRegistry.
func (r *Registry) Upsert(key string, entry RegistryEntry) error {
	normal, err := normalizeKey(key)
	if err != nil {
		return err
	}

	if err := checkEntry(entry); err != nil {
		return fmt.Errorf("entry %s: %w", normal, err)
	}

	if r.Entries == nil {
		r.Entries = map[string]RegistryEntry{}
	}

	r.Entries[normal] = entry

	return nil
}

// LoadRegistry reads the registry at path. A missing file reads as an empty
// registry; a present-but-broken file reports ErrInvalidRecord naming the
// defect instead of guessing a source. One bad entry fails the load.
func LoadRegistry(path string) (Registry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Registry{Version: registryVersion, Entries: map[string]RegistryEntry{}}, nil
		}

		return Registry{}, fmt.Errorf("read %s: %w", path, err)
	}

	var raw Registry

	if err := json.Unmarshal(data, &raw); err != nil {
		return Registry{}, fmt.Errorf("registry %s is not JSON: %w", path, ErrInvalidRecord)
	}

	if raw.Version != registryVersion {
		return Registry{}, fmt.Errorf("registry %s: version %d unsupported (reads v%d): %w",
			path, raw.Version, registryVersion, ErrInvalidRecord)
	}

	clean, err := canonicalize(path, raw.Entries)
	if err != nil {
		return Registry{}, err
	}

	return Registry{Version: registryVersion, Entries: clean}, nil
}

// SaveRegistry writes reg atomically (temp file + rename) to path, creating
// the parent directory when absent. Each entry MUST serialize under the
// per-folder budget; the version MUST be the one v1 reads.
func SaveRegistry(path string, reg Registry) error {
	if reg.Version != registryVersion {
		return fmt.Errorf("registry %s: version %d unwritable (writes v%d): %w",
			path, reg.Version, registryVersion, ErrInvalidRecord)
	}

	clean, err := canonicalize(path, reg.Entries)
	if err != nil {
		return err
	}

	for key, entry := range clean {
		data, err := json.Marshal(entry)
		if err != nil {
			return fmt.Errorf("encode registry entry %s: %w", key, err)
		}

		if len(data) > maxEntryBytes {
			return fmt.Errorf("registry entry %s %d bytes exceeds budget: %w", key, len(data), ErrInvalidRecord)
		}
	}

	data, err := json.MarshalIndent(Registry{Version: registryVersion, Entries: clean}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode registry: %w", err)
	}

	if err := installFile(path, data); err != nil {
		return err
	}

	return nil
}

// installFile writes data to path via temp file + rename so a crash cannot
// leave a half-written registry behind.
func installFile(path string, data []byte) error {
	dir := filepath.Dir(path)

	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create registry dir: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".git-wire-*.tmp")
	if err != nil {
		return fmt.Errorf("stage registry: %w", err)
	}

	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)

		return fmt.Errorf("write registry: %w", err)
	}

	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)

		return fmt.Errorf("close registry: %w", err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)

		return fmt.Errorf("install registry: %w", err)
	}

	return nil
}

// canonicalize normalizes keys and validates entries, failing closed on the
// first defect: colliding keys after normalization count as corrupt.
func canonicalize(path string, entries map[string]RegistryEntry) (map[string]RegistryEntry, error) {
	clean := make(map[string]RegistryEntry, len(entries))

	for key, entry := range entries {
		normal, err := normalizeKey(key)
		if err != nil {
			return nil, fmt.Errorf("registry %s: %w", path, err)
		}

		if _, dup := clean[normal]; dup {
			return nil, fmt.Errorf("registry %s: duplicate entry %s: %w", path, normal, ErrInvalidRecord)
		}

		if err := checkEntry(entry); err != nil {
			return nil, fmt.Errorf("registry %s entry %s: %w", path, normal, err)
		}

		clean[normal] = entry
	}

	return clean, nil
}

// KeyFor maps an absolute target to its registry-relative key under the
// registry's directory. Targets outside the registry tree report
// ErrNotACheckout: this registry does not track them.
func KeyFor(registryPath, target string) (string, error) {
	absReg, err := filepath.Abs(registryPath)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", registryPath, err)
	}

	rel, err := filepath.Rel(filepath.Dir(absReg), target)
	if err != nil {
		return "", fmt.Errorf("relate %s: %w", target, err)
	}

	if rel == "." {
		return ".", nil
	}

	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("target %s is outside the registry directory %s: %w",
			target, filepath.Dir(absReg), ErrNotACheckout)
	}

	return normalizeKey("./" + filepath.ToSlash(rel))
}

// normalizeKey canonicalizes one registry key: registry-relative slash
// paths (`./auto_backup`); absolute, empty, or `..`-escaping keys are
// rejected as invalid. A lone `"."` addresses the run directory itself.
func normalizeKey(key string) (string, error) {
	if key == "" {
		return "", fmt.Errorf("empty registry key: %w", ErrInvalidRecord)
	}

	if filepath.IsAbs(key) || filepath.IsAbs(filepath.FromSlash(key)) {
		return "", fmt.Errorf("absolute registry key %q: %w", key, ErrInvalidRecord)
	}

	slash := filepath.ToSlash(key)
	if slash == "." {
		return ".", nil
	}

	cleaned := path.Clean(strings.TrimPrefix(slash, "./"))
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("escaping registry key %q: %w", key, ErrInvalidRecord)
	}

	return "./" + cleaned, nil
}

// checkEntry enforces the v1 entry schema: commit is 40 lowercase hex,
// export hash is 64 lowercase hex, timestamps parse when set.
func checkEntry(entry RegistryEntry) error {
	if !isLowerHex(entry.ResolvedCommit, 40) {
		return fmt.Errorf("bad resolved_commit: %w", ErrInvalidRecord)
	}

	if !isLowerHex(entry.ExportHash, 64) {
		return fmt.Errorf("bad export_hash: %w", ErrInvalidRecord)
	}

	for _, stamp := range []string{entry.FetchedAt, entry.UpdatedAt} {
		if stamp == "" {
			continue
		}

		if _, err := time.Parse(time.RFC3339, stamp); err != nil {
			return fmt.Errorf("bad timestamp %q: %w", stamp, ErrInvalidRecord)
		}
	}

	if entry.SourceURL == "" || entry.Host == "" || entry.Subpath == "" {
		return fmt.Errorf("missing source identity: %w", ErrInvalidRecord)
	}

	return nil
}

// isLowerHex reports whether s is exactly n lowercase hex digits.
func isLowerHex(s string, n int) bool {
	if len(s) != n {
		return false
	}

	for _, r := range s {
		isHex := r >= '0' && r <= '9' || r >= 'a' && r <= 'f'
		if !isHex {
			return false
		}
	}

	return true
}
