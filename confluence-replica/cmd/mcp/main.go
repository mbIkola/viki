package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"confluence-replica/internal/app"
	"confluence-replica/internal/confluence"
	"confluence-replica/internal/logx"
	mcpserver "confluence-replica/internal/mcp"
	"confluence-replica/internal/store"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type confluenceWriteClient interface {
	UpdatePage(ctx context.Context, pageID, title, bodyStorage, versionMessage string) (confluence.Page, error)
	CreateChildPage(ctx context.Context, parentPageID, title, bodyStorage, versionMessage string) (confluence.Page, error)
	GetPage(ctx context.Context, pageID string) (confluence.Page, error)
}

type runtimeBackend struct {
	rt           *app.Runtime
	client       confluenceWriteClient
	writeEnabled bool
	upsertPage   func(ctx context.Context, page confluence.Page) error
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
			Score:    h.FusionScore,
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

func (r runtimeBackend) GetChanges(ctx context.Context, query mcpserver.ChangeQuery) ([]mcpserver.ChangeRecord, error) {
	diffs, err := r.rt.Store.ListPageChangeDiffs(ctx, store.PageChangeDiffQuery{
		Date:         query.Date,
		RunID:        query.RunID,
		ParentPageID: query.ParentID,
		Limit:        query.Limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]mcpserver.ChangeRecord, 0, len(diffs))
	for _, d := range diffs {
		out = append(out, mcpserver.ChangeRecord{
			RunID:                  d.RunID,
			PageID:                 d.PageID,
			Title:                  d.Title,
			OldVersion:             d.OldVersion,
			NewVersion:             d.NewVersion,
			ChangeKind:             d.ChangeKind,
			TitleChanged:           d.TitleChanged,
			ParentChanged:          d.ParentChanged,
			BodyRawChanged:         d.BodyRawChanged,
			BodyNormChanged:        d.BodyNormChanged,
			DiagramChangeDetected:  d.DiagramChangeDetected,
			DiagramContentUnparsed: d.DiagramContentUnparsed,
			Summary:                d.Summary,
			ExcerptBefore:          d.Excerpts.Before,
			ExcerptAfter:           d.Excerpts.After,
			ExcerptSource:          d.Excerpts.Source,
			CreatedAt:              d.CreatedAt,
		})
	}
	return out, nil
}

func (r runtimeBackend) UpdatePage(ctx context.Context, req mcpserver.UpdatePageRequest) (mcpserver.PageMutationResult, error) {
	if err := r.guardWriteEnabled(); err != nil {
		return mcpserver.PageMutationResult{}, err
	}
	if r.client == nil {
		return mcpserver.PageMutationResult{}, errors.New("upstream_error: confluence client is not configured")
	}

	updated, err := r.client.UpdatePage(ctx, req.PageID, strptr(req.Title), strptr(req.BodyStorage), "")
	if err != nil {
		return mcpserver.PageMutationResult{}, classifyWriteError(err)
	}
	if err := r.upsertConfluencePage(ctx, updated); err != nil {
		return mcpserver.PageMutationResult{}, localRefreshFailedError(updated.ID, err)
	}
	return toMutationResult(updated), nil
}

func (r runtimeBackend) CreateChildPage(ctx context.Context, req mcpserver.CreateChildPageRequest) (mcpserver.PageMutationResult, error) {
	if err := r.guardWriteEnabled(); err != nil {
		return mcpserver.PageMutationResult{}, err
	}
	if r.client == nil {
		return mcpserver.PageMutationResult{}, errors.New("upstream_error: confluence client is not configured")
	}

	created, err := r.client.CreateChildPage(ctx, req.ParentPageID, req.Title, req.BodyStorage, "")
	if err != nil {
		return mcpserver.PageMutationResult{}, classifyWriteError(err)
	}

	if err := r.upsertConfluencePage(ctx, created); err != nil {
		return mcpserver.PageMutationResult{}, localRefreshFailedError(created.ID, err)
	}

	parent, err := r.client.GetPage(ctx, req.ParentPageID)
	if err != nil {
		return mcpserver.PageMutationResult{}, localRefreshFailedError(created.ID, fmt.Errorf("refresh parent page %s: %w", req.ParentPageID, err))
	}
	if err := r.upsertConfluencePage(ctx, parent); err != nil {
		return mcpserver.PageMutationResult{}, localRefreshFailedError(created.ID, fmt.Errorf("upsert parent page %s: %w", req.ParentPageID, err))
	}

	return toMutationResult(created), nil
}

func (r runtimeBackend) guardWriteEnabled() error {
	if !r.writeEnabled {
		return errors.New("write_disabled: MCP write operations are disabled")
	}
	return nil
}

func (r runtimeBackend) upsertConfluencePage(ctx context.Context, page confluence.Page) error {
	if r.upsertPage != nil {
		return r.upsertPage(ctx, page)
	}
	if r.rt == nil || r.rt.Ingest == nil {
		return errors.New("ingest service is not configured")
	}
	_, err := r.rt.Ingest.UpsertConfluencePage(ctx, page)
	return err
}

func toMutationResult(page confluence.Page) mcpserver.PageMutationResult {
	return mcpserver.PageMutationResult{
		PageID:       page.ID,
		Title:        page.Title,
		ParentPageID: parentID(page),
		Version:      page.Version.Number,
		SpaceKey:     page.Space.Key,
		PageURI:      fmt.Sprintf("confluence://page/%s", page.ID),
	}
}

func parentID(page confluence.Page) string {
	if len(page.Ancestors) == 0 {
		return ""
	}
	return page.Ancestors[len(page.Ancestors)-1].ID
}

func strptr(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func classifyWriteError(err error) error {
	if err == nil {
		return nil
	}

	var httpErr *confluence.HTTPError
	if errors.As(err, &httpErr) {
		switch httpErr.Status {
		case http.StatusUnauthorized, http.StatusForbidden:
			return fmt.Errorf("auth_error: %w", err)
		case http.StatusConflict:
			return fmt.Errorf("version_conflict: %w", err)
		default:
			return fmt.Errorf("upstream_error: %w", err)
		}
	}
	if confluence.IsVersionConflict(err) {
		return fmt.Errorf("version_conflict: %w", err)
	}
	return fmt.Errorf("upstream_error: %w", err)
}

func localRefreshFailedError(pageID string, err error) error {
	return fmt.Errorf("local_refresh_failed: remote_applied=true local_refreshed=false page_id=%s: %w", pageID, err)
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

	client := confluence.NewClient(cfg.Confluence.BaseURL, cfg.Confluence.Token, time.Duration(cfg.Confluence.RequestSec)*time.Second)
	srv := mcpserver.NewServer(runtimeBackend{
		rt:           rt,
		client:       client,
		writeEnabled: cfg.MCP.WriteEnabled,
	})
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
