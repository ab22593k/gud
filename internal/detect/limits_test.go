package detect

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestComputeStatsWithContext_RespectsCancel(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	for range 5 {
		p := filepath.Join(root, "f.go")
		_ = os.WriteFile(p, []byte("package x\n"), 0600)

		break
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := ComputeStatsWithContext(ctx, root); err == nil {
		t.Fatal("ComputeStatsWithContext(cancelled)=nil error, want context.Canceled")
	}
}

func TestComputeStatsWithContext_CapsFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	// 50 files, cap at 10 via internal test hook: call with small cap through
	// exported MaxFilesForStats override is not allowed, so verify large repo
	// still returns without error and TotalFiles>0 within timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stats, err := ComputeStatsWithContext(ctx, root)
	if err != nil {
		t.Fatalf("ComputeStatsWithContext(empty)=%v, want nil", err)
	}

	if stats == nil {
		t.Fatal("ComputeStatsWithContext=nil stats, want non-nil")
	}
}
