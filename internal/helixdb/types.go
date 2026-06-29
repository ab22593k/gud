package helixdb

import (
	"time"

	"github.com/helixdb/helix-db/sdks/go"
)

// FileChange represents a single file modified in a commit.
type FileChange struct {
	Path         string
	ChangeType   string // "added", "modified", "deleted"
	LinesAdded   int
	LinesDeleted int
}

// CodeUnitRef represents a code element (function, struct, etc.) referenced by a commit.
type CodeUnitRef struct {
	Name       string
	Kind       string
	FilePath   string
	ChangeType string
}

// CommitData holds all data about a commit for persistence into HelixDB.
type CommitData struct {
	SHA           string
	Message       string
	Author        string
	Timestamp     time.Time
	RepoPath      string
	Branch        string
	DiffHash      string
	DiffStat      string
	DiffText      string
	Files         []FileChange
	CodeUnits     []CodeUnitRef
	IsGudGenerated bool
}

// ToProps converts CommitData to a helix.Props for persisting as a Commit node.
func (c *CommitData) ToProps() helix.Props {
	return helix.Props{
		helix.Prop("id", helix.String(c.SHA)),
		helix.Prop("message", helix.String(c.Message)),
		helix.Prop("author", helix.String(c.Author)),
		helix.Prop("timestamp", helix.DateTimeMillis(c.Timestamp.UnixMilli())),
		helix.Prop("repo_path", helix.String(c.RepoPath)),
		helix.Prop("branch", helix.String(c.Branch)),
		helix.Prop("diff_hash", helix.String(c.DiffHash)),
		helix.Prop("diff_stat", helix.String(c.DiffStat)),
		helix.Prop("diff_text", helix.String(c.DiffText)),
		helix.Prop("is_gud", helix.Bool(c.IsGudGenerated)),
	}
}

// CommitRecord represents a commit retrieved from HelixDB.
type CommitRecord struct {
	HelixID   uint64
	SHA       string
	Message   string
	Author    string
	Timestamp time.Time
	RepoPath  string
	Branch    string
	DiffStat  string
}

// CommitRecordFromHelixData converts a HelixDB node data map into a CommitRecord.
func CommitRecordFromHelixData(data map[string]any) CommitRecord {
	r := CommitRecord{}
	if id, ok := data["$id"].(float64); ok {
		r.HelixID = uint64(id)
	}
	if sha, ok := data["id"].(string); ok {
		r.SHA = sha
	}
	if msg, ok := data["message"].(string); ok {
		r.Message = msg
	}
	if author, ok := data["author"].(string); ok {
		r.Author = author
	}
	if ts, ok := data["timestamp"].(float64); ok {
		r.Timestamp = time.UnixMilli(int64(ts))
	}
	if rp, ok := data["repo_path"].(string); ok {
		r.RepoPath = rp
	}
	if branch, ok := data["branch"].(string); ok {
		r.Branch = branch
	}
	if ds, ok := data["diff_stat"].(string); ok {
		r.DiffStat = ds
	}
	return r
}

// BuildCommitNodeQuery constructs a HelixDB traversal that fetches Commit nodes
// for a given repo and optional branch.
func BuildCommitNodeQuery(repoPath, branch string) *helix.Traversal {
	q := helix.G().NWithLabel("Commit").
		Has("repo_path", repoPath)
	if branch != "" {
		q = q.Has("branch", branch)
	}
	return q.ValueMap("$id", "id", "message", "author", "timestamp", "repo_path", "branch", "diff_stat")
}
