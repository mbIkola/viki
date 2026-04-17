package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"confluence-replica/internal/version"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type SearchResult struct {
	PageID   string  `json:"page_id"`
	Version  int     `json:"version"`
	Title    string  `json:"title"`
	Snippet  string  `json:"snippet,omitempty"`
	Score    float64 `json:"score"`
	PageURI  string  `json:"page_uri"`
	ChunkURI string  `json:"chunk_uri,omitempty"`
}

type Citation struct {
	PageID  string `json:"page_id"`
	Version int    `json:"version"`
	Title   string `json:"title"`
	ChunkID string `json:"chunk_id,omitempty"`
}

type AskResult struct {
	Answer    string     `json:"answer"`
	Citations []Citation `json:"citations"`
	Refused   bool       `json:"refused"`
}

type PageDoc struct {
	PageID       string    `json:"page_id"`
	SpaceKey     string    `json:"space_key"`
	Title        string    `json:"title"`
	ParentPageID string    `json:"parent_page_id,omitempty"`
	Status       string    `json:"status"`
	CurrentVer   int       `json:"current_version"`
	UpdatedAt    time.Time `json:"updated_at"`
	CreatedAt    time.Time `json:"created_at"`
	Labels       []string  `json:"labels,omitempty"`
	Version      int       `json:"version"`
	BodyRaw      string    `json:"body_raw"`
	BodyNorm     string    `json:"body_norm"`
	BodyHash     string    `json:"body_hash"`
	FetchedAt    time.Time `json:"fetched_at"`
	SourceURI    string    `json:"source_uri"`
}

type ChunkDoc struct {
	ChunkID    string `json:"chunk_id"`
	PageID     string `json:"page_id"`
	Version    int    `json:"version"`
	Title      string `json:"title"`
	ChunkText  string `json:"chunk_text"`
	ChunkHash  string `json:"chunk_hash"`
	TokenCount int    `json:"token_count"`
	SourceURI  string `json:"source_uri"`
}

type DigestDoc struct {
	Date      string `json:"date"`
	Markdown  string `json:"markdown"`
	SourceURI string `json:"source_uri"`
}

type TreeNode struct {
	PageID       string `json:"page_id"`
	Title        string `json:"title"`
	ParentPageID string `json:"parent_page_id,omitempty"`
	CurrentVer   int    `json:"current_version"`
	Depth        int    `json:"depth"`
}

type ChangeQuery struct {
	Date     *time.Time
	RunID    int64
	ParentID string
	Limit    int
}

type UpdatePageRequest struct {
	PageID      string  `json:"page_id"`
	Title       *string `json:"title,omitempty"`
	BodyStorage *string `json:"body_storage,omitempty"`
}

type CreateChildPageRequest struct {
	ParentPageID string `json:"parent_page_id"`
	Title        string `json:"title"`
	BodyStorage  string `json:"body_storage"`
}

type PageMutationResult struct {
	PageID       string `json:"page_id"`
	Title        string `json:"title"`
	ParentPageID string `json:"parent_page_id,omitempty"`
	Version      int    `json:"version"`
	SpaceKey     string `json:"space_key,omitempty"`
	PageURI      string `json:"page_uri,omitempty"`
}

type ChangeRecord struct {
	RunID                  int64     `json:"run_id"`
	PageID                 string    `json:"page_id"`
	Title                  string    `json:"title"`
	OldVersion             int       `json:"old_version"`
	NewVersion             int       `json:"new_version"`
	ChangeKind             string    `json:"change_kind"`
	TitleChanged           bool      `json:"title_changed"`
	ParentChanged          bool      `json:"parent_changed"`
	BodyRawChanged         bool      `json:"body_raw_changed"`
	BodyNormChanged        bool      `json:"body_norm_changed"`
	DiagramChangeDetected  bool      `json:"diagram_change_detected"`
	DiagramContentUnparsed bool      `json:"diagram_content_unparsed"`
	Summary                string    `json:"summary"`
	ExcerptBefore          string    `json:"excerpt_before,omitempty"`
	ExcerptAfter           string    `json:"excerpt_after,omitempty"`
	ExcerptSource          string    `json:"excerpt_source,omitempty"`
	CreatedAt              time.Time `json:"created_at"`
}

type whatChangedInput struct {
	Date            string `json:"date,omitempty" jsonschema:"target date in YYYY-MM-DD. Defaults to today."`
	RunID           int64  `json:"run_id,omitempty" jsonschema:"optional sync run id"`
	ParentID        string `json:"parent_id,omitempty" jsonschema:"optional parent page id filter"`
	Limit           int    `json:"limit,omitempty" jsonschema:"max number of changes to return"`
	IncludeExcerpts *bool  `json:"include_excerpts,omitempty" jsonschema:"include before/after excerpts in response"`
}

type whatChangedOutput struct {
	Changes []whatChangedRow `json:"changes"`
}

type whatChangedRow struct {
	RunID                  int64     `json:"run_id"`
	PageID                 string    `json:"page_id"`
	Title                  string    `json:"title"`
	OldVersion             int       `json:"old_version"`
	NewVersion             int       `json:"new_version"`
	ChangeKind             string    `json:"change_kind"`
	Reasons                []string  `json:"reasons"`
	DiagramChangeDetected  bool      `json:"diagram_change_detected"`
	DiagramContentUnparsed bool      `json:"diagram_content_unparsed"`
	Summary                string    `json:"summary"`
	ExcerptBefore          string    `json:"excerpt_before,omitempty"`
	ExcerptAfter           string    `json:"excerpt_after,omitempty"`
	CreatedAt              time.Time `json:"created_at"`
}

type Backend interface {
	Search(ctx context.Context, query string, limit int, includeSnippets bool) ([]SearchResult, error)
	Ask(ctx context.Context, query string, topK int) (AskResult, error)
	GetPageCurrent(ctx context.Context, pageID string) (PageDoc, error)
	GetChunk(ctx context.Context, chunkID string) (ChunkDoc, error)
	GetDigest(ctx context.Context, date time.Time) (DigestDoc, error)
	GetTree(ctx context.Context, rootPageID string, depth int, limit int) ([]TreeNode, error)
	GetChanges(ctx context.Context, query ChangeQuery) ([]ChangeRecord, error)
	UpdatePage(ctx context.Context, req UpdatePageRequest) (PageMutationResult, error)
	CreateChildPage(ctx context.Context, req CreateChildPageRequest) (PageMutationResult, error)
}

type Server struct {
	backend Backend
	sdk     *sdk.Server
}

type searchInput struct {
	Query           string `json:"query" jsonschema:"search query text"`
	Limit           int    `json:"limit,omitempty" jsonschema:"maximum number of hits to return"`
	IncludeSnippets *bool  `json:"include_snippets,omitempty" jsonschema:"whether to include text snippets in hits"`
}

type searchOutput struct {
	Hits []SearchResult `json:"hits"`
}

type askInput struct {
	Query string `json:"query" jsonschema:"question over local replica context"`
	TopK  int    `json:"top_k,omitempty" jsonschema:"number of retrieval hits used for synthesis"`
}

type getTreeInput struct {
	RootPageID string `json:"root_page_id" jsonschema:"root page id"`
	Depth      int    `json:"depth,omitempty" jsonschema:"max traversal depth"`
	Limit      int    `json:"limit,omitempty" jsonschema:"max number of returned nodes"`
}

type getTreeOutput struct {
	Nodes []TreeNode `json:"nodes"`
}

type updatePageInput struct {
	PageID      string  `json:"page_id" jsonschema:"target page id"`
	Title       *string `json:"title,omitempty" jsonschema:"optional new title"`
	BodyStorage *string `json:"body_storage,omitempty" jsonschema:"optional Confluence storage XHTML body"`
}

type createChildPageInput struct {
	ParentPageID string `json:"parent_page_id" jsonschema:"parent page id"`
	Title        string `json:"title" jsonschema:"title for the new child page"`
	BodyStorage  string `json:"body_storage" jsonschema:"Confluence storage XHTML body for the new page"`
}

var (
	storageTagRE = regexp.MustCompile(`(?is)<\s*(?:ac:|ri:)?[a-z][a-z0-9:_-]*(?:\s+[^>]*)?>`)
)

func NewServer(backend Backend) *Server {
	s := &Server{
		backend: backend,
		sdk: sdk.NewServer(&sdk.Implementation{
			Name:    "confluence-replica-mcp",
			Version: version.MCPVersion(),
		}, nil),
	}
	s.registerTools()
	s.registerResources()
	s.registerPrompts()
	return s
}

func (s *Server) Run(ctx context.Context, transport sdk.Transport) error {
	return s.sdk.Run(ctx, transport)
}

func (s *Server) Connect(ctx context.Context, transport sdk.Transport, opts *sdk.ServerSessionOptions) (*sdk.ServerSession, error) {
	return s.sdk.Connect(ctx, transport, opts)
}

func (s *Server) registerTools() {
	sdk.AddTool(s.sdk, &sdk.Tool{
		Name:        "search",
		Description: "Hybrid search over local confluence replica index.",
	}, s.searchTool)

	sdk.AddTool(s.sdk, &sdk.Tool{
		Name:        "ask",
		Description: "Deterministic synthesis from local search context with citations.",
	}, s.askTool)

	sdk.AddTool(s.sdk, &sdk.Tool{
		Name:        "get_tree",
		Description: "Return a subtree rooted at root_page_id from local replica.",
	}, s.getTreeTool)

	sdk.AddTool(s.sdk, &sdk.Tool{
		Name:        "what_changed",
		Description: "Return structured history of what changed from local sync diffs.",
	}, s.whatChangedTool)

	sdk.AddTool(s.sdk, &sdk.Tool{
		Name:        "update_page",
		Description: "Update an existing Confluence page title and/or storage body.",
	}, s.updatePageTool)

	sdk.AddTool(s.sdk, &sdk.Tool{
		Name:        "create_child_page",
		Description: "Create a child page under a parent page using storage body format.",
	}, s.createChildPageTool)
}

func (s *Server) registerResources() {
	s.sdk.AddResourceTemplate(&sdk.ResourceTemplate{
		URITemplate: "confluence://page/{page_id}",
		Name:        "Confluence page (current version)",
		Description: "JSON document for a page from local replica",
		MIMEType:    "application/json",
	}, s.readResource)

	s.sdk.AddResourceTemplate(&sdk.ResourceTemplate{
		URITemplate: "confluence://chunk/{chunk_id}",
		Name:        "Confluence chunk",
		Description: "JSON document for one indexed chunk",
		MIMEType:    "application/json",
	}, s.readResource)

	s.sdk.AddResourceTemplate(&sdk.ResourceTemplate{
		URITemplate: "confluence://digest/{date}",
		Name:        "Daily digest",
		Description: "JSON digest object for YYYY-MM-DD",
		MIMEType:    "application/json",
	}, s.readResource)
}

func (s *Server) registerPrompts() {
	s.sdk.AddPrompt(&sdk.Prompt{
		Name:        "daily_brief",
		Description: "Create a local-context daily brief from indexed digests/pages.",
		Arguments: []*sdk.PromptArgument{
			{Name: "date", Description: "Digest date in YYYY-MM-DD."},
		},
	}, s.promptDailyBrief)

	s.sdk.AddPrompt(&sdk.Prompt{
		Name:        "investigate_page",
		Description: "Guide investigation of one page with local citations.",
		Arguments: []*sdk.PromptArgument{
			{Name: "page_id", Description: "Confluence page ID.", Required: true},
			{Name: "question", Description: "Question to investigate.", Required: true},
		},
	}, s.promptInvestigatePage)

	s.sdk.AddPrompt(&sdk.Prompt{
		Name:        "compare_versions",
		Description: "Prompt to compare two versions of one page using local sources.",
		Arguments: []*sdk.PromptArgument{
			{Name: "page_id", Description: "Confluence page ID.", Required: true},
			{Name: "from_version", Description: "Older version number.", Required: true},
			{Name: "to_version", Description: "Newer version number.", Required: true},
		},
	}, s.promptCompareVersions)
}

func (s *Server) searchTool(ctx context.Context, _ *sdk.CallToolRequest, in searchInput) (*sdk.CallToolResult, searchOutput, error) {
	query := strings.TrimSpace(in.Query)
	if query == "" {
		return nil, searchOutput{}, errors.New("query is required")
	}
	limit := in.Limit
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	includeSnippets := true
	if in.IncludeSnippets != nil {
		includeSnippets = *in.IncludeSnippets
	}
	hits, err := s.backend.Search(ctx, query, limit, includeSnippets)
	if err != nil {
		return nil, searchOutput{}, err
	}
	return nil, searchOutput{Hits: hits}, nil
}

func (s *Server) askTool(ctx context.Context, _ *sdk.CallToolRequest, in askInput) (*sdk.CallToolResult, AskResult, error) {
	query := strings.TrimSpace(in.Query)
	if query == "" {
		return nil, AskResult{}, errors.New("query is required")
	}
	topK := in.TopK
	if topK <= 0 || topK > 20 {
		topK = 8
	}
	out, err := s.backend.Ask(ctx, query, topK)
	if err != nil {
		return nil, AskResult{}, err
	}
	return nil, out, nil
}

func (s *Server) getTreeTool(ctx context.Context, _ *sdk.CallToolRequest, in getTreeInput) (*sdk.CallToolResult, getTreeOutput, error) {
	rootPageID := strings.TrimSpace(in.RootPageID)
	if rootPageID == "" {
		return nil, getTreeOutput{}, errors.New("root_page_id is required")
	}
	depth := in.Depth
	if depth < 0 || depth > 8 {
		depth = 2
	}
	limit := in.Limit
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	nodes, err := s.backend.GetTree(ctx, rootPageID, depth, limit)
	if err != nil {
		return nil, getTreeOutput{}, err
	}
	return nil, getTreeOutput{Nodes: nodes}, nil
}

func (s *Server) whatChangedTool(ctx context.Context, _ *sdk.CallToolRequest, in whatChangedInput) (*sdk.CallToolResult, whatChangedOutput, error) {
	limit := in.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	includeExcerpts := true
	if in.IncludeExcerpts != nil {
		includeExcerpts = *in.IncludeExcerpts
	}
	var day *time.Time
	if strings.TrimSpace(in.Date) != "" {
		parsed, err := time.Parse("2006-01-02", strings.TrimSpace(in.Date))
		if err != nil {
			return nil, whatChangedOutput{}, errors.New("date must be YYYY-MM-DD")
		}
		day = &parsed
	} else if in.RunID == 0 {
		d := time.Now().UTC()
		onlyDay := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC)
		day = &onlyDay
	}

	changes, err := s.backend.GetChanges(ctx, ChangeQuery{
		Date:     day,
		RunID:    in.RunID,
		ParentID: strings.TrimSpace(in.ParentID),
		Limit:    limit,
	})
	if err != nil {
		return nil, whatChangedOutput{}, err
	}
	out := make([]whatChangedRow, 0, len(changes))
	for _, c := range changes {
		row := whatChangedRow{
			RunID:                  c.RunID,
			PageID:                 c.PageID,
			Title:                  c.Title,
			OldVersion:             c.OldVersion,
			NewVersion:             c.NewVersion,
			ChangeKind:             c.ChangeKind,
			Reasons:                changeReasons(c),
			DiagramChangeDetected:  c.DiagramChangeDetected,
			DiagramContentUnparsed: c.DiagramContentUnparsed,
			Summary:                c.Summary,
			CreatedAt:              c.CreatedAt,
		}
		if includeExcerpts {
			row.ExcerptBefore = c.ExcerptBefore
			row.ExcerptAfter = c.ExcerptAfter
		}
		out = append(out, row)
	}
	return nil, whatChangedOutput{Changes: out}, nil
}

func (s *Server) updatePageTool(ctx context.Context, _ *sdk.CallToolRequest, in updatePageInput) (*sdk.CallToolResult, PageMutationResult, error) {
	pageID := strings.TrimSpace(in.PageID)
	if pageID == "" {
		return nil, PageMutationResult{}, errors.New("validation_error: page_id is required")
	}

	var title *string
	if in.Title != nil {
		trimmed := strings.TrimSpace(*in.Title)
		if trimmed == "" {
			return nil, PageMutationResult{}, errors.New("validation_error: title cannot be empty when provided")
		}
		title = &trimmed
	}

	var bodyStorage *string
	if in.BodyStorage != nil {
		trimmed := strings.TrimSpace(*in.BodyStorage)
		if trimmed == "" {
			return nil, PageMutationResult{}, errors.New("validation_error: body_storage cannot be empty when provided")
		}
		if err := validateStorageBody(trimmed); err != nil {
			return nil, PageMutationResult{}, err
		}
		bodyStorage = &trimmed
	}

	if title == nil && bodyStorage == nil {
		return nil, PageMutationResult{}, errors.New("validation_error: at least one of title or body_storage is required")
	}

	out, err := s.backend.UpdatePage(ctx, UpdatePageRequest{
		PageID:      pageID,
		Title:       title,
		BodyStorage: bodyStorage,
	})
	if err != nil {
		return nil, PageMutationResult{}, err
	}
	return nil, out, nil
}

func (s *Server) createChildPageTool(ctx context.Context, _ *sdk.CallToolRequest, in createChildPageInput) (*sdk.CallToolResult, PageMutationResult, error) {
	parentPageID := strings.TrimSpace(in.ParentPageID)
	if parentPageID == "" {
		return nil, PageMutationResult{}, errors.New("validation_error: parent_page_id is required")
	}
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return nil, PageMutationResult{}, errors.New("validation_error: title cannot be empty after trimming")
	}
	bodyStorage := strings.TrimSpace(in.BodyStorage)
	if bodyStorage == "" {
		return nil, PageMutationResult{}, errors.New("validation_error: body_storage cannot be empty after trimming")
	}
	if err := validateStorageBody(bodyStorage); err != nil {
		return nil, PageMutationResult{}, err
	}

	out, err := s.backend.CreateChildPage(ctx, CreateChildPageRequest{
		ParentPageID: parentPageID,
		Title:        title,
		BodyStorage:  bodyStorage,
	})
	if err != nil {
		return nil, PageMutationResult{}, err
	}
	return nil, out, nil
}

func validateStorageBody(body string) error {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return errors.New("validation_error: body_storage is required")
	}
	// Heuristic check only: require XHTML-like storage content with visible tags.
	// We intentionally avoid strict parsing and avoid aggressive markdown detection.
	if !storageTagRE.MatchString(trimmed) {
		return errors.New("validation_error: body_storage must be Confluence storage XHTML (expected XML-like tags such as <p>...</p>)")
	}
	return nil
}

func changeReasons(c ChangeRecord) []string {
	reasons := make([]string, 0, 5)
	if c.BodyRawChanged {
		reasons = append(reasons, "content_raw")
	}
	if c.BodyNormChanged {
		reasons = append(reasons, "content_norm")
	}
	if c.TitleChanged {
		reasons = append(reasons, "title")
	}
	if c.ParentChanged {
		reasons = append(reasons, "parent")
	}
	if c.DiagramChangeDetected {
		reasons = append(reasons, "diagram_meta")
	}
	if len(reasons) == 0 {
		reasons = append(reasons, "metadata")
	}
	return reasons
}

func (s *Server) readResource(ctx context.Context, req *sdk.ReadResourceRequest) (*sdk.ReadResourceResult, error) {
	uri := req.Params.URI
	kind, key := parseURI(uri)
	switch kind {
	case uriPage:
		doc, err := s.backend.GetPageCurrent(ctx, key)
		if err != nil {
			return nil, err
		}
		return toResourceResult(uri, doc)
	case uriChunk:
		doc, err := s.backend.GetChunk(ctx, key)
		if err != nil {
			return nil, err
		}
		return toResourceResult(uri, doc)
	case uriDigest:
		day, err := time.Parse("2006-01-02", key)
		if err != nil {
			return nil, sdk.ResourceNotFoundError(uri)
		}
		doc, err := s.backend.GetDigest(ctx, day)
		if err != nil {
			return nil, err
		}
		return toResourceResult(uri, doc)
	default:
		return nil, sdk.ResourceNotFoundError(uri)
	}
}

func (s *Server) promptDailyBrief(_ context.Context, req *sdk.GetPromptRequest) (*sdk.GetPromptResult, error) {
	date := req.Params.Arguments["date"]
	if strings.TrimSpace(date) == "" {
		date = time.Now().Format("2006-01-02")
	}
	text := fmt.Sprintf("Use confluence://digest/%s and relevant pages/chunks to prepare a concise daily brief. Include what changed, why it matters, and explicit citations.", date)
	return promptResponse("Daily brief prompt for local replica", text), nil
}

func (s *Server) promptInvestigatePage(_ context.Context, req *sdk.GetPromptRequest) (*sdk.GetPromptResult, error) {
	pageID := strings.TrimSpace(req.Params.Arguments["page_id"])
	question := strings.TrimSpace(req.Params.Arguments["question"])
	if pageID == "" || question == "" {
		return nil, errors.New("page_id and question are required")
	}
	text := fmt.Sprintf("Investigate page %s from local replica. Start with confluence://page/%s, then use search/get_tree if needed. Question: %s", pageID, pageID, question)
	return promptResponse("Investigate one page deeply with local citations", text), nil
}

func (s *Server) promptCompareVersions(_ context.Context, req *sdk.GetPromptRequest) (*sdk.GetPromptResult, error) {
	pageID := strings.TrimSpace(req.Params.Arguments["page_id"])
	fromVersion := strings.TrimSpace(req.Params.Arguments["from_version"])
	toVersion := strings.TrimSpace(req.Params.Arguments["to_version"])
	if pageID == "" || fromVersion == "" || toVersion == "" {
		return nil, errors.New("page_id, from_version, to_version are required")
	}
	text := fmt.Sprintf("Compare page %s from version %s to version %s using local context only. Report meaningful changes and cite supporting chunks/pages.", pageID, fromVersion, toVersion)
	return promptResponse("Compare page versions using local replica", text), nil
}

func promptResponse(description, text string) *sdk.GetPromptResult {
	return &sdk.GetPromptResult{
		Description: description,
		Messages: []*sdk.PromptMessage{
			{
				Role:    "user",
				Content: &sdk.TextContent{Text: text},
			},
		},
	}
}

func toResourceResult(uri string, payload any) (*sdk.ReadResourceResult, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return &sdk.ReadResourceResult{
		Contents: []*sdk.ResourceContents{
			{
				URI:      uri,
				MIMEType: "application/json",
				Text:     string(raw),
			},
		},
	}, nil
}

type uriKind int

const (
	uriUnknown uriKind = iota
	uriPage
	uriChunk
	uriDigest
)

func parseURI(uri string) (uriKind, string) {
	const prefix = "confluence://"
	if !strings.HasPrefix(uri, prefix) {
		return uriUnknown, ""
	}
	rest := strings.TrimPrefix(uri, prefix)
	switch {
	case strings.HasPrefix(rest, "page/"):
		return uriPage, strings.TrimPrefix(rest, "page/")
	case strings.HasPrefix(rest, "chunk/"):
		return uriChunk, strings.TrimPrefix(rest, "chunk/")
	case strings.HasPrefix(rest, "digest/"):
		return uriDigest, strings.TrimPrefix(rest, "digest/")
	default:
		return uriUnknown, ""
	}
}
