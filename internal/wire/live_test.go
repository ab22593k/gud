package wire

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// liveEnabled skips the caller unless live-network integration may run.
func liveEnabled(t *testing.T) {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping live-network test in short mode")
	}

	if os.Getenv("RUN_GITWIRE_INTEGRATION") == "" {
		t.Skip("set RUN_GITWIRE_INTEGRATION=1 for live-network tests")
	}
}

func TestLiveFetchOCAExample(t *testing.T) {
	liveEnabled(t)

	store := NewStoreWithDir(t.TempDir())
	fetcher := NewFetcher(store)

	source, err := ParseSourceURL("https://github.com/OCA/server-tools/tree/19.0/auto_backup")
	if err != nil {
		t.Fatalf("ParseSourceURL: %v", err)
	}

	run := t.TempDir()
	registry := RegistryPath(run)
	target := filepath.Join(run, "auto_backup")

	opts := FetchOptions{Fetcher: fetcher, RegistryPath: registry}

	summary, err := Fetch(context.Background(), opts, source, target)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if !strings.Contains(summary, "Fetched github.com/OCA/server-tools@19.0:auto_backup") {
		t.Fatalf("summary = %q", summary)
	}

	entries, err := os.ReadDir(target)
	if err != nil || len(entries) == 0 {
		t.Fatalf("empty target, err = %v", err)
	}

	reg, err := LoadRegistry(registry)
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}

	entry, err := reg.Lookup("./auto_backup")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}

	if entry.Ref != "19.0" || entry.Subpath != "auto_backup" || len(entry.ResolvedCommit) != 40 {
		t.Fatalf("entry = %+v", entry)
	}
}

func TestLiveUpdateNoOp(t *testing.T) {
	liveEnabled(t)

	store := NewStoreWithDir(t.TempDir())
	fetcher := NewFetcher(store)

	source, err := ParseSourceURL("https://github.com/OCA/server-tools/tree/19.0/auto_backup")
	if err != nil {
		t.Fatalf("ParseSourceURL: %v", err)
	}

	run := t.TempDir()
	registry := RegistryPath(run)
	target := filepath.Join(run, "auto_backup")

	opts := FetchOptions{Fetcher: fetcher, RegistryPath: registry}

	if _, err := Fetch(context.Background(), opts, source, target); err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	summary, err := Update(context.Background(), UpdateOptions{Fetcher: fetcher, RegistryPath: registry}, target)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	if !strings.Contains(summary, "Already up to date") {
		t.Fatalf("summary = %q", summary)
	}
}

func TestLiveListCurrent(t *testing.T) {
	liveEnabled(t)

	store := NewStoreWithDir(t.TempDir())
	fetcher := NewFetcher(store)

	source, err := ParseSourceURL("https://github.com/OCA/server-tools/tree/19.0/auto_backup")
	if err != nil {
		t.Fatalf("ParseSourceURL: %v", err)
	}

	root := t.TempDir()
	registry := RegistryPath(root)

	for _, name := range []string{"one", "two"} {
		_, err := Fetch(
			context.Background(),
			FetchOptions{Fetcher: fetcher, RegistryPath: registry},
			source,
			filepath.Join(root, name),
		)
		if err != nil {
			t.Fatalf("Fetch: %v", err)
		}
	}

	entries, err := List(context.Background(), fetcher, root)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}

	for _, e := range entries {
		if e.State != StateCurrent {
			t.Fatalf("%s state = %q", e.Dir, e.State)
		}
	}
}

func TestLiveMergeLocalAdd(t *testing.T) {
	liveEnabled(t)

	store := NewStoreWithDir(t.TempDir())
	fetcher := NewFetcher(store)

	source, err := ParseSourceURL("https://github.com/OCA/server-tools/tree/19.0/auto_backup")
	if err != nil {
		t.Fatalf("ParseSourceURL: %v", err)
	}

	run := t.TempDir()
	registry := RegistryPath(run)
	target := filepath.Join(run, "auto_backup")

	opts := FetchOptions{Fetcher: fetcher, RegistryPath: registry}

	if _, err := Fetch(context.Background(), opts, source, target); err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	const localFile = "wire-local-note.txt"

	if err := os.WriteFile(filepath.Join(target, localFile), []byte("local"), 0o600); err != nil {
		t.Fatalf("local add: %v", err)
	}

	summary, err := Update(context.Background(), UpdateOptions{Fetcher: fetcher, RegistryPath: registry}, target)

	switch {
	case errors.Is(err, ErrDiverged):
		// Upstream unmoved: same-commit divergence refusal. The local
		// file and record must survive untouched.
	case err != nil:
		t.Fatalf("Update: %v", err)
	case strings.Contains(summary, "Merged"):
		reg, err := LoadRegistry(registry)
		if err != nil {
			t.Fatalf("LoadRegistry: %v", err)
		}

		entry, err := reg.Lookup("./auto_backup")
		if err != nil {
			t.Fatalf("Lookup: %v", err)
		}

		live, err := HashDir(target)
		if err != nil {
			t.Fatalf("HashDir: %v", err)
		}

		if entry.ExportHash != live {
			t.Fatal("entry hash does not match merged checkout")
		}
	default:
		t.Fatalf("unexpected summary: %q", summary)
	}

	data, err := os.ReadFile(filepath.Join(target, localFile))
	if err != nil || string(data) != "local" {
		t.Fatalf("local file lost: %q, err = %v", data, err)
	}
}
