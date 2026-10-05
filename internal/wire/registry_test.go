package wire

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testEntry() RegistryEntry {
	return EntryFor(
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

func saveFixtureRegistry(t *testing.T, reg Registry) string {
	t.Helper()

	path := RegistryPath(t.TempDir())

	if err := SaveRegistry(path, reg); err != nil {
		t.Fatalf("SaveRegistry: %v", err)
	}

	return path
}

func TestRegistryRoundTrip(t *testing.T) {
	t.Parallel()

	want := Registry{Version: registryVersion, Entries: map[string]RegistryEntry{"./auto_backup": testEntry()}}
	path := saveFixtureRegistry(t, want)

	got, err := LoadRegistry(path)
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}

	if got.Version != registryVersion {
		t.Fatalf("version = %d", got.Version)
	}

	entry, err := got.Lookup("./auto_backup")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}

	if entry != testEntry() {
		t.Fatalf("got %+v, want %+v", entry, testEntry())
	}
}

func TestLoadRegistryMissing(t *testing.T) {
	t.Parallel()

	got, err := LoadRegistry(RegistryPath(t.TempDir()))
	if err != nil {
		t.Fatalf("missing file must read as empty: %v", err)
	}

	if got.Version != registryVersion || len(got.Entries) != 0 {
		t.Fatalf("got %+v, want empty v1 registry", got)
	}
}

func envelopeJSON(entryKey string, entry map[string]any) string {
	data, err := json.Marshal(map[string]any{"version": 1, "entries": map[string]any{entryKey: entry}})
	if err != nil {
		panic(err)
	}

	return string(data)
}

func validEntryMap() map[string]any {
	return map[string]any{
		"source_url":      "https://github.com/OCA/server-tools/tree/19.0/auto_backup",
		"host":            "github.com",
		"owner":           "OCA",
		"repo":            "server-tools",
		"ref":             "19.0",
		"subpath":         "auto_backup",
		"resolved_commit": strings.Repeat("a", 40),
		"export_hash":     strings.Repeat("b", 64),
	}
}

func TestLoadRegistryCorrupt(t *testing.T) {
	t.Parallel()

	short := validEntryMap()
	short["resolved_commit"] = "abc"

	upper := validEntryMap()
	upper["resolved_commit"] = strings.Repeat("A", 40)

	badStamp := validEntryMap()
	badStamp["fetched_at"] = "yesterday"

	entryData, err := json.Marshal(validEntryMap())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	colliding := `{"version":1,"entries":{"x":` + string(entryData) + `,"./x":` + string(entryData) + `}}`

	cases := []struct {
		name    string
		content string
	}{
		{name: "not json", content: "{oops"},
		{name: "wrong version", content: `{"version":3,"entries":{}}`},
		{name: "missing version", content: `{"entries":{}}`},
		{name: "short commit", content: envelopeJSON("./x", short)},
		{name: "uppercase commit", content: envelopeJSON("./x", upper)},
		{name: "bad timestamp", content: envelopeJSON("./x", badStamp)},
		{name: "absolute key", content: envelopeJSON("/abs", validEntryMap())},
		{name: "escaping key", content: envelopeJSON("../out", validEntryMap())},
		{name: "colliding keys", content: colliding},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			path := RegistryPath(t.TempDir())

			if err := os.WriteFile(path, []byte(tc.content), 0o600); err != nil {
				t.Fatalf("fixture: %v", err)
			}

			if _, err := LoadRegistry(path); !errors.Is(err, ErrInvalidRecord) {
				t.Fatalf("err = %v, want ErrInvalidRecord", err)
			}
		})
	}
}

func TestRegistryKeyNormalization(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want string
	}{
		{in: "./auto_backup", want: "./auto_backup"},
		{in: "auto_backup", want: "./auto_backup"},
		{in: "./mods/../auto_backup", want: "./auto_backup"},
		{in: ".", want: "."},
	}

	for _, tc := range cases {
		if got, err := normalizeKey(tc.in); err != nil || got != tc.want {
			t.Fatalf("normalizeKey(%q) = %q, %v; want %q", tc.in, got, err, tc.want)
		}
	}

	for _, bad := range []string{"", "/", "/abs/path", "..", "../out", "x/../../out"} {
		if _, err := normalizeKey(bad); !errors.Is(err, ErrInvalidRecord) {
			t.Fatalf("normalizeKey(%q) = nil, want ErrInvalidRecord", bad)
		}
	}
}

func TestKeyFor(t *testing.T) {
	t.Parallel()

	run := t.TempDir()
	regPath := RegistryPath(run)

	key, err := KeyFor(regPath, filepath.Join(run, "auto_backup"))
	if err != nil {
		t.Fatalf("KeyFor: %v", err)
	}

	if key != "./auto_backup" {
		t.Fatalf("key = %q", key)
	}

	if key, err := KeyFor(regPath, run); err != nil || key != "." {
		t.Fatalf("run-dir key = %q, %v", key, err)
	}

	if _, err := KeyFor(regPath, filepath.Join(t.TempDir(), "elsewhere")); !errors.Is(err, ErrNotACheckout) {
		t.Fatalf("err = %v, want ErrNotACheckout", err)
	}
}

func TestRegistryUpsertLookup(t *testing.T) {
	t.Parallel()

	var reg Registry

	if err := reg.Upsert("auto_backup", testEntry()); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	if _, err := reg.Lookup("./auto_backup"); err != nil {
		t.Fatalf("Lookup normalized: %v", err)
	}

	if _, err := reg.Lookup("./missing"); !errors.Is(err, ErrNotACheckout) {
		t.Fatalf("err = %v, want ErrNotACheckout", err)
	}

	if err := reg.Upsert("/abs", testEntry()); !errors.Is(err, ErrInvalidRecord) {
		t.Fatalf("Upsert abs = nil, want ErrInvalidRecord")
	}
}

func TestRegistryEntrySizeBudget(t *testing.T) {
	t.Parallel()

	entry := testEntry()
	entry.SourceURL = "https://github.com/" + strings.Repeat("o", 500) + "/" + strings.Repeat("r", 500) +
		"/tree/" + strings.Repeat("v", 200) + "/" + strings.Repeat("p", 500)

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if len(data) > maxEntryBytes {
		t.Fatalf("entry %d bytes exceeds %d budget", len(data), maxEntryBytes)
	}

	path := saveFixtureRegistry(t, Registry{Version: registryVersion, Entries: map[string]RegistryEntry{"./x": entry}})

	if _, err := LoadRegistry(path); err != nil {
		t.Fatalf("budget-fitting entry rejected: %v", err)
	}
}

func TestRegistryEntryBudgetEnforced(t *testing.T) {
	t.Parallel()

	entry := testEntry()
	entry.SourceURL = "https://github.com/" + strings.Repeat("o", 11000) + "/x/tree/v/p"

	overBudget := Registry{Version: registryVersion, Entries: map[string]RegistryEntry{"./x": entry}}

	if err := SaveRegistry(RegistryPath(t.TempDir()), overBudget); !errors.Is(err, ErrInvalidRecord) {
		t.Fatalf("err = %v, want budget ErrInvalidRecord", err)
	}
}

func TestRegistryPathUsesConstant(t *testing.T) {
	t.Parallel()

	if got := filepath.Base(RegistryPath("/run")); got != ".git-wire.json" {
		t.Fatalf("registry file = %q", got)
	}
}

func TestRegistryEntrySourceRoundTrip(t *testing.T) {
	t.Parallel()

	entry := testEntry()
	src := entry.Source()

	if src.Host != entry.Host || src.Owner != entry.Owner || src.Repo != entry.Repo {
		t.Fatalf("identity lost: %+v", src)
	}

	if src.Ref != entry.Ref || src.Subpath != entry.Subpath || src.SourceURL != entry.SourceURL {
		t.Fatalf("reference lost: %+v", src)
	}
}
