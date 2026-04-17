package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"confluence-replica/internal/search"
)

func TestLoadConfigWithOptionsRequiresParentIDsWithoutOverride(t *testing.T) {
	cfgPath := writeTestConfig(t, `
database:
  path: "/tmp/replica.db"
confluence:
  base_url: "https://example.atlassian.net"
  token: "plain-token"
`)
	_, err := LoadConfigWithOptions(cfgPath, LoadOptions{
		RequireConfluenceToken: false,
		RequireParentIDs:       true,
	})
	if err == nil || !strings.Contains(err.Error(), "confluence.parent_ids") {
		t.Fatalf("expected missing parent_ids error, got: %v", err)
	}
}

func TestLoadConfigWithOptionsAllowsMissingParentIDsWithOverride(t *testing.T) {
	cfgPath := writeTestConfig(t, `
database:
  path: "/tmp/replica.db"
confluence:
  base_url: "https://example.atlassian.net"
  token: "plain-token"
`)
	cfg, err := LoadConfigWithOptions(cfgPath, LoadOptions{
		RequireConfluenceToken: false,
		RequireParentIDs:       true,
		ParentIDsOverride:      []string{" 42 "},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Confluence.ParentIDs) != 0 {
		t.Fatalf("expected config parent_ids to stay empty, got: %v", cfg.Confluence.ParentIDs)
	}
}

func TestLoadConfigWithOptionsNormalizesParentIDs(t *testing.T) {
	cfgPath := writeTestConfig(t, `
database:
  path: "/tmp/replica.db"
confluence:
  base_url: "https://example.atlassian.net"
  token: "plain-token"
  parent_ids: [" 10 ", "20", "10", ""]
`)
	cfg, err := LoadConfigWithOptions(cfgPath, LoadOptions{
		RequireConfluenceToken: false,
		RequireParentIDs:       true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.Join(cfg.Confluence.ParentIDs, ","); got != "10,20" {
		t.Fatalf("unexpected normalized parent_ids: %v", cfg.Confluence.ParentIDs)
	}
}

func TestLoadConfigWithOptionsMCPWriteEnabledRequiresToken(t *testing.T) {
	cfgPath := writeTestConfig(t, `
mcp:
  write_enabled: true
confluence:
  base_url: "https://example.atlassian.net"
  parent_ids: ["10"]
`)
	_, err := LoadConfigWithOptions(cfgPath, LoadOptions{
		RequireParentIDs:       true,
		RequireConfluenceToken: false,
	})
	if err == nil || !strings.Contains(err.Error(), "mcp.write_enabled requires confluence token") {
		t.Fatalf("expected MCP write token error, got: %v", err)
	}
}

func TestLoadConfigWithOptionsMCPWriteEnabledRequiresBaseURL(t *testing.T) {
	cfgPath := writeTestConfig(t, `
mcp:
  write_enabled: true
confluence:
  base_url: "   "
  token: "plain-token"
  parent_ids: ["10"]
`)
	_, err := LoadConfigWithOptions(cfgPath, LoadOptions{
		RequireParentIDs:       true,
		RequireConfluenceToken: false,
	})
	if err == nil || !strings.Contains(err.Error(), "mcp.write_enabled=true requires confluence.base_url") {
		t.Fatalf("expected MCP write base_url error, got: %v", err)
	}
}

func TestLoadConfigWithOptionsMCPWriteEnabledResolvesKeychain(t *testing.T) {
	defer func(orig func(...string) ([]byte, error)) { securityExec = orig }(securityExec)
	securityExec = func(args ...string) ([]byte, error) {
		return []byte("resolved-token"), nil
	}

	cfgPath := writeTestConfig(t, `
mcp:
  write_enabled: true
confluence:
  base_url: "https://example.atlassian.net"
  token: "keychain://mcp-test"
  parent_ids: ["10"]
`)
	cfg, err := LoadConfigWithOptions(cfgPath, LoadOptions{
		RequireParentIDs:       true,
		RequireConfluenceToken: false,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Confluence.Token != "resolved-token" {
		t.Fatalf("expected resolved token, got: %q", cfg.Confluence.Token)
	}
}

func TestLoadConfigWithOptionsDefaultsDatabasePath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfgPath := writeTestConfig(t, `
confluence:
  base_url: "https://example.atlassian.net"
  token: "plain-token"
  parent_ids: ["10"]
embeddings:
  provider: "noop"
  model: ""
`)

	cfg, err := LoadConfigWithOptions(cfgPath, LoadOptions{
		RequireConfluenceToken: false,
		RequireParentIDs:       true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := filepath.Join(home, ".local", "viki", "confluence", "replica.db")
	if cfg.Database.Path != want {
		t.Fatalf("unexpected default database path: got %q want %q", cfg.Database.Path, want)
	}
}

func TestBuildIndexProfileUsesNoopFallback(t *testing.T) {
	profile, err := buildIndexProfile(context.Background(), Config{
		Embeddings: struct {
			Provider   string `yaml:"provider"`
			BaseURL    string `yaml:"base_url"`
			Model      string `yaml:"model"`
			RequestSec int    `yaml:"request_timeout_seconds"`
		}{
			Provider: "noop",
		},
	}, search.NoopEmbedder{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if profile.EmbeddingProvider != "noop" {
		t.Fatalf("unexpected provider: %#v", profile)
	}
	if profile.EmbeddingDimension != 1 {
		t.Fatalf("expected noop dimension of 1, got %#v", profile)
	}
	if profile.ChunkingVersion != defaultChunkingVersion {
		t.Fatalf("unexpected chunking version: %#v", profile)
	}
}

func writeTestConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(strings.TrimSpace(body)), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}
