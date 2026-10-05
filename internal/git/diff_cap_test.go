package git

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestCappedWriter_EnforcesBound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		max           int
		writes        []string
		wantLen       int
		wantTruncated bool
	}{
		{name: "under bound", max: 8, writes: []string{"abc"}, wantLen: 3},
		{name: "exact bound", max: 3, writes: []string{"abc"}, wantLen: 3},
		{name: "single overrun", max: 3, writes: []string{"abcdef"}, wantLen: 3, wantTruncated: true},
		{name: "cumulative overrun", max: 5, writes: []string{"ab", "cd", "ef"}, wantLen: 5, wantTruncated: true},
		{name: "writes after full are dropped", max: 2, writes: []string{"ab", "cd"}, wantLen: 2, wantTruncated: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			w := newCappedWriter(tt.max)

			for _, s := range tt.writes {
				n, err := w.Write([]byte(s))
				if err != nil {
					t.Fatalf("Write() error = %v", err)
				}

				if n != len(s) {
					t.Errorf("Write() = %d, want %d (callers assume full consumption)", n, len(s))
				}
			}

			if w.buf.Len() != tt.wantLen {
				t.Errorf("buffered = %d, want %d", w.buf.Len(), tt.wantLen)
			}

			if w.truncated != tt.wantTruncated {
				t.Errorf("truncated = %v, want %v", w.truncated, tt.wantTruncated)
			}
		})
	}
}

func TestGetStagedChanges_TruncatesLargeDiff(t *testing.T) {
	newConfiguredRepo(t)

	ctx := context.Background()

	big := strings.Repeat("x", MaxDiffBytes+65536) + "\n"
	if err := os.WriteFile("big.txt", []byte(big), 0o600); err != nil {
		t.Fatalf("write big.txt: %v", err)
	}

	runGitInTest(t, "add", "big.txt")

	sc, err := GetStagedChanges(ctx)
	if err != nil {
		t.Fatalf("GetStagedChanges() error = %v", err)
	}

	if !sc.Truncated {
		t.Fatal("Truncated = false for a diff larger than MaxDiffBytes")
	}

	if len(sc.Diff) > MaxDiffBytes+1024 {
		t.Errorf("Diff = %d bytes, want bounded near MaxDiffBytes (%d)", len(sc.Diff), MaxDiffBytes)
	}

	if !strings.Contains(sc.Diff, "truncated by gud") {
		t.Error("Diff missing truncation notice")
	}
}

func TestGetStagedChanges_DeletedNamesSurviveTruncation(t *testing.T) {
	newConfiguredRepo(t)

	ctx := context.Background()

	// "a-" sorts before "z-": the huge addition fills the capped prefix so the
	// deletion entry falls beyond the cutoff, exercising the by-name fallback.
	if err := os.WriteFile("z-victim.go", []byte("package main\n"), 0o600); err != nil {
		t.Fatalf("write z-victim.go: %v", err)
	}

	runGitInTest(t, "add", ".")

	if _, err := Commit(ctx, "init"); err != nil {
		t.Fatalf("initial commit: %v", err)
	}

	big := strings.Repeat("y", MaxDiffBytes) + "\n"
	if err := os.WriteFile("a-huge.txt", []byte(big), 0o600); err != nil {
		t.Fatalf("write a-huge.txt: %v", err)
	}

	if err := os.Remove("z-victim.go"); err != nil {
		t.Fatalf("remove z-victim.go: %v", err)
	}

	runGitInTest(t, "add", "-A")

	sc, err := GetStagedChanges(ctx)
	if err != nil {
		t.Fatalf("GetStagedChanges() error = %v", err)
	}

	if !sc.Truncated {
		t.Fatal("Truncated = false, precondition for the fallback path failed")
	}

	found := false

	for _, d := range sc.Deleted {
		if d == "z-victim.go" {
			found = true
		}
	}

	if !found {
		t.Errorf("Deleted = %v, want it to contain z-victim.go despite truncation", sc.Deleted)
	}
}

func TestGetStagedChanges_SmallDiffNotTruncated(t *testing.T) {
	newConfiguredRepo(t)

	ctx := context.Background()

	if err := os.WriteFile("small.txt", []byte("hello\n"), 0o600); err != nil {
		t.Fatalf("write small.txt: %v", err)
	}

	runGitInTest(t, "add", "small.txt")

	sc, err := GetStagedChanges(ctx)
	if err != nil {
		t.Fatalf("GetStagedChanges() error = %v", err)
	}

	if sc.Truncated {
		t.Error("Truncated = true for a small diff")
	}

	if strings.Contains(sc.Diff, "truncated by gud") {
		t.Error("small diff carries a truncation notice")
	}
}
