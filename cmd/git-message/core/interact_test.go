package core

import (
	"testing"

	"gud/internal/git"
	"gud/internal/mem"
)

func TestToFileChanges(t *testing.T) {
	t.Parallel()

	const (
		mainFile  = "main.go"
		changeMod = "modified"
	)

	tests := []struct {
		name  string
		units []git.CodeUnit
		want  []mem.FileChange
	}{
		{
			name:  "empty input returns nil",
			units: nil,
			want:  nil,
		},
		{
			name: "single unit",
			units: []git.CodeUnit{
				{FilePath: mainFile, ChangeType: changeMod},
			},
			want: []mem.FileChange{
				{Path: mainFile, ChangeType: changeMod},
			},
		},
		{
			name: "duplicate file paths are deduplicated",
			units: []git.CodeUnit{
				{FilePath: mainFile, Name: "Run", ChangeType: changeMod},
				{FilePath: mainFile, Name: "Stop", ChangeType: changeMod},
			},
			want: []mem.FileChange{
				{Path: mainFile, ChangeType: changeMod},
			},
		},
		{
			name: "multiple unique files preserved",
			units: []git.CodeUnit{
				{FilePath: "a.go", ChangeType: "added"},
				{FilePath: "b.go", ChangeType: changeMod},
			},
			want: []mem.FileChange{
				{Path: "a.go", ChangeType: "added"},
				{Path: "b.go", ChangeType: changeMod},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := toFileChanges(tt.units)
			if len(got) != len(tt.want) {
				t.Fatalf("toFileChanges() returned %d items, want %d", len(got), len(tt.want))
			}
			for i := range got {
				if got[i].Path != tt.want[i].Path || got[i].ChangeType != tt.want[i].ChangeType {
					t.Errorf("toFileChanges()[%d] = {Path:%q, ChangeType:%q}, want {Path:%q, ChangeType:%q}",
						i, got[i].Path, got[i].ChangeType, tt.want[i].Path, tt.want[i].ChangeType)
				}
			}
		})
	}
}
