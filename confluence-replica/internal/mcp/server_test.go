package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type fakeBackend struct{}

func (f fakeBackend) Search(_ context.Context, _ string, _ int, _ bool) ([]SearchResult, error) {
	return []SearchResult{
		{
			PageID:   "42",
			Version:  7,
			Title:    "Runbook",
			Snippet:  "critical path",
			Score:    0.91,
			PageURI:  "confluence://page/42",
			ChunkURI: "confluence://chunk/42:7:0",
		},
	}, nil
}

func (f fakeBackend) Ask(_ context.Context, _ string, _ int) (AskResult, error) {
	return AskResult{
		Answer:  "Only from local context",
		Refused: false,
		Citations: []Citation{
			{PageID: "42", Version: 7, Title: "Runbook", ChunkID: "42:7:0"},
		},
	}, nil
}

func (f fakeBackend) GetPageCurrent(_ context.Context, pageID string) (PageDoc, error) {
	return PageDoc{
		PageID:       pageID,
		Title:        "Runbook",
		CurrentVer:   7,
		UpdatedAt:    time.Unix(0, 0).UTC(),
		BodyRaw:      "<p>raw</p>",
		BodyNorm:     "raw",
		SourceURI:    "confluence://page/" + pageID,
		ParentPageID: "41",
		SpaceKey:     "OPS",
	}, nil
}

func (f fakeBackend) GetChunk(_ context.Context, chunkID string) (ChunkDoc, error) {
	return ChunkDoc{
		ChunkID:   chunkID,
		PageID:    "42",
		Version:   7,
		Title:     "Runbook",
		ChunkText: "critical path",
		SourceURI: "confluence://chunk/" + chunkID,
	}, nil
}

func (f fakeBackend) GetDigest(_ context.Context, date time.Time) (DigestDoc, error) {
	return DigestDoc{
		Date:      date.Format("2006-01-02"),
		Markdown:  "# digest",
		SourceURI: "confluence://digest/" + date.Format("2006-01-02"),
	}, nil
}

func (f fakeBackend) GetTree(_ context.Context, rootPageID string, depth int, limit int) ([]TreeNode, error) {
	return []TreeNode{
		{PageID: rootPageID, Title: "Root", Depth: 0, ParentPageID: "", CurrentVer: 1},
		{PageID: "child", Title: "Child", Depth: 1, ParentPageID: rootPageID, CurrentVer: 2},
	}, nil
}

func (f fakeBackend) GetChanges(_ context.Context, query ChangeQuery) ([]ChangeRecord, error) {
	_ = query
	return []ChangeRecord{
		{
			RunID:                  6,
			PageID:                 "42",
			Title:                  "Runbook",
			OldVersion:             7,
			NewVersion:             8,
			ChangeKind:             "updated",
			BodyRawChanged:         true,
			BodyNormChanged:        false,
			DiagramChangeDetected:  true,
			DiagramContentUnparsed: true,
			Summary:                "content updated",
			ExcerptBefore:          "revision=6",
			ExcerptAfter:           "revision=8",
			CreatedAt:              time.Unix(0, 0).UTC(),
		},
	}, nil
}

func (f fakeBackend) UpdatePage(_ context.Context, req UpdatePageRequest) (PageMutationResult, error) {
	title := "Runbook"
	if req.Title != nil {
		title = *req.Title
	}
	return PageMutationResult{
		PageID:  req.PageID,
		Title:   title,
		Version: 8,
	}, nil
}

func (f fakeBackend) CreateChildPage(_ context.Context, req CreateChildPageRequest) (PageMutationResult, error) {
	return PageMutationResult{
		PageID:       "100",
		Title:        req.Title,
		ParentPageID: req.ParentPageID,
		Version:      1,
	}, nil
}

func connectClient(t *testing.T, s *Server) *sdk.ClientSession {
	t.Helper()
	ctx := context.Background()
	client := sdk.NewClient(&sdk.Implementation{Name: "test-client", Version: "0.1.0"}, nil)
	t1, t2 := sdk.NewInMemoryTransports()
	if _, err := s.Connect(ctx, t1, nil); err != nil {
		t.Fatalf("connect server: %v", err)
	}
	cs, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	return cs
}

func TestSearchToolExposedAndCallable(t *testing.T) {
	s := NewServer(fakeBackend{})
	cs := connectClient(t, s)
	defer cs.Close()

	toolNames := map[string]bool{}
	for tool, err := range cs.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatalf("tools iterator error: %v", err)
		}
		toolNames[tool.Name] = true
	}
	for _, expected := range []string{"search", "ask", "get_tree", "what_changed", "update_page", "create_child_page"} {
		if !toolNames[expected] {
			t.Fatalf("expected tool %q to be listed", expected)
		}
	}

	resp, err := cs.CallTool(context.Background(), &sdk.CallToolParams{
		Name:      "search",
		Arguments: map[string]any{"query": "critical", "limit": 3},
	})
	if err != nil {
		t.Fatalf("call search: %v", err)
	}
	if len(resp.Content) == 0 {
		t.Fatalf("expected content in search response")
	}
	tc, ok := resp.Content[0].(*sdk.TextContent)
	if !ok {
		t.Fatalf("expected text content, got %T", resp.Content[0])
	}
	if !strings.Contains(tc.Text, "Runbook") {
		t.Fatalf("expected title in content: %s", tc.Text)
	}
	raw, _ := json.Marshal(resp.StructuredContent)
	if !strings.Contains(string(raw), "\"page_id\":\"42\"") {
		t.Fatalf("expected structured hit payload: %s", string(raw))
	}
}

func TestUpdatePageRejectsMissingPatchFields(t *testing.T) {
	s := NewServer(fakeBackend{})
	cs := connectClient(t, s)
	defer cs.Close()

	resp, err := cs.CallTool(context.Background(), &sdk.CallToolParams{
		Name:      "update_page",
		Arguments: map[string]any{"page_id": "42"},
	})
	if err != nil {
		t.Fatalf("call update_page: %v", err)
	}
	if !resp.IsError {
		t.Fatalf("expected MCP error result for missing patch fields")
	}
	if len(resp.Content) == 0 {
		t.Fatalf("expected error content for missing patch fields")
	}
	tc, ok := resp.Content[0].(*sdk.TextContent)
	if !ok {
		t.Fatalf("expected text content, got %T", resp.Content[0])
	}
	if !strings.Contains(tc.Text, "validation_error: at least one of title or body_storage is required") {
		t.Fatalf("unexpected error: %s", tc.Text)
	}
}

func TestUpdatePageRejectsMissingPageID(t *testing.T) {
	s := NewServer(fakeBackend{})
	cs := connectClient(t, s)
	defer cs.Close()

	resp, err := cs.CallTool(context.Background(), &sdk.CallToolParams{
		Name:      "update_page",
		Arguments: map[string]any{"title": "New Title"},
	})
	if err != nil {
		t.Fatalf("call update_page: %v", err)
	}
	if !resp.IsError {
		t.Fatalf("expected MCP error result for missing page_id")
	}
	if len(resp.Content) == 0 {
		t.Fatalf("expected error content for missing page_id")
	}
	tc, ok := resp.Content[0].(*sdk.TextContent)
	if !ok {
		t.Fatalf("expected text content, got %T", resp.Content[0])
	}
	if !strings.Contains(tc.Text, "missing properties") || !strings.Contains(tc.Text, "\"page_id\"") {
		t.Fatalf("unexpected error: %s", tc.Text)
	}
}

func TestUpdatePageRejectsEmptyPageID(t *testing.T) {
	s := NewServer(fakeBackend{})
	cs := connectClient(t, s)
	defer cs.Close()

	resp, err := cs.CallTool(context.Background(), &sdk.CallToolParams{
		Name: "update_page",
		Arguments: map[string]any{
			"page_id": "   ",
			"title":   "New Title",
		},
	})
	if err != nil {
		t.Fatalf("call update_page: %v", err)
	}
	if !resp.IsError {
		t.Fatalf("expected MCP error result for empty page_id")
	}
	if len(resp.Content) == 0 {
		t.Fatalf("expected error content for empty page_id")
	}
	tc, ok := resp.Content[0].(*sdk.TextContent)
	if !ok {
		t.Fatalf("expected text content, got %T", resp.Content[0])
	}
	if !strings.Contains(tc.Text, "validation_error: page_id is required") {
		t.Fatalf("unexpected error: %s", tc.Text)
	}
}

func TestUpdatePageRejectsPresentButEmptyTitle(t *testing.T) {
	s := NewServer(fakeBackend{})
	cs := connectClient(t, s)
	defer cs.Close()

	resp, err := cs.CallTool(context.Background(), &sdk.CallToolParams{
		Name: "update_page",
		Arguments: map[string]any{
			"page_id": "42",
			"title":   "   ",
		},
	})
	if err != nil {
		t.Fatalf("call update_page: %v", err)
	}
	if !resp.IsError {
		t.Fatalf("expected MCP error result for empty title")
	}
	if len(resp.Content) == 0 {
		t.Fatalf("expected error content for empty title")
	}
	tc, ok := resp.Content[0].(*sdk.TextContent)
	if !ok {
		t.Fatalf("expected text content, got %T", resp.Content[0])
	}
	if !strings.Contains(tc.Text, "validation_error: title cannot be empty when provided") {
		t.Fatalf("unexpected error: %s", tc.Text)
	}
}

func TestUpdatePageRejectsPresentButEmptyBodyStorage(t *testing.T) {
	s := NewServer(fakeBackend{})
	cs := connectClient(t, s)
	defer cs.Close()

	resp, err := cs.CallTool(context.Background(), &sdk.CallToolParams{
		Name: "update_page",
		Arguments: map[string]any{
			"page_id":      "42",
			"body_storage": "   ",
		},
	})
	if err != nil {
		t.Fatalf("call update_page: %v", err)
	}
	if !resp.IsError {
		t.Fatalf("expected MCP error result for empty body_storage")
	}
	if len(resp.Content) == 0 {
		t.Fatalf("expected error content for empty body_storage")
	}
	tc, ok := resp.Content[0].(*sdk.TextContent)
	if !ok {
		t.Fatalf("expected text content, got %T", resp.Content[0])
	}
	if !strings.Contains(tc.Text, "validation_error: body_storage cannot be empty when provided") {
		t.Fatalf("unexpected error: %s", tc.Text)
	}
}

func TestUpdatePageHappyPath(t *testing.T) {
	s := NewServer(fakeBackend{})
	cs := connectClient(t, s)
	defer cs.Close()

	resp, err := cs.CallTool(context.Background(), &sdk.CallToolParams{
		Name: "update_page",
		Arguments: map[string]any{
			"page_id":      " 42 ",
			"title":        "  New Title  ",
			"body_storage": "  <p>Updated body</p>  ",
		},
	})
	if err != nil {
		t.Fatalf("call update_page: %v", err)
	}
	if resp.IsError {
		t.Fatalf("expected successful update_page response")
	}
	raw, _ := json.Marshal(resp.StructuredContent)
	if !strings.Contains(string(raw), "\"page_id\":\"42\"") {
		t.Fatalf("expected page_id in structured payload: %s", string(raw))
	}
	if !strings.Contains(string(raw), "\"title\":\"New Title\"") {
		t.Fatalf("expected trimmed title in structured payload: %s", string(raw))
	}
}

func TestCreateChildPageRejectsMarkdownBodyStorage(t *testing.T) {
	s := NewServer(fakeBackend{})
	cs := connectClient(t, s)
	defer cs.Close()

	resp, err := cs.CallTool(context.Background(), &sdk.CallToolParams{
		Name: "create_child_page",
		Arguments: map[string]any{
			"parent_page_id": "42",
			"title":          "Child",
			"body_storage":   "# Heading",
		},
	})
	if err != nil {
		t.Fatalf("call create_child_page: %v", err)
	}
	if !resp.IsError {
		t.Fatalf("expected MCP error result for markdown body_storage")
	}
	if len(resp.Content) == 0 {
		t.Fatalf("expected error content for markdown body_storage")
	}
	tc, ok := resp.Content[0].(*sdk.TextContent)
	if !ok {
		t.Fatalf("expected text content, got %T", resp.Content[0])
	}
	if !strings.Contains(tc.Text, "validation_error: body_storage must be Confluence storage XHTML") {
		t.Fatalf("unexpected error: %s", tc.Text)
	}
}

func TestCreateChildPageRejectsPresentButEmptyBodyStorage(t *testing.T) {
	s := NewServer(fakeBackend{})
	cs := connectClient(t, s)
	defer cs.Close()

	resp, err := cs.CallTool(context.Background(), &sdk.CallToolParams{
		Name: "create_child_page",
		Arguments: map[string]any{
			"parent_page_id": "42",
			"title":          "Child",
			"body_storage":   "   ",
		},
	})
	if err != nil {
		t.Fatalf("call create_child_page: %v", err)
	}
	if !resp.IsError {
		t.Fatalf("expected MCP error result for empty body_storage")
	}
	if len(resp.Content) == 0 {
		t.Fatalf("expected error content for empty body_storage")
	}
	tc, ok := resp.Content[0].(*sdk.TextContent)
	if !ok {
		t.Fatalf("expected text content, got %T", resp.Content[0])
	}
	if !strings.Contains(tc.Text, "validation_error: body_storage cannot be empty after trimming") {
		t.Fatalf("unexpected error: %s", tc.Text)
	}
}

func TestCreateChildPageRejectsMissingParentPageID(t *testing.T) {
	s := NewServer(fakeBackend{})
	cs := connectClient(t, s)
	defer cs.Close()

	resp, err := cs.CallTool(context.Background(), &sdk.CallToolParams{
		Name: "create_child_page",
		Arguments: map[string]any{
			"title":        "Child",
			"body_storage": "<p>Child body</p>",
		},
	})
	if err != nil {
		t.Fatalf("call create_child_page: %v", err)
	}
	if !resp.IsError {
		t.Fatalf("expected MCP error result for missing parent_page_id")
	}
	if len(resp.Content) == 0 {
		t.Fatalf("expected error content for missing parent_page_id")
	}
	tc, ok := resp.Content[0].(*sdk.TextContent)
	if !ok {
		t.Fatalf("expected text content, got %T", resp.Content[0])
	}
	if !strings.Contains(tc.Text, "missing properties") || !strings.Contains(tc.Text, "\"parent_page_id\"") {
		t.Fatalf("unexpected error: %s", tc.Text)
	}
}

func TestCreateChildPageRejectsEmptyParentPageID(t *testing.T) {
	s := NewServer(fakeBackend{})
	cs := connectClient(t, s)
	defer cs.Close()

	resp, err := cs.CallTool(context.Background(), &sdk.CallToolParams{
		Name: "create_child_page",
		Arguments: map[string]any{
			"parent_page_id": "   ",
			"title":          "Child",
			"body_storage":   "<p>Child body</p>",
		},
	})
	if err != nil {
		t.Fatalf("call create_child_page: %v", err)
	}
	if !resp.IsError {
		t.Fatalf("expected MCP error result for empty parent_page_id")
	}
	if len(resp.Content) == 0 {
		t.Fatalf("expected error content for empty parent_page_id")
	}
	tc, ok := resp.Content[0].(*sdk.TextContent)
	if !ok {
		t.Fatalf("expected text content, got %T", resp.Content[0])
	}
	if !strings.Contains(tc.Text, "validation_error: parent_page_id is required") {
		t.Fatalf("unexpected error: %s", tc.Text)
	}
}

func TestCreateChildPageHappyPath(t *testing.T) {
	s := NewServer(fakeBackend{})
	cs := connectClient(t, s)
	defer cs.Close()

	resp, err := cs.CallTool(context.Background(), &sdk.CallToolParams{
		Name: "create_child_page",
		Arguments: map[string]any{
			"parent_page_id": " 42 ",
			"title":          "  Child page  ",
			"body_storage":   "  <p>Child body</p>  ",
		},
	})
	if err != nil {
		t.Fatalf("call create_child_page: %v", err)
	}
	if resp.IsError {
		t.Fatalf("expected successful create_child_page response")
	}
	raw, _ := json.Marshal(resp.StructuredContent)
	if !strings.Contains(string(raw), "\"page_id\":\"100\"") {
		t.Fatalf("expected page_id in structured payload: %s", string(raw))
	}
	if !strings.Contains(string(raw), "\"parent_page_id\":\"42\"") {
		t.Fatalf("expected trimmed parent_page_id in structured payload: %s", string(raw))
	}
	if !strings.Contains(string(raw), "\"title\":\"Child page\"") {
		t.Fatalf("expected trimmed title in structured payload: %s", string(raw))
	}
}

func TestWhatChangedTool(t *testing.T) {
	s := NewServer(fakeBackend{})
	cs := connectClient(t, s)
	defer cs.Close()

	resp, err := cs.CallTool(context.Background(), &sdk.CallToolParams{
		Name:      "what_changed",
		Arguments: map[string]any{"run_id": 6, "limit": 5},
	})
	if err != nil {
		t.Fatalf("call what_changed: %v", err)
	}
	if len(resp.Content) == 0 {
		t.Fatalf("expected content in what_changed response")
	}
	raw, _ := json.Marshal(resp.StructuredContent)
	if !strings.Contains(string(raw), "\"diagram_meta\"") {
		t.Fatalf("expected reasons in structured payload: %s", string(raw))
	}
}

func TestWhatChangedToolWithoutExcerpts(t *testing.T) {
	s := NewServer(fakeBackend{})
	cs := connectClient(t, s)
	defer cs.Close()

	resp, err := cs.CallTool(context.Background(), &sdk.CallToolParams{
		Name:      "what_changed",
		Arguments: map[string]any{"run_id": 6, "include_excerpts": false},
	})
	if err != nil {
		t.Fatalf("call what_changed: %v", err)
	}
	raw, _ := json.Marshal(resp.StructuredContent)
	if strings.Contains(string(raw), "revision=6") || strings.Contains(string(raw), "revision=8") {
		t.Fatalf("expected excerpts to be omitted: %s", string(raw))
	}
}

func TestWhatChangedToolRejectsBadDate(t *testing.T) {
	s := NewServer(fakeBackend{})
	cs := connectClient(t, s)
	defer cs.Close()

	resp, err := cs.CallTool(context.Background(), &sdk.CallToolParams{
		Name:      "what_changed",
		Arguments: map[string]any{"date": "2026/04/07"},
	})
	if err != nil {
		t.Fatalf("call what_changed: %v", err)
	}
	if !resp.IsError {
		t.Fatalf("expected MCP error result for invalid date")
	}
}

func TestReadPageResource(t *testing.T) {
	s := NewServer(fakeBackend{})
	cs := connectClient(t, s)
	defer cs.Close()

	resp, err := cs.ReadResource(context.Background(), &sdk.ReadResourceParams{
		URI: "confluence://page/42",
	})
	if err != nil {
		t.Fatalf("read resource: %v", err)
	}
	if len(resp.Contents) != 1 {
		t.Fatalf("expected one resource content item")
	}
	if !strings.Contains(resp.Contents[0].Text, "\"page_id\":\"42\"") {
		t.Fatalf("expected page payload: %s", resp.Contents[0].Text)
	}
}

func TestCompareVersionsPrompt(t *testing.T) {
	s := NewServer(fakeBackend{})
	cs := connectClient(t, s)
	defer cs.Close()

	resp, err := cs.GetPrompt(context.Background(), &sdk.GetPromptParams{
		Name: "compare_versions",
		Arguments: map[string]string{
			"page_id":      "42",
			"from_version": "3",
			"to_version":   "7",
		},
	})
	if err != nil {
		t.Fatalf("get prompt: %v", err)
	}
	if len(resp.Messages) == 0 {
		t.Fatalf("expected prompt message")
	}
	tc, ok := resp.Messages[0].Content.(*sdk.TextContent)
	if !ok {
		t.Fatalf("expected text prompt content, got %T", resp.Messages[0].Content)
	}
	if !strings.Contains(tc.Text, "from version 3 to version 7") {
		t.Fatalf("expected compare prompt text, got: %s", tc.Text)
	}
}
