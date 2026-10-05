package core

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gud/internal/wire"

	"github.com/spf13/cobra"
)

// coreFakeFetcher scripts wire.Fetcher for command tests.
type coreFakeFetcher struct {
	commits map[string]string
	files   map[string]map[string]string
}

func (f *coreFakeFetcher) Resolve(_ context.Context, source wire.SourceRef) (wire.Resolution, error) {
	sha, ok := f.commits[source.Ref]
	if !ok {
		return wire.Resolution{}, fmt.Errorf("ref %q: %w", source.Ref, wire.ErrUnknownRef)
	}

	return wire.Resolution{Commit: sha, Ref: source.Ref, Subpath: source.Subpath}, nil
}

func (f *coreFakeFetcher) Materialize(
	_ context.Context, _ wire.SourceRef, res wire.Resolution, dir string,
) (int, error) {
	files, ok := f.files[res.Commit]
	if !ok {
		return 0, fmt.Errorf("commit: %w", wire.ErrMissingPath)
	}

	for name, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(name))

		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return 0, err
		}

		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			return 0, err
		}
	}

	return len(files), nil
}

// testFetchCmd builds a fetch command wired to a fake backend.
func testFetchCmd(t *testing.T, fetcher wire.Fetcher, args ...string) (*cobra.Command, *bytes.Buffer) {
	t.Helper()

	cmd := &cobra.Command{Use: "git-wire"}

	addWireTargetFlags(cmd, fetchForceUsage)

	var out bytes.Buffer

	cmd.SetOut(&out)
	cmd.SetArgs(args)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return fetchWith(cmd, args, fetcher)
	}

	return cmd, &out
}

func TestFetchWithSuccess(t *testing.T) {
	fake := &coreFakeFetcher{
		commits: map[string]string{"19.0": strings.Repeat("a", 40)},
		files:   map[string]map[string]string{strings.Repeat("a", 40): {"a.txt": "alpha"}},
	}

	cmd, out := testFetchCmd(t, fake, "https://github.com/OCA/server-tools/tree/19.0/auto_backup", "-t", t.TempDir()+"/x")

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if !strings.Contains(out.String(), "Fetched github.com/OCA/server-tools@19.0:auto_backup") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestFetchWithBadURL(t *testing.T) {
	fake := &coreFakeFetcher{}

	cmd, _ := testFetchCmd(t, fake, "not-a-url")

	err := cmd.Execute()
	if !errors.Is(err, wire.ErrBadURL) {
		t.Fatalf("err = %v, want ErrBadURL", err)
	}

	if !strings.Contains(err.Error(), "expected shape") {
		t.Fatalf("err missing guidance: %v", err)
	}
}

func TestFetchWithDefaultTarget(t *testing.T) {
	t.Chdir(t.TempDir())

	fake := &coreFakeFetcher{
		commits: map[string]string{"19.0": strings.Repeat("a", 40)},
		files:   map[string]map[string]string{strings.Repeat("a", 40): {"a.txt": "alpha"}},
	}

	cmd, _ := testFetchCmd(t, fake, "https://github.com/OCA/server-tools/tree/19.0/auto_backup")

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
}

// testUpdateCmd builds an update command wired to a fake backend.
func testUpdateCmd(t *testing.T, fetcher wire.Fetcher, args ...string) (*cobra.Command, *bytes.Buffer) {
	t.Helper()

	cmd := &cobra.Command{Use: "update"}

	addWireTargetFlags(cmd, updateForceUsage)

	var out bytes.Buffer

	cmd.SetOut(&out)
	cmd.SetArgs(args)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return updateWith(cmd, args, fetcher)
	}

	return cmd, &out
}

// fetchCoreCheckout fetches the fake's commit into target for update tests.
func fetchCoreCheckout(t *testing.T, fake *coreFakeFetcher, target string) {
	t.Helper()

	_, err := wire.Fetch(context.Background(), wire.FetchOptions{Fetcher: fake}, wire.SourceRef{
		Host:      "github.com",
		Owner:     "OCA",
		Repo:      "server-tools",
		Ref:       "19.0",
		Subpath:   "auto_backup",
		SourceURL: "https://github.com/OCA/server-tools/tree/19.0/auto_backup",
	}, target)
	if err != nil {
		t.Fatalf("fixture fetch: %v", err)
	}
}

func coreSuccessFake() *coreFakeFetcher {
	return &coreFakeFetcher{
		commits: map[string]string{"19.0": strings.Repeat("a", 40)},
		files:   map[string]map[string]string{strings.Repeat("a", 40): {"a.txt": "alpha"}},
	}
}

func TestUpdateWithNoOp(t *testing.T) {
	fake := coreSuccessFake()
	target := t.TempDir() + "/co"
	fetchCoreCheckout(t, fake, target)

	cmd, out := testUpdateCmd(t, fake, "-t", target)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if !strings.Contains(out.String(), "Already up to date") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestUpdateWithUpstreamChange(t *testing.T) {
	fake := coreSuccessFake()
	target := t.TempDir() + "/co"
	fetchCoreCheckout(t, fake, target)

	fake.commits["19.0"] = strings.Repeat("b", 40)
	fake.files[strings.Repeat("b", 40)] = map[string]string{"a.txt": "alpha2"}

	cmd, out := testUpdateCmd(t, fake, target)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if !strings.Contains(out.String(), "Updated") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestUpdateWithNotACheckout(t *testing.T) {
	cmd, _ := testUpdateCmd(t, coreSuccessFake(), "-t", t.TempDir())

	if err := cmd.Execute(); !errors.Is(err, wire.ErrNotACheckout) {
		t.Fatalf("err = %v, want ErrNotACheckout", err)
	}
}

// testListCmd builds a list command wired to a fake backend.
func testListCmd(t *testing.T, fetcher wire.Fetcher, args ...string) (*cobra.Command, *bytes.Buffer) {
	t.Helper()

	cmd := &cobra.Command{Use: "list"}

	var out bytes.Buffer

	cmd.SetOut(&out)
	cmd.SetArgs(args)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return listWith(cmd, args, fetcher)
	}

	return cmd, &out
}

func TestListWithRows(t *testing.T) {
	fake := coreSuccessFake()
	root := t.TempDir()
	fetchCoreCheckout(t, fake, root+"/one")
	fetchCoreCheckout(t, fake, root+"/two")

	cmd, out := testListCmd(t, fake, root)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	for _, want := range []string{"one", "two", "github.com/OCA/server-tools@19.0:auto_backup", "current"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output missing %q:\n%s", want, out.String())
		}
	}
}

func TestListWithEmpty(t *testing.T) {
	cmd, out := testListCmd(t, coreSuccessFake(), t.TempDir())

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if !strings.Contains(out.String(), "No tracked folders") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestListWithUnreachable(t *testing.T) {
	fake := coreSuccessFake()
	root := t.TempDir()
	fetchCoreCheckout(t, fake, root+"/one")

	cmd, out := testListCmd(t, &unreachableFake{}, root)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unreachable must not fail: %v", err)
	}

	if !strings.Contains(out.String(), "unreachable") {
		t.Fatalf("output = %q", out.String())
	}
}

// unreachableFake fails every resolution like a dead network.
type unreachableFake struct {
	coreFakeFetcher
}

func (f *unreachableFake) Resolve(_ context.Context, _ wire.SourceRef) (wire.Resolution, error) {
	return wire.Resolution{}, fmt.Errorf("dial: %w", wire.ErrUpstream)
}

func TestUpdateWithMerge(t *testing.T) {
	fake := coreSuccessFake()
	target := t.TempDir() + "/co"
	fetchCoreCheckout(t, fake, target)

	fake.commits["19.0"] = strings.Repeat("b", 40)
	fake.files[strings.Repeat("b", 40)] = map[string]string{"a.txt": "alpha2"}

	if err := os.WriteFile(target+"/mine.txt", []byte("local"), 0o600); err != nil {
		t.Fatalf("local edit: %v", err)
	}

	cmd, out := testUpdateCmd(t, fake, "-t", target)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if !strings.Contains(out.String(), "Merged") || !strings.Contains(out.String(), "local files kept") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestUpdateWithConflict(t *testing.T) {
	fake := coreSuccessFake()
	target := t.TempDir() + "/co"
	fetchCoreCheckout(t, fake, target)

	fake.commits["19.0"] = strings.Repeat("b", 40)
	fake.files[strings.Repeat("b", 40)] = map[string]string{"a.txt": "alpha2"}

	if err := os.WriteFile(target+"/a.txt", []byte("local"), 0o600); err != nil {
		t.Fatalf("local edit: %v", err)
	}

	cmd, _ := testUpdateCmd(t, fake, "-t", target)

	err := cmd.Execute()
	if !errors.Is(err, wire.ErrDiverged) {
		t.Fatalf("err = %v, want ErrDiverged", err)
	}

	if !strings.Contains(err.Error(), "a.txt") {
		t.Fatalf("err names no paths: %v", err)
	}
}
