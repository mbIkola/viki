package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"confluence-replica/internal/app"
	"confluence-replica/internal/logx"
	mcpserver "confluence-replica/internal/mcp"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type runtimeBackend struct {
	rt *app.Runtime
}

func (r runtimeBackend) Search(ctx context.Context, query string, limit int, includeSnippets bool) ([]mcpserver.SearchResult, error) {
	hits, err := r.rt.Search.Query(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	out := make([]mcpserver.SearchResult, 0, len(hits))
	for _, h := range hits {
		snippet := h.Snippet
		if !includeSnippets {
			snippet = ""
		}
		out = append(out, mcpserver.SearchResult{
			PageID:   h.PageID,
			Version:  h.Version,
			Title:    h.Title,
			Snippet:  snippet,
			Score:    h.FinalScore,
			PageURI:  fmt.Sprintf("confluence://page/%s", h.PageID),
			ChunkURI: chunkURI(h.ChunkID),
		})
	}
	return out, nil
}

func (r runtimeBackend) Ask(ctx context.Context, query string, topK int) (mcpserver.AskResult, error) {
	resp, err := r.rt.RAG.AnswerWithTopK(ctx, query, topK)
	if err != nil {
		return mcpserver.AskResult{}, err
	}
	cites := make([]mcpserver.Citation, 0, len(resp.Citations))
	for _, c := range resp.Citations {
		cites = append(cites, mcpserver.Citation{
			PageID:  c.PageID,
			Version: c.Version,
			Title:   c.Title,
			ChunkID: c.ChunkID,
		})
	}
	return mcpserver.AskResult{
		Answer:    resp.Answer,
		Citations: cites,
		Refused:   resp.Refused,
	}, nil
}

func (r runtimeBackend) GetPageCurrent(ctx context.Context, pageID string) (mcpserver.PageDoc, error) {
	doc, err := r.rt.Store.GetPageCurrent(ctx, pageID)
	if err != nil {
		return mcpserver.PageDoc{}, err
	}
	return mcpserver.PageDoc{
		PageID:       doc.PageID,
		SpaceKey:     doc.SpaceKey,
		Title:        doc.Title,
		ParentPageID: doc.ParentPageID,
		Status:       doc.Status,
		CurrentVer:   doc.CurrentVer,
		UpdatedAt:    doc.UpdatedAt,
		CreatedAt:    doc.CreatedAt,
		Labels:       doc.Labels,
		Version:      doc.Version,
		BodyRaw:      doc.BodyRaw,
		BodyNorm:     doc.BodyNorm,
		BodyHash:     doc.BodyHash,
		FetchedAt:    doc.FetchedAt,
		SourceURI:    fmt.Sprintf("confluence://page/%s", doc.PageID),
	}, nil
}

func (r runtimeBackend) GetChunk(ctx context.Context, chunkID string) (mcpserver.ChunkDoc, error) {
	doc, err := r.rt.Store.GetChunk(ctx, chunkID)
	if err != nil {
		return mcpserver.ChunkDoc{}, err
	}
	return mcpserver.ChunkDoc{
		ChunkID:    doc.ChunkID,
		PageID:     doc.PageID,
		Version:    doc.Version,
		Title:      doc.Title,
		ChunkText:  doc.ChunkText,
		ChunkHash:  doc.ChunkHash,
		TokenCount: doc.TokenCount,
		SourceURI:  fmt.Sprintf("confluence://chunk/%s", doc.ChunkID),
	}, nil
}

func (r runtimeBackend) GetDigest(ctx context.Context, date time.Time) (mcpserver.DigestDoc, error) {
	md, err := r.rt.Store.GetDigest(ctx, date)
	if err != nil {
		return mcpserver.DigestDoc{}, err
	}
	dateText := date.Format("2006-01-02")
	return mcpserver.DigestDoc{
		Date:      dateText,
		Markdown:  md,
		SourceURI: fmt.Sprintf("confluence://digest/%s", dateText),
	}, nil
}

func (r runtimeBackend) GetTree(ctx context.Context, rootPageID string, depth int, limit int) ([]mcpserver.TreeNode, error) {
	nodes, err := r.rt.Store.GetTree(ctx, rootPageID, depth, limit)
	if err != nil {
		return nil, err
	}
	out := make([]mcpserver.TreeNode, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, mcpserver.TreeNode{
			PageID:       n.PageID,
			Title:        n.Title,
			ParentPageID: n.ParentPageID,
			CurrentVer:   n.CurrentVer,
			Depth:        n.Depth,
		})
	}
	return out, nil
}

func main() {
	defaultConfigPath := os.Getenv("CONF_REPLICA_CONFIG")
	if defaultConfigPath == "" {
		defaultConfigPath = "config/config.yaml"
	}

	fs := flag.NewFlagSet("mcp", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "path to config yaml")
	quiet := fs.Bool("quiet", false, "set log level to ERROR")
	verbose := fs.Bool("verbose", false, "set log level to DEBUG")
	_ = fs.Parse(os.Args[1:])

	cfg, err := app.LoadConfigWithOptions(*configPath, app.LoadOptions{RequireConfluenceToken: false})
	if err != nil {
		log.Fatal(err)
	}
	if err := logx.Configure(cfg.Logging.Level, *quiet, *verbose); err != nil {
		log.Fatal(err)
	}
	rt, err := app.NewRuntime(context.Background(), cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer rt.Close()

	srv := mcpserver.NewServer(runtimeBackend{rt: rt})
	if err := srv.Run(context.Background(), &sdk.StdioTransport{}); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}

func chunkURI(chunkID string) string {
	if chunkID == "" {
		return ""
	}
	return fmt.Sprintf("confluence://chunk/%s", chunkID)
}
