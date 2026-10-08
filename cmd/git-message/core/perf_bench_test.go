package core

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gud/internal/detect"
	"gud/internal/git"
)

// These benchmarks measure the part of an invocation gud controls: everything
// between starting the command and issuing the model request. The API call
// itself dominates wall-clock in production, so the point of tracking these is
// to keep the local work from creeping, not to shave milliseconds off a
// multi-second round trip.
//
// Timings on a loaded workstation vary several-fold; allocation counts are
// deterministic and are the more reliable signal for regressions.

// benchMonorepo builds a synthetic monorepo: nDirs packages, each holding a Go
// file and its own AGENTS.md, plus ignored and vendored trees that must be
// pruned.
func benchMonorepo(b *testing.B, nDirs int) string {
	b.Helper()

	root := b.TempDir()

	if err := os.MkdirAll(filepath.Join(root, ".git"), 0750); err != nil {
		b.Fatal(err)
	}

	write := func(rel, content string) {
		b.Helper()

		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0750); err != nil {
			b.Fatal(err)
		}

		if err := os.WriteFile(full, []byte(content), 0600); err != nil {
			b.Fatal(err)
		}
	}

	write(".gitignore", "node_modules/\nvendor/\n")
	write("AGENTS.md", "# Conventions\n\nUse conventional commits.\n")

	for i := range nDirs {
		write(fmt.Sprintf("services/svc%03d/main.go", i), strings.Repeat("package main\n", 20))
		write(fmt.Sprintf("services/svc%03d/AGENTS.md", i), "svc conventions\n")
	}

	// Trees that must be pruned rather than mounted.
	for _, d := range []string{"node_modules/dep/pkg", "vendor/lib"} {
		write(filepath.Join(d, "AGENTS.md"), "IGNORED\n")
	}

	return root
}

// benchCommit turns root into a real repository with one staged change, so the
// git subprocess calls behave as they do in production.
func benchCommit(b *testing.B, root string) {
	b.Helper()

	run := func(args ...string) {
		b.Helper()

		out, err := exec.CommandContext(b.Context(), args[0], args[1:]...).CombinedOutput()
		if err != nil {
			b.Fatalf("%s %v: %v (%s)", args[0], args[1:], err, out)
		}
	}

	run("git", "-C", root, "init", "-q")
	run("git", "-C", root, "config", "user.email", "bench@example.com")
	run("git", "-C", root, "config", "user.name", "bench")
	run("git", "-C", root, "add", "-A")
	run("git", "-C", root, "commit", "-qm", "init")

	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("changed\n"), 0600); err != nil {
		b.Fatal(err)
	}

	run("git", "-C", root, "add", "README.md")
}

// BenchmarkResolveAgentFiles isolates nested AGENTS.md discovery.
func BenchmarkResolveAgentFiles(b *testing.B) {
	for _, nDirs := range []int{50, 300} {
		root := benchMonorepo(b, nDirs)
		b.Chdir(root)

		b.Run(fmt.Sprintf("dirs=%d", nDirs), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()

			for range b.N {
				// A fresh AppContext each pass: discovery is memoised per
				// invocation, so reusing one would measure the cache.
				app := &AppContext{}

				if files := app.AgentFiles(context.Background()); len(files) == 0 {
					b.Fatal("no files discovered")
				}
			}
		})
	}
}

// BenchmarkContextBuild isolates the prompt-context assembly that runs
// concurrently ahead of the model call.
func BenchmarkContextBuild(b *testing.B) {
	for _, nDirs := range []int{50, 300} {
		root := benchMonorepo(b, nDirs)
		benchCommit(b, root)
		b.Chdir(root)

		b.Run(fmt.Sprintf("dirs=%d", nDirs), func(b *testing.B) {
			app := &AppContext{}
			diff := "diff --git a/README.md b/README.md"

			b.ReportAllocs()
			b.ResetTimer()

			for range b.N {
				app.repoRoot, app.repoRootErr, app.repoRootOK = "", nil, false
				app.branch, app.branchOK = "", false
				app.operation, app.operationOK = git.OperationNone, false

				_ = buildPromptContextParallel(context.Background(), app, diff, git.OperationNone)
			}
		})
	}
}

// BenchmarkRepoStats isolates the extension-count walk that feeds repo context,
// the other full-tree traversal in the pre-model path.
func BenchmarkRepoStats(b *testing.B) {
	for _, nDirs := range []int{50, 300} {
		root := benchMonorepo(b, nDirs)

		b.Run(fmt.Sprintf("dirs=%d", nDirs), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()

			for range b.N {
				if _, err := detect.ComputeStatsWithContext(context.Background(), root); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkGitOps records the cost of each subprocess in the pre-model path,
// which is the floor the filesystem work sits on top of.
func BenchmarkGitOps(b *testing.B) {
	root := benchMonorepo(b, 300)
	benchCommit(b, root)
	b.Chdir(root)

	ctx := context.Background()

	b.Run("repoRoot", func(b *testing.B) {
		for range b.N {
			if _, err := git.GetRepoRoot(ctx); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("stagedDiff", func(b *testing.B) {
		for range b.N {
			if _, err := git.GetStagedChanges(ctx); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkRegenerateLoop measures what the review loop pays. Every regenerate
// re-runs generation; memoisation means discovery is paid once, not once per
// pass, so the cost of the loop is flat instead of linear in passes.
func BenchmarkRegenerateLoop(b *testing.B) {
	root := benchMonorepo(b, 300)
	b.Chdir(root)

	ctx := context.Background()

	b.Run("memoised", func(b *testing.B) {
		for range b.N {
			app := &AppContext{}
			for range 3 {
				if files := app.AgentFiles(ctx); len(files) == 0 {
					b.Fatal("no files")
				}
			}
		}
	})

	b.Run("unmemoised", func(b *testing.B) {
		for range b.N {
			app := &AppContext{}
			for range 3 {
				_ = resolveAgentFiles(ctx, app)
			}
		}
	})
}
