package app

import (
	"context"
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"

	"confluence-replica/internal/confluence"
	"confluence-replica/internal/ingest"
	"confluence-replica/internal/rag"
	"confluence-replica/internal/search"
	"confluence-replica/internal/store"
)

type Config struct {
	Database struct {
		DSN string `yaml:"dsn"`
	} `yaml:"database"`
	Confluence struct {
		BaseURL         string `yaml:"base_url"`
		Token           string `yaml:"token"`
		RequestSec      int    `yaml:"request_timeout_seconds"`
		DefaultParentID string `yaml:"default_parent_id"`
	} `yaml:"confluence"`
	API struct {
		Addr string `yaml:"addr"`
	} `yaml:"api"`
	Embeddings struct {
		Provider   string `yaml:"provider"`
		BaseURL    string `yaml:"base_url"`
		Model      string `yaml:"model"`
		RequestSec int    `yaml:"request_timeout_seconds"`
	} `yaml:"embeddings"`
}

type Runtime struct {
	Config Config
	Store  *store.PostgresStore
	Ingest *ingest.Service
	Digest *ingest.DigestService
	Search *search.Service
	RAG    *rag.Engine
}

func LoadConfig(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return Config{}, err
	}
	if cfg.Database.DSN == "" {
		cfg.Database.DSN = os.Getenv("DATABASE_DSN")
	}
	if cfg.Confluence.Token == "" {
		cfg.Confluence.Token = os.Getenv("CONFLUENCE_PAT")
	}
	cfg.Confluence.Token, err = resolveSecretRef(cfg.Confluence.Token)
	if err != nil {
		return Config{}, fmt.Errorf("resolve confluence token: %w", err)
	}
	if cfg.Confluence.RequestSec <= 0 {
		cfg.Confluence.RequestSec = 30
	}
	if cfg.API.Addr == "" {
		cfg.API.Addr = ":8080"
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
	if cfg.Database.DSN == "" {
		return Config{}, fmt.Errorf("database dsn missing")
	}
	return cfg, nil
}

func NewRuntime(ctx context.Context, cfg Config) (*Runtime, error) {
	st, err := store.NewPostgresStore(ctx, cfg.Database.DSN)
	if err != nil {
		return nil, err
	}
	cl := confluence.NewClient(cfg.Confluence.BaseURL, cfg.Confluence.Token, time.Duration(cfg.Confluence.RequestSec)*time.Second)
	var emb search.Embedder = search.NoopEmbedder{}
	if cfg.Embeddings.Provider == "ollama" && cfg.Embeddings.Model != "" {
		emb = search.NewOllamaEmbedder(cfg.Embeddings.BaseURL, cfg.Embeddings.Model, time.Duration(cfg.Embeddings.RequestSec)*time.Second)
	}
	ing := ingest.NewService(cl, st, emb)
	digest := ingest.NewDigestService(st)
	searchService := search.NewService(st, emb)
	ragEngine := rag.NewEngine(searchService, rag.EchoLLM{})
	return &Runtime{Config: cfg, Store: st, Ingest: ing, Digest: digest, Search: searchService, RAG: ragEngine}, nil
}

func (r *Runtime) Close() {
	if r.Store != nil {
		r.Store.Close()
	}
}
