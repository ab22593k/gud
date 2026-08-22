package mem

import (
	"time"

	helix "github.com/helixdb/helix-db/sdks/go"
)

const (
	// DefaultTenantProperty is the canonical property name for tenant isolation.
	// DefaultBaseURL is the HelixDB server URL used when none is configured.
	DefaultBaseURL = "http://localhost:3223"

	DefaultTenantProperty = "tenantId"

	// DefaultEmbeddingDimension is the vector dimension used for embedding
	// indexes (text-embedding-3-small, 1536 dimensions).
	DefaultEmbeddingDimension = 1536
)

// FileChange represents a single file modified in a commit.
type FileChange struct {
	Path         string
	ChangeType   string
	LinesAdded   int
	LinesDeleted int
}

// CodeUnitRef represents a code element referenced by a commit.
type CodeUnitRef struct {
	Name       string
	Kind       string
	FilePath   string
	ChangeType string
}

// CommitData holds all data about a commit for persistence into HelixDB.
// tenantId is set to repoPath to isolate commits by repository.
type CommitData struct {
	SHA            string
	Message        string
	Author         string
	Timestamp      time.Time
	RepoPath       string
	Branch         string
	DiffHash       string
	DiffStat       string
	DiffText       string
	Files          []FileChange
	CodeUnits      []CodeUnitRef
	IsGudGenerated bool

	Embedding []float32
	IsLatest  bool
	DeletedAt *time.Time
	ExpiresAt *time.Time
}

// ToProps converts CommitData to a helix.Props for persisting as a Commit node.
func (c *CommitData) ToProps() helix.Props {
	props := helix.Props{
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
		helix.Prop(DefaultTenantProperty, helix.String(c.RepoPath)),
		helix.Prop("isLatest", helix.Bool(c.IsLatest)),
	}
	if len(c.Embedding) > 0 {
		props = append(props, helix.Prop("embedding", helix.F32Array(c.Embedding...)))
	}

	if c.DeletedAt != nil {
		props = append(props, helix.Prop("deletedAt", helix.DateTimeMillis(c.DeletedAt.UnixMilli())))
	}

	if c.ExpiresAt != nil {
		props = append(props, helix.Prop("expiresAt", helix.DateTimeMillis(c.ExpiresAt.UnixMilli())))
	}

	return props
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
	// Distance is the $distance from the search source (smaller = more
	// relevant). Zero when the result did not come from a scored search.
	Distance float64
}

// CommitRecordFromHelixData converts a HelixDB query result Node into a CommitRecord.
func CommitRecordFromHelixData(node Node) CommitRecord {
	return CommitRecord{
		HelixID:   node.Uint64("$id"),
		SHA:       firstNonEmpty(node.String("sha"), node.String("id")),
		Message:   node.String("message"),
		Author:    node.String("author"),
		Timestamp: time.UnixMilli(int64(node.Float64("timestamp"))),
		RepoPath:  node.String("repo_path"),
		Branch:    node.String("branch"),
		DiffStat:  node.String("diff_stat"),
		Distance:  node.Float64("distance"),
	}
}

// firstNonEmpty returns the first non-empty string from the given values.
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}

	return ""
}

// BuildCommitNodeQuery constructs a HelixDB traversal that fetches Commit nodes
// for a given repo and optional branch. Filters out soft-deleted records.
func BuildCommitNodeQuery(repoPath, branch string) *helix.Traversal {
	q := helix.G().NWithLabel("Commit").
		Has("repo_path", repoPath).
		Where(helix.PredIsNull("deletedAt"))
	if branch != "" {
		q = q.Has("branch", branch)
	}

	return q.ValueMap("$id", "id", "message", "author", "timestamp", "repo_path", "branch", "diff_stat")
}

// MemoryKind classifies a memory entry.
type MemoryKind string

const (
	MemoryFact       MemoryKind = "fact"
	MemoryPreference MemoryKind = "preference"
	MemoryEpisode    MemoryKind = "episode"
	MemoryProcedure  MemoryKind = "procedure"
)

// MemoryData represents a general memory node beyond commits.
type MemoryData struct {
	MemoryID  string
	Content   string
	TenantID  string
	UserID    string
	Kind      MemoryKind
	Salience  float64
	Embedding []float32

	IsLatest  bool
	ValidFrom *time.Time
	ValidTo   *time.Time
	DeletedAt *time.Time
	ExpiresAt *time.Time

	EventStartAt *time.Time
	EventEndAt   *time.Time
	TemporalText string
	Timezone     string

	CreatedAt time.Time
	UpdatedAt time.Time
}

// ToProps converts MemoryData to a helix.Props for persisting as a Memory node.
func (m *MemoryData) ToProps() helix.Props {
	props := helix.Props{
		helix.Prop("memoryId", helix.String(m.MemoryID)),
		helix.Prop("content", helix.String(m.Content)),
		helix.Prop(DefaultTenantProperty, helix.String(m.TenantID)),
		helix.Prop("userId", helix.String(m.UserID)),
		helix.Prop("kind", helix.String(string(m.Kind))),
		helix.Prop("salience", helix.F64(m.Salience)),
		helix.Prop("isLatest", helix.Bool(m.IsLatest)),
		helix.Prop("createdAt", helix.DateTimeMillis(m.CreatedAt.UnixMilli())),
		helix.Prop("updatedAt", helix.DateTimeMillis(m.UpdatedAt.UnixMilli())),
	}
	if len(m.Embedding) > 0 {
		props = append(props, helix.Prop("embedding", helix.F32Array(m.Embedding...)))
	}

	if m.ValidFrom != nil {
		props = append(props, helix.Prop("validFrom", helix.DateTimeMillis(m.ValidFrom.UnixMilli())))
	}

	if m.ValidTo != nil {
		props = append(props, helix.Prop("validTo", helix.DateTimeMillis(m.ValidTo.UnixMilli())))
	}

	if m.DeletedAt != nil {
		props = append(props, helix.Prop("deletedAt", helix.DateTimeMillis(m.DeletedAt.UnixMilli())))
	}

	if m.ExpiresAt != nil {
		props = append(props, helix.Prop("expiresAt", helix.DateTimeMillis(m.ExpiresAt.UnixMilli())))
	}

	if m.EventStartAt != nil {
		props = append(props, helix.Prop("eventStartAt", helix.DateTimeMillis(m.EventStartAt.UnixMilli())))
	}

	if m.EventEndAt != nil {
		props = append(props, helix.Prop("eventEndAt", helix.DateTimeMillis(m.EventEndAt.UnixMilli())))
	}

	if m.TemporalText != "" {
		props = append(props, helix.Prop("temporalText", helix.String(m.TemporalText)))
	}

	if m.Timezone != "" {
		props = append(props, helix.Prop("timezone", helix.String(m.Timezone)))
	}

	return props
}

// CategoryData represents a memory category scoped by tenant.
type CategoryData struct {
	CategoryKey string
	TenantID    string
	Name        string
	ParentKey   string
	Description string
}

// ToProps converts CategoryData to a helix.Props for persisting as a Category node.
func (c *CategoryData) ToProps() helix.Props {
	props := helix.Props{
		helix.Prop("categoryKey", helix.String(c.CategoryKey)),
		helix.Prop(DefaultTenantProperty, helix.String(c.TenantID)),
		helix.Prop("name", helix.String(c.Name)),
		helix.Prop("description", helix.String(c.Description)),
	}
	if c.ParentKey != "" {
		props = append(props, helix.Prop("parentKey", helix.String(c.ParentKey)))
	}

	return props
}

// EntityData represents a named entity (person, project, code unit, etc.) scoped by tenant.
type EntityData struct {
	EntityKey string
	TenantID  string
	Name      string
	Kind      string
	Metadata  map[string]string
}

// ToProps converts EntityData to a helix.Props for persisting as an Entity node.
func (e *EntityData) ToProps() helix.Props {
	props := helix.Props{
		helix.Prop("entityKey", helix.String(e.EntityKey)),
		helix.Prop(DefaultTenantProperty, helix.String(e.TenantID)),
		helix.Prop("name", helix.String(e.Name)),
		helix.Prop("kind", helix.String(e.Kind)),
		helix.Prop("metadata", helix.String("")),
	}

	return props
}
