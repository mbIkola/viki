package store

import (
	"context"
	"encoding/json"
	"time"
)

type Page struct {
	PageID       string
	SpaceKey     string
	Title        string
	ParentPageID string
	CurrentVer   int
	UpdatedAt    time.Time
	CreatedAt    time.Time
	PathHash     string
	Tags         []string
	Status       string
}

type PageVersion struct {
	PageID     string
	Version    int
	AuthorID   string
	BodyRaw    string
	BodyNorm   string
	BodyHash   string
	Title      string
	ParentPage string
	FetchedAt  time.Time
}

type Chunk struct {
	PageID     string
	Version    int
	ChunkID    string
	ChunkText  string
	ChunkHash  string
	TokenCount int
	Embedding  []float32
}

type PageState struct {
	PageID       string
	Title        string
	ParentPageID string
	Version      int
	BodyNormHash string
}

type ChangeEvent struct {
	RunID      int64
	PageID     string
	Type       string
	OldVersion int
	NewVersion int
	OldParent  string
	NewParent  string
	OldTitle   string
	NewTitle   string
	Summary    string
}

type ChangeExcerpts struct {
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
	Source string `json:"source,omitempty"`
}

type PageChangeDiff struct {
	RunID                  int64
	PageID                 string
	Title                  string
	ParentPageID           string
	OldVersion             int
	NewVersion             int
	ChangeKind             string
	TitleChanged           bool
	ParentChanged          bool
	BodyRawChanged         bool
	BodyNormChanged        bool
	BodyHashOld            string
	BodyHashNew            string
	DiagramChangeDetected  bool
	DiagramContentUnparsed bool
	Summary                string
	Excerpts               ChangeExcerpts
	CreatedAt              time.Time
}

type PageChangeDiffQuery struct {
	Date         *time.Time
	RunID        int64
	ParentPageID string
	Limit        int
}

type LexicalSearchRow struct {
	PageID    string
	ChunkID   string
	Version   int
	Title     string
	Snippet   string
	Rank      int
	RankValue float64
}

type SemanticSearchRow struct {
	PageID         string
	ChunkID        string
	Version        int
	Title          string
	Snippet        string
	Rank           int
	Distance       float64
	EmbeddingModel string
}

type PageDocument struct {
	PageID       string
	SpaceKey     string
	Title        string
	ParentPageID string
	Status       string
	CurrentVer   int
	CreatedAt    time.Time
	UpdatedAt    time.Time
	Labels       []string
	Version      int
	BodyRaw      string
	BodyNorm     string
	BodyHash     string
	FetchedAt    time.Time
}

type ChunkDocument struct {
	ChunkID    string
	PageID     string
	Version    int
	Title      string
	ChunkText  string
	ChunkHash  string
	TokenCount int
}

type TreeNode struct {
	PageID       string
	Title        string
	ParentPageID string
	CurrentVer   int
	Depth        int
	UpdatedAt    time.Time
}

type Store interface {
	Close()
	BeginSyncRun(ctx context.Context, mode string) (int64, error)
	FinishSyncRun(ctx context.Context, runID int64, status string, stats map[string]any) error
	UpsertPageWithVersion(ctx context.Context, p Page, v PageVersion, chunks []Chunk) error
	ListCurrentStates(ctx context.Context) ([]PageState, error)
	InsertChangeEvents(ctx context.Context, events []ChangeEvent) error
	InsertPageChangeDiffs(ctx context.Context, diffs []PageChangeDiff) error
	SaveDigest(ctx context.Context, date time.Time, markdown string, stats map[string]any) error
	GetDigest(ctx context.Context, date time.Time) (string, error)
	ListChangeEventsForDate(ctx context.Context, date time.Time) ([]ChangeEvent, error)
	ListPageChangeDiffs(ctx context.Context, query PageChangeDiffQuery) ([]PageChangeDiff, error)
	SearchLexical(ctx context.Context, query string, limit int) ([]LexicalSearchRow, error)
	SearchSemantic(ctx context.Context, embedding []float32, limit int) ([]SemanticSearchRow, error)
	GetPageCurrent(ctx context.Context, pageID string) (PageDocument, error)
	GetPageVersion(ctx context.Context, pageID string, version int) (PageDocument, error)
	GetChunk(ctx context.Context, chunkID string) (ChunkDocument, error)
	GetTree(ctx context.Context, rootPageID string, depth int, limit int) ([]TreeNode, error)
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func zeroToNull(n int) any {
	if n == 0 {
		return nil
	}
	return n
}

func toJSON(v any) string {
	if v == nil {
		return "{}"
	}
	b, err := json.Marshal(v)
	if err != nil || len(b) == 0 {
		return "{}"
	}
	return string(b)
}
