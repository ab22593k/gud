package wire

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// testSnap encodes one table value: missing keys mean absent (callers omit
// them), "link:" prefix means non-regular, anything else is content.
func testSnap(v string) snapFile {
	if after, ok := strings.CutPrefix(v, "link:"); ok {
		return snapFile{kind: kindOther, token: after}
	}

	return snapFile{kind: kindRegular, token: v}
}

// fileMaps encodes table maps into snapshots.
func fileMaps(base, local, newer map[string]string) (b, l, n map[string]snapFile) {
	encode := func(in map[string]string) map[string]snapFile {
		out := make(map[string]snapFile, len(in))

		for p, v := range in {
			out[p] = testSnap(v)
		}

		return out
	}

	return encode(base), encode(local), encode(newer)
}

func TestClassify(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		base          map[string]string
		local         map[string]string
		new           map[string]string
		wantTake      []string
		wantDel       []string
		wantConflicts []string
	}{
		{
			name:  "all clean",
			base:  map[string]string{"a": "1"},
			local: map[string]string{"a": "1"},
			new:   map[string]string{"a": "1"},
		},
		{
			name:     "upstream-only modify",
			base:     map[string]string{"a": "1"},
			local:    map[string]string{"a": "1"},
			new:      map[string]string{"a": "2"},
			wantTake: []string{"a"},
		},
		{
			name:  "local-only modify",
			base:  map[string]string{"a": "1"},
			local: map[string]string{"a": "mine"},
			new:   map[string]string{"a": "1"},
		},
		{
			name:  "convergent edits",
			base:  map[string]string{"a": "1"},
			local: map[string]string{"a": "2"},
			new:   map[string]string{"a": "2"},
		},
		{
			name:          "both differ",
			base:          map[string]string{"a": "1"},
			local:         map[string]string{"a": "mine"},
			new:           map[string]string{"a": "2"},
			wantConflicts: []string{"a"},
		},
		{
			name:     "upstream add",
			base:     map[string]string{},
			local:    map[string]string{},
			new:      map[string]string{"a": "2"},
			wantTake: []string{"a"},
		},
		{
			name:  "local add kept",
			base:  map[string]string{},
			local: map[string]string{"a": "mine"},
			new:   map[string]string{},
		},
		{
			name:          "add add differ",
			base:          map[string]string{},
			local:         map[string]string{"a": "mine"},
			new:           map[string]string{"a": "2"},
			wantConflicts: []string{"a"},
		},
		{
			name:    "clean upstream delete",
			base:    map[string]string{"a": "1"},
			local:   map[string]string{"a": "1"},
			new:     map[string]string{},
			wantDel: []string{"a"},
		},
		{
			name:          "upstream delete vs local modify",
			base:          map[string]string{"a": "1"},
			local:         map[string]string{"a": "mine"},
			new:           map[string]string{},
			wantConflicts: []string{"a"},
		},
		{
			name:  "local delete kept when upstream clean",
			base:  map[string]string{"a": "1"},
			local: map[string]string{},
			new:   map[string]string{"a": "1"},
		},
		{
			name:          "local delete vs upstream modify",
			base:          map[string]string{"a": "1"},
			local:         map[string]string{},
			new:           map[string]string{"a": "2"},
			wantConflicts: []string{"a"},
		},
		{
			name:  "upstream link added alone skipped",
			base:  map[string]string{},
			local: map[string]string{},
			new:   map[string]string{"a": "link:x"},
		},
		{
			name:          "local file vs upstream link",
			base:          map[string]string{},
			local:         map[string]string{"a": "mine"},
			new:           map[string]string{"a": "link:x"},
			wantConflicts: []string{"a"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b, l, n := fileMaps(tc.base, tc.local, tc.new)
			take, del, conflicts := classify(b, l, n)

			slices.Sort(take)
			slices.Sort(del)
			slices.Sort(conflicts)

			if !slices.Equal(take, tc.wantTake) {
				t.Fatalf("take = %v, want %v", take, tc.wantTake)
			}

			if !slices.Equal(del, tc.wantDel) {
				t.Fatalf("del = %v, want %v", del, tc.wantDel)
			}

			if !slices.Equal(conflicts, tc.wantConflicts) {
				t.Fatalf("conflicts = %v, want %v", conflicts, tc.wantConflicts)
			}
		})
	}
}

func TestApplyMerge(t *testing.T) {
	t.Parallel()

	staging := t.TempDir()
	writeFile(t, staging, "keep.txt", "local")
	writeFile(t, staging, "gone.txt", "old")
	writeFile(t, staging, "same.txt", "v1")

	newDir := t.TempDir()
	writeFile(t, newDir, "same.txt", "v2")
	writeFile(t, newDir, "added.txt", "new")

	if err := applyMerge(staging, newDir, []string{"same.txt", "added.txt"}, []string{"gone.txt"}); err != nil {
		t.Fatalf("applyMerge: %v", err)
	}

	for name, want := range map[string]string{"keep.txt": "local", "same.txt": "v2", "added.txt": "new"} {
		data, err := os.ReadFile(filepath.Join(staging, filepath.FromSlash(name)))
		if err != nil || string(data) != want {
			t.Fatalf("%s = %q, err = %v", name, data, err)
		}
	}

	if _, err := os.Stat(filepath.Join(staging, "gone.txt")); !os.IsNotExist(err) {
		t.Fatal("deleted path still present")
	}
}

func TestCloneCheckoutPreservesLinks(t *testing.T) {
	t.Parallel()

	src := t.TempDir()
	writeFile(t, src, "a.txt", "alpha")
	writeFile(t, src, "sub/b.txt", "beta")

	if err := os.Symlink("a.txt", filepath.Join(src, "link.txt")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	staging := filepath.Join(t.TempDir(), "staging")

	if err := cloneCheckout(src, staging); err != nil {
		t.Fatalf("cloneCheckout: %v", err)
	}

	info, err := os.Lstat(filepath.Join(staging, "link.txt"))
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("link not preserved: %v %v", info, err)
	}

	data, err := os.ReadFile(filepath.Join(staging, "sub", "b.txt"))
	if err != nil || string(data) != "beta" {
		t.Fatalf("nested file = %q, err = %v", data, err)
	}
}
