package git

import (
	"reflect"
	"testing"
)

func TestFilterRemovedContent(t *testing.T) {
	modifiedBlock := "diff --git a/keep.go b/keep.go\n" +
		"index 111..222 100644\n" +
		"--- a/keep.go\n" +
		"+++ b/keep.go\n" +
		"@@ -1 +1 @@\n" +
		"-a\n" +
		"+b\n"
	deletedBlock := "diff --git a/gone.go b/gone.go\n" +
		"deleted file mode 100644\n" +
		"index 333..000\n" +
		"--- a/gone.go\n" +
		"+++ /dev/null\n" +
		"-x\n"
	renameBlock := "diff --git a/old.txt b/new.txt\n" +
		"similarity index 100%\n" +
		"rename from old.txt\n" +
		"rename to new.txt\n"
	renameEditBlock := "diff --git a/a.txt b/b.txt\n" +
		"similarity index 80%\n" +
		"rename from a.txt\n" +
		"rename to b.txt\n" +
		"index 444..555 100644\n" +
		"--- a/a.txt\n" +
		"+++ b/b.txt\n" +
		"@@ -1 +1 @@\n" +
		"-a\n" +
		"+b\n"
	binaryDeleteBlock := "diff --git a/img.png b/img.png\n" +
		"deleted file mode 100644\n" +
		"index 666..000\n" +
		"Binary files a/img.png and /dev/null differ\n"

	tests := []struct {
		name        string
		diff        string
		include     bool
		wantKept    string
		wantDeleted []string
		wantRenamed []RenamedFile
	}{
		{
			name:        "empty diff",
			diff:        "",
			include:     false,
			wantKept:    "",
			wantDeleted: nil,
			wantRenamed: nil,
		},
		{
			name:        "no removed changes returned unchanged",
			diff:        modifiedBlock,
			include:     false,
			wantKept:    modifiedBlock,
			wantDeleted: nil,
			wantRenamed: nil,
		},
		{
			name:        "deleted file content dropped, name kept",
			diff:        modifiedBlock + deletedBlock,
			include:     false,
			wantKept:    modifiedBlock,
			wantDeleted: []string{"gone.go"},
			wantRenamed: nil,
		},
		{
			name:        "pure rename hunks dropped, pair kept",
			diff:        modifiedBlock + renameBlock,
			include:     false,
			wantKept:    modifiedBlock,
			wantDeleted: nil,
			wantRenamed: []RenamedFile{{OldPath: "old.txt", NewPath: "new.txt"}},
		},
		{
			name:        "rename with edits dropped as one block",
			diff:        modifiedBlock + renameEditBlock,
			include:     false,
			wantKept:    modifiedBlock,
			wantDeleted: nil,
			wantRenamed: []RenamedFile{{OldPath: "a.txt", NewPath: "b.txt"}},
		},
		{
			name:        "binary deletion dropped with name",
			diff:        modifiedBlock + binaryDeleteBlock,
			include:     false,
			wantKept:    modifiedBlock,
			wantDeleted: []string{"img.png"},
			wantRenamed: nil,
		},
		{
			name:        "deletion-only diff leaves empty kept with names",
			diff:        deletedBlock,
			include:     false,
			wantKept:    "",
			wantDeleted: []string{"gone.go"},
			wantRenamed: nil,
		},
		{
			name:        "headerless deletion fixture",
			diff:        "--- a/old.txt\n+++ /dev/null\n-deleted content\n",
			include:     false,
			wantKept:    "",
			wantDeleted: []string{"old.txt"},
			wantRenamed: nil,
		},
		{
			name:        "include keeps full diff and still reports refs",
			diff:        modifiedBlock + deletedBlock + renameBlock,
			include:     true,
			wantKept:    modifiedBlock + deletedBlock + renameBlock,
			wantDeleted: []string{"gone.go"},
			wantRenamed: []RenamedFile{{OldPath: "old.txt", NewPath: "new.txt"}},
		},
		{
			name:        "multiple removals keep diff order",
			diff:        deletedBlock + renameBlock + modifiedBlock,
			include:     false,
			wantKept:    modifiedBlock,
			wantDeleted: []string{"gone.go"},
			wantRenamed: []RenamedFile{{OldPath: "old.txt", NewPath: "new.txt"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotKept, gotDeleted, gotRenamed := FilterRemovedContent(tt.diff, tt.include)
			if gotKept != tt.wantKept {
				t.Errorf("FilterRemovedContent() kept:\n got: %q\nwant: %q", gotKept, tt.wantKept)
			}

			if !reflect.DeepEqual(gotDeleted, tt.wantDeleted) {
				t.Errorf("FilterRemovedContent() deleted = %v, want %v", gotDeleted, tt.wantDeleted)
			}

			if !reflect.DeepEqual(gotRenamed, tt.wantRenamed) {
				t.Errorf("FilterRemovedContent() renamed = %v, want %v", gotRenamed, tt.wantRenamed)
			}
		})
	}
}
