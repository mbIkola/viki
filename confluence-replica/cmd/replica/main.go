package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"confluence-replica/internal/app"
	"confluence-replica/internal/logx"
)

const (
	scopeModeFull    = "full"
	scopeModePartial = "partial"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatalf("usage: replica <bootstrap|sync|rebuild|digest> [flags]")
	}

	cmd := os.Args[1]
	switch cmd {
	case "bootstrap", "sync":
		runSyncLike(cmd)
	case "rebuild":
		runRebuild()
	case "digest":
		runDigest()
	default:
		log.Fatalf("unknown command: %s", cmd)
	}
}

func runSyncLike(mode string) {
	fs := flag.NewFlagSet(mode, flag.ExitOnError)
	configPath := fs.String("config", "config/config.yaml", "path to config yaml")
	parentID := fs.String("parent-id", "", "confluence parent page id")
	parentIDsCSV := fs.String("parent-ids", "", "comma-separated confluence parent page ids")
	quiet := fs.Bool("quiet", false, "set log level to ERROR")
	verbose := fs.Bool("verbose", false, "set log level to DEBUG")
	_ = fs.Parse(os.Args[2:])

	ctx := context.Background()
	overrideParentIDs := collectParentOverrides(*parentID, *parentIDsCSV)
	cfg, err := app.LoadConfigWithOptions(*configPath, app.LoadOptions{
		RequireConfluenceToken: true,
		RequireParentIDs:       true,
		ParentIDsOverride:      overrideParentIDs,
	})
	if err != nil {
		log.Fatal(err)
	}
	if err := logx.Configure(cfg.Logging.Level, *quiet, *verbose); err != nil {
		log.Fatal(err)
	}
	logx.Infof("[replica] command=%s config=%s", mode, *configPath)
	parentIDs, scopeMode, err := resolveParentIDs(cfg.Confluence.ParentIDs, overrideParentIDs)
	if err != nil {
		log.Fatal(err)
	}
	logx.Infof("[replica] import_params mode=%s scope_mode=%s parent_ids=%v confluence_url=%s", mode, scopeMode, parentIDs, cfg.Confluence.BaseURL)

	rt, err := app.NewRuntime(ctx, cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer rt.Close()

	if mode == "bootstrap" {
		err = rt.Ingest.Bootstrap(ctx, parentIDs, scopeMode)
	} else {
		err = rt.Ingest.Sync(ctx, parentIDs, scopeMode)
	}
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s completed for %d parent roots (%s)\n", mode, len(parentIDs), scopeMode)
}

func runRebuild() {
	fs := flag.NewFlagSet("rebuild", flag.ExitOnError)
	configPath := fs.String("config", "config/config.yaml", "path to config yaml")
	parentID := fs.String("parent-id", "", "confluence parent page id")
	parentIDsCSV := fs.String("parent-ids", "", "comma-separated confluence parent page ids")
	quiet := fs.Bool("quiet", false, "set log level to ERROR")
	verbose := fs.Bool("verbose", false, "set log level to DEBUG")
	_ = fs.Parse(os.Args[2:])

	ctx := context.Background()
	overrideParentIDs := collectParentOverrides(*parentID, *parentIDsCSV)
	cfg, err := app.LoadConfigWithOptions(*configPath, app.LoadOptions{
		RequireConfluenceToken: true,
		RequireParentIDs:       true,
		ParentIDsOverride:      overrideParentIDs,
	})
	if err != nil {
		log.Fatal(err)
	}
	if err := logx.Configure(cfg.Logging.Level, *quiet, *verbose); err != nil {
		log.Fatal(err)
	}
	parentIDs, scopeMode, err := resolveParentIDs(cfg.Confluence.ParentIDs, overrideParentIDs)
	if err != nil {
		log.Fatal(err)
	}
	if err := removeSQLiteArtifacts(cfg.Database.Path); err != nil {
		log.Fatal(err)
	}
	logx.Infof("[replica] command=rebuild config=%s database_path=%s scope_mode=%s parent_ids=%v", *configPath, cfg.Database.Path, scopeMode, parentIDs)

	rt, err := app.NewRuntime(ctx, cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer rt.Close()

	if err := rt.Ingest.Bootstrap(ctx, parentIDs, scopeMode); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("rebuild completed for %d parent roots (%s)\n", len(parentIDs), scopeMode)
}

func runDigest() {
	fs := flag.NewFlagSet("digest", flag.ExitOnError)
	configPath := fs.String("config", "config/config.yaml", "path to config yaml")
	dateText := fs.String("date", time.Now().Format("2006-01-02"), "digest date")
	quiet := fs.Bool("quiet", false, "set log level to ERROR")
	verbose := fs.Bool("verbose", false, "set log level to DEBUG")
	_ = fs.Parse(os.Args[2:])

	day, err := time.Parse("2006-01-02", *dateText)
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()
	cfg, err := app.LoadConfigWithOptions(*configPath, app.LoadOptions{
		RequireConfluenceToken: true,
		RequireParentIDs:       false,
	})
	if err != nil {
		log.Fatal(err)
	}
	if err := logx.Configure(cfg.Logging.Level, *quiet, *verbose); err != nil {
		log.Fatal(err)
	}
	logx.Infof("[replica] command=digest config=%s date=%s", *configPath, day.Format("2006-01-02"))
	rt, err := app.NewRuntime(ctx, cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer rt.Close()

	md, err := rt.Digest.Generate(ctx, day)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(md)
}

func collectParentOverrides(parentID, parentIDsCSV string) []string {
	out := make([]string, 0)
	out = append(out, splitParentIDsCSV(parentIDsCSV)...)
	if id := strings.TrimSpace(parentID); id != "" {
		out = append(out, id)
	}
	return normalizeIDs(out)
}

func splitParentIDsCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		out = append(out, strings.TrimSpace(p))
	}
	return out
}

func resolveParentIDs(configParentIDs, overrideParentIDs []string) ([]string, string, error) {
	override := normalizeIDs(overrideParentIDs)
	if len(override) > 0 {
		return override, scopeModePartial, nil
	}
	cfg := normalizeIDs(configParentIDs)
	if len(cfg) == 0 {
		return nil, "", errors.New("confluence.parent_ids is required when no --parent-id/--parent-ids override is provided")
	}
	return cfg, scopeModeFull, nil
}

func normalizeIDs(ids []string) []string {
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

func removeSQLiteArtifacts(path string) error {
	for _, target := range []string{path, path + "-wal", path + "-shm"} {
		if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove %s: %w", target, err)
		}
	}
	return nil
}
