package mcp

import (
	"context"
	"strings"
	"testing"
	"time"
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

func TestHandleToolCallSearch(t *testing.T) {
	s := NewServer(fakeBackend{})
	out, err := s.handleToolCall(context.Background(), toolCallParams{
		Name:      "search",
		Arguments: map[string]any{"query": "critical", "limit": 3},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Content) == 0 {
		t.Fatalf("expected tool content")
	}
	if !strings.Contains(out.Content[0].Text, "Runbook") {
		t.Fatalf("expected title in tool output: %s", out.Content[0].Text)
	}
}

func TestHandleResourceReadPage(t *testing.T) {
	s := NewServer(fakeBackend{})
	out, err := s.handleResourceRead(context.Background(), resourceReadParams{
		URI: "confluence://page/42",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Contents) != 1 {
		t.Fatalf("expected one content item")
	}
	if !strings.Contains(out.Contents[0].Text, "\"page_id\":\"42\"") {
		t.Fatalf("expected page payload: %s", out.Contents[0].Text)
	}
}

func TestHandlePromptGetCompareVersions(t *testing.T) {
	s := NewServer(fakeBackend{})
	out, err := s.handlePromptGet(promptGetParams{
		Name: "compare_versions",
		Arguments: map[string]string{
			"page_id":      "42",
			"from_version": "3",
			"to_version":   "7",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Messages) == 0 {
		t.Fatalf("expected prompt message")
	}
	txt := out.Messages[0].Content.Text
	if !strings.Contains(txt, "from version 3 to version 7") {
		t.Fatalf("expected compare prompt text, got: %s", txt)
	}
}
