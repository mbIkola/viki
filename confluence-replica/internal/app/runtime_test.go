package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSanitizeDSNMasksPassword(t *testing.T) {
	in := "postgres://alice:secret@localhost:5432/confluence_replica?sslmode=disable"
	got := sanitizeDSN(in)
	if got == in {
		t.Fatalf("expected sanitized dsn to differ from input")
	}
	if got == "" {
		t.Fatalf("expected non-empty dsn")
	}
	if strings.Contains(got, "secret") {
		t.Fatalf("password leaked in dsn: %s", got)
	}
}

func TestLoadConfigWithOptionsRequiresParentIDsWithoutOverride(t *testing.T) {
	cfgPath := writeTestConfig(t, `
database:
  dsn: "postgres://postgres:postgres@localhost:5432/confluence_replica?sslmode=disable"
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
  dsn: "postgres://postgres:postgres@localhost:5432/confluence_replica?sslmode=disable"
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
  dsn: "postgres://postgres:postgres@localhost:5432/confluence_replica?sslmode=disable"
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

func writeTestConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(strings.TrimSpace(body)), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}
