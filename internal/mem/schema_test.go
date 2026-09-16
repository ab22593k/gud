package mem

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	helix "github.com/helixdb/helix-db/sdks/go"
)

func TestSchemaIndexTraversals_Count(t *testing.T) {
	t.Parallel()

	if got := len(schemaIndexTraversals()); got != 23 {
		t.Fatalf("schemaIndexTraversals()=%d, want 23", got)
	}
}

func TestBuildSchemaMigrationQuery_SingleBatch(t *testing.T) {
	t.Parallel()

	req := buildSchemaMigrationQuery()

	if err := req.Validate(); err != nil {
		t.Fatalf("Validate()=%v, want nil", err)
	}

	raw, err := helix.MarshalRequest(req)
	if err != nil {
		t.Fatalf("MarshalRequest()=%v, want nil", err)
	}

	var decoded struct {
		RequestType string  `json:"request_type"`
		QueryName   *string `json:"query_name"`
		Query       map[string]struct {
			Entries []struct {
				Query struct {
					Name string `json:"name"`
				} `json:"query"`
			} `json:"entries"`
			Returns []string `json:"returns"`
		} `json:"query"`
	}

	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal()=%v, want nil", err)
	}

	if decoded.RequestType != "write" {
		t.Errorf("request_type=%q, want %q", decoded.RequestType, "write")
	}

	if decoded.QueryName == nil || *decoded.QueryName != schemaQueryName {
		t.Errorf("query_name=%v, want %q", decoded.QueryName, schemaQueryName)
	}

	entries := decoded.Query["write"].Entries
	if len(entries) != 23 {
		t.Fatalf("entries=%d, want 23 in one transaction", len(entries))
	}

	seen := make(map[string]bool, len(entries))

	for _, entry := range entries {
		if entry.Query.Name == "" {
			t.Error("entry with empty name, want unique idx_NN")
		}

		if seen[entry.Query.Name] {
			t.Errorf("duplicate entry name %q, want unique names", entry.Query.Name)
		}

		seen[entry.Query.Name] = true
	}
}

func TestSchemaMarker_RoundTrip(t *testing.T) {
	t.Parallel()

	db := NewDB(Options{DataDir: t.TempDir(), Database: "gud-test", Enabled: false})

	if db.isSchemaCurrent() {
		t.Fatal("isSchemaCurrent()=true before marking, want false")
	}

	if err := db.markSchemaCurrent(); err != nil {
		t.Fatalf("markSchemaCurrent()=%v, want nil", err)
	}

	if !db.isSchemaCurrent() {
		t.Fatal("isSchemaCurrent()=false after marking, want true")
	}

	data, err := os.ReadFile(db.schemaMarkerPath())
	if err != nil {
		t.Fatalf("ReadFile()=%v, want nil", err)
	}

	if string(data) != strconv.Itoa(currentSchemaVersion) {
		t.Errorf("marker=%q, want %q", string(data), strconv.Itoa(currentSchemaVersion))
	}
}

func TestSchemaMarker_States(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func(path string)
		want  bool
	}{
		{name: "missing file", setup: func(string) {}, want: false},
		{name: "stale version", setup: func(path string) {
			_ = os.WriteFile(path, []byte("0"), 0o600)
		}, want: false},
		{name: "future version", setup: func(path string) {
			_ = os.WriteFile(path, []byte(strconv.Itoa(currentSchemaVersion+1)), 0o600)
		}, want: false},
		{name: "current with whitespace", setup: func(path string) {
			_ = os.WriteFile(path, []byte("  "+strconv.Itoa(currentSchemaVersion)+"\n"), 0o600)
		}, want: true},
		{name: "empty file", setup: func(path string) {
			_ = os.WriteFile(path, []byte(""), 0o600)
		}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			db := NewDB(Options{DataDir: t.TempDir(), Database: "gud-test", Enabled: false})
			tt.setup(db.schemaMarkerPath())

			if got := db.isSchemaCurrent(); got != tt.want {
				t.Errorf("isSchemaCurrent()=%v, want %v", got, tt.want)
			}
		})
	}
}

func TestSchemaMarkerPath_IsolatedByDatabase(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	first := NewDB(Options{DataDir: dir, Database: "db-one", Enabled: false})
	second := NewDB(Options{DataDir: dir, Database: "db-two", Enabled: false})

	if first.schemaMarkerPath() == second.schemaMarkerPath() {
		t.Error("marker paths collide across databases, want isolated paths")
	}

	if want := filepath.Join(dir, "db-one.schema_version"); first.schemaMarkerPath() != want {
		t.Errorf("marker path=%q, want %q", first.schemaMarkerPath(), want)
	}
}

func TestSchemaMarkerPath_Empty(t *testing.T) {
	t.Parallel()

	var nilDB *DB
	if got := nilDB.schemaMarkerPath(); got != "" {
		t.Errorf("nil marker path=%q, want empty", got)
	}

	empty := &DB{}
	if got := empty.schemaMarkerPath(); got != "" {
		t.Errorf("empty marker path=%q, want empty", got)
	}

	if empty.isSchemaCurrent() {
		t.Error("isSchemaCurrent()=true for empty DB, want false")
	}

	if err := empty.markSchemaCurrent(); err == nil {
		t.Error("markSchemaCurrent()=nil for empty DB, want error")
	}
}
