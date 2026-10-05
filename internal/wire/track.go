package wire

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Tracking record persistence for checkouts.

// recordVersion is the only tracking record version v1 reads.
const recordVersion = 1

// maxRecordBytes bounds serialized tracking records (SC-006 budget).
const maxRecordBytes = 10 * 1024

// TrackingRecord is the colocated state that makes a directory a checkout.
// It is the sole input the update operation needs.
type TrackingRecord struct {
	Version        int    `json:"version"`
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

// RecordFor builds the initial record for a fresh fetch.
func RecordFor(source SourceRef, commit, exportHash string, now time.Time) TrackingRecord {
	stamp := now.UTC().Format(time.RFC3339)

	return TrackingRecord{
		Version:        recordVersion,
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

// Source rebuilds the source identity from a record, preserving the
// verbatim fetch URL. Records store resolved refs, so update re-resolves
// the same upstream without re-parsing user input.
func (r TrackingRecord) Source() SourceRef {
	return SourceRef{
		Host:      r.Host,
		Owner:     r.Owner,
		Repo:      r.Repo,
		Ref:       r.Ref,
		Subpath:   r.Subpath,
		SourceURL: r.SourceURL,
	}
}

// SaveRecord writes rec atomically (temp file + rename) into dir.
func SaveRecord(dir string, rec TrackingRecord) error {
	if err := checkRecord(rec); err != nil {
		return err
	}

	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return fmt.Errorf("encode tracking record: %w", err)
	}

	if len(data) > maxRecordBytes {
		return fmt.Errorf("tracking record %d bytes exceeds budget: %w", len(data), ErrInvalidRecord)
	}

	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create checkout dir: %w", err)
	}

	path := RecordPath(dir)

	tmp, err := os.CreateTemp(dir, ".git-wire-*.tmp")
	if err != nil {
		return fmt.Errorf("stage tracking record: %w", err)
	}

	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)

		return fmt.Errorf("write tracking record: %w", err)
	}

	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)

		return fmt.Errorf("close tracking record: %w", err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)

		return fmt.Errorf("install tracking record: %w", err)
	}

	return nil
}

// LoadRecord reads the tracking record in dir. A missing file reports
// ErrNotACheckout; a present-but-broken file reports ErrInvalidRecord
// naming the defect instead of guessing a source.
func LoadRecord(dir string) (TrackingRecord, error) {
	path := RecordPath(dir)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return TrackingRecord{}, fmt.Errorf("no record at %s: %w", path, ErrNotACheckout)
		}

		return TrackingRecord{}, fmt.Errorf("read %s: %w", path, err)
	}

	var rec TrackingRecord

	if err := json.Unmarshal(data, &rec); err != nil {
		return TrackingRecord{}, fmt.Errorf("record %s is not JSON: %w", path, ErrInvalidRecord)
	}

	if err := checkRecord(rec); err != nil {
		return TrackingRecord{}, fmt.Errorf("record %s: %w", path, err)
	}

	return rec, nil
}

// checkRecord enforces the v1 schema: version equals 1, commit is 40
// lowercase hex, export hash is 64 lowercase hex, timestamps parse when set.
func checkRecord(rec TrackingRecord) error {
	if rec.Version != recordVersion {
		return fmt.Errorf("version %d unsupported (reads v%d): %w", rec.Version, recordVersion, ErrInvalidRecord)
	}

	if !isLowerHex(rec.ResolvedCommit, 40) {
		return fmt.Errorf("bad resolved_commit: %w", ErrInvalidRecord)
	}

	if !isLowerHex(rec.ExportHash, 64) {
		return fmt.Errorf("bad export_hash: %w", ErrInvalidRecord)
	}

	for _, stamp := range []string{rec.FetchedAt, rec.UpdatedAt} {
		if stamp == "" {
			continue
		}

		if _, err := time.Parse(time.RFC3339, stamp); err != nil {
			return fmt.Errorf("bad timestamp %q: %w", stamp, ErrInvalidRecord)
		}
	}

	if rec.SourceURL == "" || rec.Host == "" || rec.Subpath == "" {
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
