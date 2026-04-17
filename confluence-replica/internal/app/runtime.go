package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"confluence-replica/internal/confluence"
	"confluence-replica/internal/ingest"
	"confluence-replica/internal/logx"
	"confluence-replica/internal/rag"
	"confluence-replica/internal/search"
	"confluence-replica/internal/store"
)

const (
	defaultSchemaVersion          = 1
	defaultChunkingVersion        = "runes900-v1"
	defaultEmbeddingNormalization = "none"
)

type Config struct {
	Database struct {
		Path string `yaml:"path"`
	} `yaml:"database"`
	MCP struct {
		WriteEnabled bool `yaml:"write_enabled"`
	} `yaml:"mcp"`
	Confluence struct {
		BaseURL    string   `yaml:"base_url"`
		Token      string   `yaml:"token"`
		RequestSec int      `yaml:"request_timeout_seconds"`
		ParentIDs  []string `yaml:"parent_ids"`
	} `yaml:"confluence"`
	API struct {
		Addr string `yaml:"addr"`
	} `yaml:"api"`
	Logging struct {
		Level string `yaml:"level"`
	} `yaml:"logging"`
	Embeddings struct {
		Provider   string `yaml:"provider"`
		BaseURL    string `yaml:"base_url"`
		Model      string `yaml:"model"`
		RequestSec int    `yaml:"request_timeout_seconds"`
	} `yaml:"embeddings"`
}

type Runtime struct {
	Config Config
	Store  store.Store
	Ingest *ingest.Service
	Digest *ingest.DigestService
	Search *search.Service
	RAG    *rag.Engine
}

type LoadOptions struct {
	RequireConfluenceToken bool
	RequireParentIDs       bool
	ParentIDsOverride      []string
}

func LoadConfig(path string) (Config, error) {
	return LoadConfigWithOptions(path, LoadOptions{
		RequireConfluenceToken: true,
		RequireParentIDs:       true,
	})
}

func LoadConfigWithOptions(path string, opts LoadOptions) (Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return Config{}, err
	}
	if cfg.Database.Path == "" {
		cfg.Database.Path = os.Getenv("DATABASE_PATH")
	}
	if cfg.Confluence.Token == "" {
		cfg.Confluence.Token = os.Getenv("CONFLUENCE_PAT")
	}
	if cfg.MCP.WriteEnabled {
		cfg.Confluence.Token, err = resolveSecretRef(cfg.Confluence.Token)
		if err != nil {
			return Config{}, fmt.Errorf("resolve confluence token: %w", err)
		}
		if cfg.Confluence.Token == "" {
			return Config{}, fmt.Errorf("mcp.write_enabled requires confluence token")
		}
	} else if opts.RequireConfluenceToken {
		cfg.Confluence.Token, err = resolveSecretRef(cfg.Confluence.Token)
		if err != nil {
			return Config{}, fmt.Errorf("resolve confluence token: %w", err)
		}
	} else if strings.HasPrefix(cfg.Confluence.Token, "keychain://") {
		// Local-only runtimes (for MCP retrieval facade) should not require Confluence auth.
		cfg.Confluence.Token = ""
	}
	if cfg.Confluence.RequestSec <= 0 {
		cfg.Confluence.RequestSec = 30
	}
	cfg.Confluence.ParentIDs = normalizeParentIDs(cfg.Confluence.ParentIDs)
	if opts.RequireParentIDs {
		overrideParentIDs := normalizeParentIDs(opts.ParentIDsOverride)
		if len(cfg.Confluence.ParentIDs) == 0 && len(overrideParentIDs) == 0 {
			return Config{}, fmt.Errorf("confluence.parent_ids is required unless parent override is provided")
		}
	}
	if cfg.API.Addr == "" {
		cfg.API.Addr = ":8080"
	}
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = os.Getenv("LOG_LEVEL")
	}
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = "INFO"
	}
	if _, err := logx.ParseLevel(cfg.Logging.Level); err != nil {
		return Config{}, err
	}
	if cfg.Embeddings.Provider == "" {
		cfg.Embeddings.Provider = "ollama"
	}
	if cfg.Embeddings.BaseURL == "" {
		cfg.Embeddings.BaseURL = os.Getenv("OLLAMA_BASE_URL")
	}
	if cfg.Embeddings.Model == "" {
		cfg.Embeddings.Model = os.Getenv("OLLAMA_EMBED_MODEL")
	}
	if cfg.Embeddings.RequestSec <= 0 {
		cfg.Embeddings.RequestSec = 20
	}
	if cfg.Embeddings.BaseURL == "" {
		cfg.Embeddings.BaseURL = "http://127.0.0.1:11434"
	}
	if cfg.Database.Path == "" {
		cfg.Database.Path = defaultDatabasePath()
	}
	return cfg, nil
}

func NewRuntime(ctx context.Context, cfg Config) (*Runtime, error) {
	logx.Infof("[runtime] embeddings provider=%s base_url=%s model=%s", cfg.Embeddings.Provider, cfg.Embeddings.BaseURL, cfg.Embeddings.Model)
	cl := confluence.NewClient(cfg.Confluence.BaseURL, cfg.Confluence.Token, time.Duration(cfg.Confluence.RequestSec)*time.Second)
	var emb search.Embedder = search.NoopEmbedder{}
	if cfg.Embeddings.Provider == "ollama" && cfg.Embeddings.Model != "" {
		emb = search.NewOllamaEmbedder(cfg.Embeddings.BaseURL, cfg.Embeddings.Model, time.Duration(cfg.Embeddings.RequestSec)*time.Second)
	}
	profile, err := buildIndexProfile(ctx, cfg, emb)
	if err != nil {
		return nil, err
	}
	logx.Infof("[runtime] init database_path=%s", cfg.Database.Path)
	st, err := store.NewSQLiteStore(ctx, cfg.Database.Path, profile)
	if err != nil {
		return nil, err
	}
	ing := ingest.NewService(cl, st, emb)
	digest := ingest.NewDigestService(st)
	searchService := search.NewService(st, emb)
	ragEngine := rag.NewEngine(searchService, rag.DeterministicLLM{})
	return &Runtime{Config: cfg, Store: st, Ingest: ing, Digest: digest, Search: searchService, RAG: ragEngine}, nil
}

func (r *Runtime) Close() {
	if r.Store != nil {
		r.Store.Close()
	}
}

func defaultDatabasePath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".local", "viki", "confluence", "replica.db")
	}
	return filepath.Join(home, ".local", "viki", "confluence", "replica.db")
}

func buildIndexProfile(ctx context.Context, cfg Config, emb search.Embedder) (store.IndexProfile, error) {
	profile := store.IndexProfile{
		SchemaVersion:          defaultSchemaVersion,
		ChunkingVersion:        defaultChunkingVersion,
		EmbeddingNormalization: defaultEmbeddingNormalization,
	}

	if cfg.Embeddings.Provider == "noop" || cfg.Embeddings.Model == "" {
		profile.EmbeddingProvider = "noop"
		profile.EmbeddingDimension = 1
		return profile, nil
	}

	vector, err := emb.Embed(ctx, "confluence-replica index profile probe")
	if err != nil {
		return store.IndexProfile{}, fmt.Errorf("probe embedding dimension: %w", err)
	}
	if len(vector) == 0 {
		return store.IndexProfile{}, fmt.Errorf("probe embedding dimension: empty embedding")
	}
	profile.EmbeddingProvider = cfg.Embeddings.Provider
	profile.EmbeddingModel = cfg.Embeddings.Model
	profile.EmbeddingDimension = len(vector)
	return profile, nil
}

func normalizeParentIDs(ids []string) []string {
	out := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, raw := range ids {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
