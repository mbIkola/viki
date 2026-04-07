package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

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

type Backend interface {
	Search(ctx context.Context, query string, limit int, includeSnippets bool) ([]SearchResult, error)
	Ask(ctx context.Context, query string, topK int) (AskResult, error)
	GetPageCurrent(ctx context.Context, pageID string) (PageDoc, error)
	GetChunk(ctx context.Context, chunkID string) (ChunkDoc, error)
	GetDigest(ctx context.Context, date time.Time) (DigestDoc, error)
	GetTree(ctx context.Context, rootPageID string, depth int, limit int) ([]TreeNode, error)
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

func NewServer(backend Backend) *Server {
	s := &Server{
		backend: backend,
		sdk: sdk.NewServer(&sdk.Implementation{
			Name:    "confluence-replica-mcp",
			Version: "0.2.0",
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
