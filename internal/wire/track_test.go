package wire

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testRecord() TrackingRecord {
	return RecordFor(
		SourceRef{
			Host:      "github.com",
			Owner:     "OCA",
			Repo:      "server-tools",
			Ref:       "19.0",
			Subpath:   "auto_backup",
			SourceURL: "https://github.com/OCA/server-tools/tree/19.0/auto_backup",
		},
		strings.Repeat("a", 40),
		strings.Repeat("b", 64),
		time.Date(2026, 10, 5, 9, 55, 0, 0, time.UTC),
	)
}

func TestRecordRoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	want := testRecord()

	if err := SaveRecord(dir, want); err != nil {
		t.Fatalf("SaveRecord: %v", err)
	}

	got, err := LoadRecord(dir)
	if err != nil {
		t.Fatalf("LoadRecord: %v", err)
	}

	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestLoadRecordMissing(t *testing.T) {
	t.Parallel()

	if _, err := LoadRecord(t.TempDir()); !errors.Is(err, ErrNotACheckout) {
		t.Fatalf("err = %v, want ErrNotACheckout", err)
	}
}

func TestLoadRecordCorrupt(t *testing.T) {
	t.Parallel()

	commit := strings.Repeat("a", 40)
	hash := strings.Repeat("b", 64)

	cases := []struct {
		name    string
		content string
	}{
		{name: "not json", content: "{oops"},
		{name: "wrong version", content: `{"version":3}`},
		{name: "short commit", content: `{"version":1,"resolved_commit":"abc","export_hash":"` + hash + `"}`},
		{
			name:    "uppercase commit",
			content: `{"version":1,"resolved_commit":"` + strings.Repeat("A", 40) + `","export_hash":"` + hash + `"}`,
		},
		{
			name:    "bad timestamp",
			content: `{"version":1,"resolved_commit":"` + commit + `","export_hash":"` + hash + `","fetched_at":"yesterday"}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()

			if err := os.WriteFile(RecordPath(dir), []byte(tc.content), 0o600); err != nil {
				t.Fatalf("fixture: %v", err)
			}

			if _, err := LoadRecord(dir); !errors.Is(err, ErrInvalidRecord) {
				t.Fatalf("err = %v, want ErrInvalidRecord", err)
			}
		})
	}
}

func TestRecordSizeBudget(t *testing.T) {
	t.Parallel()

	rec := testRecord()
	rec.SourceURL = "https://github.com/" + strings.Repeat("o", 500) + "/" + strings.Repeat("r", 500) +
		"/tree/" + strings.Repeat("v", 200) + "/" + strings.Repeat("p", 500)

	dir := t.TempDir()

	if err := SaveRecord(dir, rec); err != nil {
		t.Fatalf("SaveRecord: %v", err)
	}

	info, err := os.Stat(RecordPath(dir))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	if info.Size() > maxRecordBytes {
		t.Fatalf("record %d bytes exceeds %d budget", info.Size(), maxRecordBytes)
	}
}

func TestRecordPathUsesConstant(t *testing.T) {
	t.Parallel()

	if got := filepath.Base(RecordPath("/x")); got != ".git-wire.json" {
		t.Fatalf("record file = %q", got)
	}
}

func TestRecordSourceRoundTrip(t *testing.T) {
	t.Parallel()

	rec := testRecord()
	src := rec.Source()

	if src.Host != rec.Host || src.Owner != rec.Owner || src.Repo != rec.Repo {
		t.Fatalf("identity lost: %+v", src)
	}

	if src.Ref != rec.Ref || src.Subpath != rec.Subpath || src.SourceURL != rec.SourceURL {
		t.Fatalf("reference lost: %+v", src)
	}
}
