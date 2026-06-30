package helixdb

import (
	"fmt"
	"strings"

	"github.com/helixdb/helix-db/sdks/go"
)

// BuildPersistCommitQuery constructs a WriteBatch that atomically stores a
// commit and all its metadata in HelixDB, including CodeElement mentions
// with the enhanced memory model (tenantId, lifecycle, entity relationships).
func BuildPersistCommitQuery(data CommitData) helix.Request {
	b := helix.WriteQuery("persist_commit")

	// 1. Create the Commit node.
	b.VarAs("commit", helix.G().AddN("Commit", data.ToProps()))

	commitNode := helix.NodeVar("commit")

	// 2. Create/merge File nodes and MODIFIES edges.
	for _, f := range data.Files {
		fileVar := "file_" + safeVarName(f.Path)
		b.VarAs(fileVar,
			helix.G().AddN("File", helix.Props{
				helix.Prop("path", f.Path),
				helix.Prop("repo_path", data.RepoPath),
				helix.Prop(DefaultTenantProperty, helix.String(data.RepoPath)),
			}))
		b.VarAs("mod_"+fileVar,
			helix.G().N(commitNode).AddE("MODIFIES", helix.NodeVar(fileVar), helix.Props{
				helix.Prop("change_type", f.ChangeType),
				helix.Prop("lines_added", f.LinesAdded),
				helix.Prop("lines_deleted", f.LinesDeleted),
				helix.Prop("path", f.Path),
				helix.Prop(DefaultTenantProperty, helix.String(data.RepoPath)),
			}))
	}

	// 3. Create/merge CodeElement nodes and MENTIONS edges.
	for _, cu := range data.CodeUnits {
		elementKey := fmt.Sprintf("%s:%s:%s", data.RepoPath, cu.FilePath, cu.Name)
		elemVar := "elem_" + safeVarName(elementKey)
		b.VarAs(elemVar,
			helix.G().AddN("CodeElement", helix.Props{
				helix.Prop("elementKey", helix.String(elementKey)),
				helix.Prop("name", helix.String(cu.Name)),
				helix.Prop("kind", helix.String(cu.Kind)),
				helix.Prop("file_path", helix.String(cu.FilePath)),
				helix.Prop("signature", helix.String(fmt.Sprintf("%s %s", cu.Kind, cu.Name))),
				helix.Prop(DefaultTenantProperty, helix.String(data.RepoPath)),
			}))

		b.VarAs("mentions_"+elemVar,
			helix.G().N(commitNode).AddE("MENTIONS", helix.NodeVar(elemVar), helix.Props{
				helix.Prop("change_type", cu.ChangeType),
				helix.Prop(DefaultTenantProperty, helix.String(data.RepoPath)),
			}))
	}

	return b.Returning("commit")
}

// safeVarName converts a file path to a valid HelixDB variable name.
func safeVarName(path string) string {
	name := strings.ReplaceAll(path, "/", "_")
	name = strings.ReplaceAll(name, ".", "_")
	name = strings.ReplaceAll(name, "-", "_")
	name = strings.ReplaceAll(name, ":", "_")
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

// BuildPersistMemoryQuery constructs a WriteBatch that stores a Memory node
// with the full lifecycle, optional embedding, and ownership fields.
func BuildPersistMemoryQuery(data MemoryData) helix.Request {
	b := helix.WriteQuery("persist_memory")

	b.VarAs("memory",
		helix.G().AddN("Memory", data.ToProps()),
	)

	return b.Returning("memory")
}

// BuildCategorizeMemoryQuery links a Memory node to a Category node.
func BuildCategorizeMemoryQuery(memoryID, tenantID, categoryKey string) helix.Request {
	b := helix.WriteQuery("categorize_memory")

	memory := b.ParamString("memory_id", memoryID)
	tenant := b.ParamString("tenant_id", tenantID)
	catKey := b.ParamString("category_key", categoryKey)

	b.VarAs("memory",
		helix.G().NWithLabel("Memory").
			Where(helix.PredEq("memoryId", memory)).
			Where(helix.PredEq(DefaultTenantProperty, tenant)),
	)
	b.VarAs("category",
		helix.G().NWithLabel("Category").
			Where(helix.PredEq("categoryKey", catKey)).
			Where(helix.PredEq(DefaultTenantProperty, tenant)),
	)
	b.VarAs("in_category",
		helix.G().N(helix.NodeVar("memory")).AddE("IN_CATEGORY", helix.NodeVar("category"), helix.Props{
			helix.Prop(DefaultTenantProperty, helix.String(tenantID)),
		}),
	)

	return b.Returning("in_category")
}

// BuildMentionEntityQuery links a Memory node to an Entity node.
func BuildMentionEntityQuery(memoryID, tenantID, entityKey string) helix.Request {
	b := helix.WriteQuery("mention_entity")

	memory := b.ParamString("memory_id", memoryID)
	tenant := b.ParamString("tenant_id", tenantID)
	entKey := b.ParamString("entity_key", entityKey)

	b.VarAs("memory",
		helix.G().NWithLabel("Memory").
			Where(helix.PredEq("memoryId", memory)).
			Where(helix.PredEq(DefaultTenantProperty, tenant)),
	)
	b.VarAs("entity",
		helix.G().NWithLabel("Entity").
			Where(helix.PredEq("entityKey", entKey)).
			Where(helix.PredEq(DefaultTenantProperty, tenant)),
	)
	b.VarAs("mentions",
		helix.G().N(helix.NodeVar("memory")).AddE("MENTIONS", helix.NodeVar("entity"), helix.Props{
			helix.Prop(DefaultTenantProperty, helix.String(tenantID)),
		}),
	)

	return b.Returning("mentions")
}

// BuildUpdateMemoryQuery creates a new version of a Memory and links it with
// an UPDATES edge to the old version, marking the old as not latest.
func BuildUpdateMemoryQuery(oldMemoryID, tenantID string, newData MemoryData) helix.Request {
	b := helix.WriteQuery("update_memory")

	oldID := b.ParamString("old_memory_id", oldMemoryID)
	tenant := b.ParamString("tenant_id", tenantID)

	newData.IsLatest = true
	newData.TenantID = tenantID

	b.VarAs("old_memory",
		helix.G().NWithLabel("Memory").
			Where(helix.PredEq("memoryId", oldID)).
			Where(helix.PredEq(DefaultTenantProperty, tenant)),
	)
	b.VarAs("old_isLatest_false",
		helix.G().N(helix.NodeVar("old_memory")).SetProperty("isLatest", false),
	)
	b.VarAs("new_memory",
		helix.G().AddN("Memory", newData.ToProps()),
	)
	b.VarAs("updates_edge",
		helix.G().N(helix.NodeVar("new_memory")).AddE("UPDATES", helix.NodeVar("old_memory"), helix.Props{
			helix.Prop(DefaultTenantProperty, helix.String(tenantID)),
		}),
	)

	return b.Returning("new_memory")
}

// BuildSoftDeleteMemoryQuery sets deletedAt on a memory node.
func BuildSoftDeleteMemoryQuery(memoryID, tenantID string) helix.Request {
	b := helix.WriteQuery("soft_delete_memory")

	memory := b.ParamString("memory_id", memoryID)
	tenant := b.ParamString("tenant_id", tenantID)

	b.VarAs("memory",
		helix.G().NWithLabel("Memory").
			Where(helix.PredEq("memoryId", memory)).
			Where(helix.PredEq(DefaultTenantProperty, tenant)),
	)
	b.VarAs("deleted",
		helix.G().N(helix.NodeVar("memory")).
			SetProperty("deletedAt", helix.ExprDateTime()).
			SetProperty("isLatest", false),
	)

	return b.Returning("deleted")
}
