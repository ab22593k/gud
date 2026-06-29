package helixdb

import (
	"fmt"
	"strings"

	"github.com/helixdb/helix-db/sdks/go"
)

// BuildPersistCommitQuery constructs a WriteBatch that atomically stores a
// commit and all its metadata in HelixDB.
func BuildPersistCommitQuery(data CommitData) helix.Request {
	b := helix.WriteQuery("persist_commit")

	// 1. Create the Commit node.
	b.VarAs("commit", helix.G().AddN("Commit", data.ToProps()))

	// 2. Create/merge File nodes and MODIFIES edges.
	for _, f := range data.Files {
		b.VarAs("file_"+safeVarName(f.Path),
			helix.G().AddN("File", helix.Props{
				helix.Prop("path", f.Path),
				helix.Prop("repo_path", data.RepoPath),
			}))
		b.VarAs("mod_"+safeVarName(f.Path),
			helix.G().AddE("MODIFIES", helix.NodeVar("commit"), helix.Props{
				helix.Prop("change_type", f.ChangeType),
				helix.Prop("lines_added", f.LinesAdded),
				helix.Prop("lines_deleted", f.LinesDeleted),
			}))
	}

	return b.Returning("commit")
}

// safeVarName converts a file path to a valid HelixDB variable name.
func safeVarName(path string) string {
	name := strings.ReplaceAll(path, "/", "_")
	name = strings.ReplaceAll(name, ".", "_")
	name = strings.ReplaceAll(name, "-", "_")
	return name
}

// FormatDiffStat creates a human-readable diff summary for the LLM prompt.
func FormatDiffStat(files []FileChange) string {
	if len(files) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("Files changed:\n")
	for _, f := range files {
		_, _ = fmt.Fprintf(&b, "  %s (%s: +%d/-%d)\n",
			f.Path, f.ChangeType, f.LinesAdded, f.LinesDeleted)
	}
	return b.String()
}
