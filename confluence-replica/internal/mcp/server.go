package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
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
}

func NewServer(backend Backend) *Server {
	return &Server{backend: backend}
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id,omitempty"`
	Result  any       `json:"result,omitempty"`
	Error   *rpcError `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type contentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type toolCallResult struct {
	Content           []contentItem `json:"content"`
	StructuredContent any           `json:"structuredContent,omitempty"`
	IsError           bool          `json:"isError,omitempty"`
}

type resourceReadResult struct {
	Contents []resourceContent `json:"contents"`
}

type resourceContent struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType"`
	Text     string `json:"text"`
}

type toolCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type resourceReadParams struct {
	URI string `json:"uri"`
}

type promptGetParams struct {
	Name      string            `json:"name"`
	Arguments map[string]string `json:"arguments"`
}

type promptGetResult struct {
	Description string          `json:"description"`
	Messages    []promptMessage `json:"messages"`
}

type promptMessage struct {
	Role    string        `json:"role"`
	Content promptContent `json:"content"`
}

type promptContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type uriKind int

const (
	uriUnknown uriKind = iota
	uriPage
	uriChunk
	uriDigest
)

func (s *Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	r := bufio.NewReader(in)
	for {
		msg, err := readFramedJSON(r)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}

		var req rpcRequest
		if err := json.Unmarshal(msg, &req); err != nil {
			continue
		}
		if req.Method == "notifications/initialized" {
			continue
		}
		if len(req.ID) == 0 {
			continue
		}
		resp := s.dispatch(ctx, req)
		if err := writeFramedJSON(out, resp); err != nil {
			return err
		}
	}
}

func (s *Server) dispatch(ctx context.Context, req rpcRequest) rpcResponse {
	id := decodeID(req.ID)
	switch req.Method {
	case "initialize":
		return rpcResponse{JSONRPC: "2.0", ID: id, Result: map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]any{
				"resources": map[string]any{},
				"tools":     map[string]any{},
				"prompts":   map[string]any{},
			},
			"serverInfo": map[string]any{
				"name":    "confluence-replica-mcp",
				"version": "0.1.0",
			},
		}}
	case "ping":
		return rpcResponse{JSONRPC: "2.0", ID: id, Result: map[string]any{}}
	case "tools/list":
		return rpcResponse{JSONRPC: "2.0", ID: id, Result: s.toolsList()}
	case "tools/call":
		var p toolCallParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: -32602, Message: "invalid tool params"}}
		}
		out, err := s.handleToolCall(ctx, p)
		if err != nil {
			return rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: -32000, Message: err.Error()}}
		}
		return rpcResponse{JSONRPC: "2.0", ID: id, Result: out}
	case "resources/list":
		return rpcResponse{JSONRPC: "2.0", ID: id, Result: s.resourcesList()}
	case "resources/read":
		var p resourceReadParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: -32602, Message: "invalid resource params"}}
		}
		out, err := s.handleResourceRead(ctx, p)
		if err != nil {
			return rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: -32000, Message: err.Error()}}
		}
		return rpcResponse{JSONRPC: "2.0", ID: id, Result: out}
	case "prompts/list":
		return rpcResponse{JSONRPC: "2.0", ID: id, Result: s.promptsList()}
	case "prompts/get":
		var p promptGetParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: -32602, Message: "invalid prompt params"}}
		}
		out, err := s.handlePromptGet(p)
		if err != nil {
			return rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: -32000, Message: err.Error()}}
		}
		return rpcResponse{JSONRPC: "2.0", ID: id, Result: out}
	default:
		return rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: -32601, Message: "method not found"}}
	}
}

func (s *Server) handleToolCall(ctx context.Context, p toolCallParams) (toolCallResult, error) {
	switch p.Name {
	case "search":
		query := stringArg(p.Arguments, "query", "")
		if strings.TrimSpace(query) == "" {
			return toolCallResult{}, errors.New("query is required")
		}
		limit := intArg(p.Arguments, "limit", 10)
		if limit <= 0 || limit > 50 {
			limit = 10
		}
		includeSnippets := boolArg(p.Arguments, "include_snippets", true)
		results, err := s.backend.Search(ctx, query, limit, includeSnippets)
		if err != nil {
			return toolCallResult{}, err
		}
		raw, _ := json.MarshalIndent(results, "", "  ")
		return toolCallResult{
			Content: []contentItem{{
				Type: "text",
				Text: string(raw),
			}},
			StructuredContent: map[string]any{"hits": results},
		}, nil
	case "ask":
		query := stringArg(p.Arguments, "query", "")
		if strings.TrimSpace(query) == "" {
			return toolCallResult{}, errors.New("query is required")
		}
		topK := intArg(p.Arguments, "top_k", 8)
		if topK <= 0 || topK > 20 {
			topK = 8
		}
		out, err := s.backend.Ask(ctx, query, topK)
		if err != nil {
			return toolCallResult{}, err
		}
		raw, _ := json.MarshalIndent(out, "", "  ")
		return toolCallResult{
			Content:           []contentItem{{Type: "text", Text: string(raw)}},
			StructuredContent: out,
		}, nil
	case "get_tree":
		rootPageID := stringArg(p.Arguments, "root_page_id", "")
		if rootPageID == "" {
			return toolCallResult{}, errors.New("root_page_id is required")
		}
		depth := intArg(p.Arguments, "depth", 2)
		if depth < 0 || depth > 8 {
			depth = 2
		}
		limit := intArg(p.Arguments, "limit", 200)
		if limit <= 0 || limit > 1000 {
			limit = 200
		}
		tree, err := s.backend.GetTree(ctx, rootPageID, depth, limit)
		if err != nil {
			return toolCallResult{}, err
		}
		raw, _ := json.MarshalIndent(tree, "", "  ")
		return toolCallResult{
			Content:           []contentItem{{Type: "text", Text: string(raw)}},
			StructuredContent: map[string]any{"nodes": tree},
		}, nil
	default:
		return toolCallResult{}, fmt.Errorf("unknown tool: %s", p.Name)
	}
}

func (s *Server) handleResourceRead(ctx context.Context, p resourceReadParams) (resourceReadResult, error) {
	kind, key := parseURI(p.URI)
	switch kind {
	case uriPage:
		doc, err := s.backend.GetPageCurrent(ctx, key)
		if err != nil {
			return resourceReadResult{}, err
		}
		raw, _ := json.Marshal(doc)
		return resourceReadResult{Contents: []resourceContent{{URI: p.URI, MimeType: "application/json", Text: string(raw)}}}, nil
	case uriChunk:
		doc, err := s.backend.GetChunk(ctx, key)
		if err != nil {
			return resourceReadResult{}, err
		}
		raw, _ := json.Marshal(doc)
		return resourceReadResult{Contents: []resourceContent{{URI: p.URI, MimeType: "application/json", Text: string(raw)}}}, nil
	case uriDigest:
		day, err := time.Parse("2006-01-02", key)
		if err != nil {
			return resourceReadResult{}, fmt.Errorf("invalid digest date: %s", key)
		}
		doc, err := s.backend.GetDigest(ctx, day)
		if err != nil {
			return resourceReadResult{}, err
		}
		raw, _ := json.Marshal(doc)
		return resourceReadResult{Contents: []resourceContent{{URI: p.URI, MimeType: "application/json", Text: string(raw)}}}, nil
	default:
		return resourceReadResult{}, fmt.Errorf("unsupported resource uri: %s", p.URI)
	}
}

func (s *Server) handlePromptGet(p promptGetParams) (promptGetResult, error) {
	switch p.Name {
	case "daily_brief":
		date := p.Arguments["date"]
		if date == "" {
			date = time.Now().Format("2006-01-02")
		}
		text := fmt.Sprintf("Use confluence://digest/%s and relevant pages/chunks to prepare a concise daily brief. Include what changed, why it matters, and explicit citations.", date)
		return promptResponse("Daily brief prompt for local replica", text), nil
	case "investigate_page":
		pageID := p.Arguments["page_id"]
		question := p.Arguments["question"]
		if pageID == "" || question == "" {
			return promptGetResult{}, errors.New("page_id and question are required")
		}
		text := fmt.Sprintf("Investigate page %s from local replica. Start with confluence://page/%s, then use search/get_tree if needed. Question: %s", pageID, pageID, question)
		return promptResponse("Investigate one page deeply with local citations", text), nil
	case "compare_versions":
		pageID := p.Arguments["page_id"]
		fromVersion := p.Arguments["from_version"]
		toVersion := p.Arguments["to_version"]
		if pageID == "" || fromVersion == "" || toVersion == "" {
			return promptGetResult{}, errors.New("page_id, from_version, to_version are required")
		}
		text := fmt.Sprintf("Compare page %s from version %s to version %s using local context only. Report meaningful changes and cite supporting chunks/pages.", pageID, fromVersion, toVersion)
		return promptResponse("Compare page versions using local replica", text), nil
	default:
		return promptGetResult{}, fmt.Errorf("unknown prompt: %s", p.Name)
	}
}

func promptResponse(description, text string) promptGetResult {
	return promptGetResult{
		Description: description,
		Messages: []promptMessage{
			{
				Role:    "user",
				Content: promptContent{Type: "text", Text: text},
			},
		},
	}
}

func (s *Server) toolsList() map[string]any {
	return map[string]any{
		"tools": []map[string]any{
			{
				"name":        "search",
				"description": "Hybrid search over local confluence replica index.",
				"inputSchema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"query":            map[string]any{"type": "string"},
						"limit":            map[string]any{"type": "integer", "default": 10},
						"include_snippets": map[string]any{"type": "boolean", "default": true},
					},
					"required": []string{"query"},
				},
			},
			{
				"name":        "ask",
				"description": "Deterministic synthesis from local search context with citations.",
				"inputSchema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"query": map[string]any{"type": "string"},
						"top_k": map[string]any{"type": "integer", "default": 8},
					},
					"required": []string{"query"},
				},
			},
			{
				"name":        "get_tree",
				"description": "Return a subtree rooted at root_page_id from local replica.",
				"inputSchema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"root_page_id": map[string]any{"type": "string"},
						"depth":        map[string]any{"type": "integer", "default": 2},
						"limit":        map[string]any{"type": "integer", "default": 200},
					},
					"required": []string{"root_page_id"},
				},
			},
		},
	}
}

func (s *Server) resourcesList() map[string]any {
	return map[string]any{
		"resources": []map[string]any{
			{
				"uri":         "confluence://page/{page_id}",
				"name":        "Confluence page (current version)",
				"description": "JSON document for a page from local replica",
				"mimeType":    "application/json",
			},
			{
				"uri":         "confluence://chunk/{chunk_id}",
				"name":        "Confluence chunk",
				"description": "JSON document for one indexed chunk",
				"mimeType":    "application/json",
			},
			{
				"uri":         "confluence://digest/{date}",
				"name":        "Daily digest",
				"description": "JSON digest object for YYYY-MM-DD",
				"mimeType":    "application/json",
			},
		},
	}
}

func (s *Server) promptsList() map[string]any {
	return map[string]any{
		"prompts": []map[string]any{
			{
				"name":        "daily_brief",
				"description": "Create a local-context daily brief from indexed digests/pages.",
				"arguments": []map[string]any{
					{"name": "date", "required": false, "description": "Digest date in YYYY-MM-DD."},
				},
			},
			{
				"name":        "investigate_page",
				"description": "Guide investigation of one page with local citations.",
				"arguments": []map[string]any{
					{"name": "page_id", "required": true, "description": "Confluence page ID."},
					{"name": "question", "required": true, "description": "Question to investigate."},
				},
			},
			{
				"name":        "compare_versions",
				"description": "Prompt to compare two versions of one page using local sources.",
				"arguments": []map[string]any{
					{"name": "page_id", "required": true, "description": "Confluence page ID."},
					{"name": "from_version", "required": true, "description": "Older version number."},
					{"name": "to_version", "required": true, "description": "Newer version number."},
				},
			},
		},
	}
}

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

func readFramedJSON(r *bufio.Reader) ([]byte, error) {
	contentLength := 0
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		l := strings.ToLower(line)
		if strings.HasPrefix(l, "content-length:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) != 2 {
				continue
			}
			n := strings.TrimSpace(parts[1])
			if v, err := strconv.Atoi(n); err == nil {
				contentLength = v
			}
		}
	}
	if contentLength <= 0 {
		return nil, errors.New("missing content-length")
	}
	buf := make([]byte, contentLength)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

func writeFramedJSON(w io.Writer, payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Content-Length: %d\r\n\r\n", len(b)); err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}

func decodeID(raw json.RawMessage) any {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil
	}
	return v
}

func stringArg(args map[string]any, key, fallback string) string {
	if args == nil {
		return fallback
	}
	v, ok := args[key]
	if !ok || v == nil {
		return fallback
	}
	s, ok := v.(string)
	if !ok {
		return fallback
	}
	return s
}

func intArg(args map[string]any, key string, fallback int) int {
	if args == nil {
		return fallback
	}
	v, ok := args[key]
	if !ok || v == nil {
		return fallback
	}
	switch n := v.(type) {
	case int:
		return n
	case int32:
		return int(n)
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return fallback
	}
}

func boolArg(args map[string]any, key string, fallback bool) bool {
	if args == nil {
		return fallback
	}
	v, ok := args[key]
	if !ok || v == nil {
		return fallback
	}
	b, ok := v.(bool)
	if !ok {
		return fallback
	}
	return b
}
