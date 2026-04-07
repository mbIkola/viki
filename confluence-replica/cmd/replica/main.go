package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"confluence-replica/internal/app"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatalf("usage: replica <bootstrap|sync|digest> [flags]")
	}

	cmd := os.Args[1]
	switch cmd {
	case "bootstrap", "sync":
		runSyncLike(cmd)
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
	_ = fs.Parse(os.Args[2:])

	ctx := context.Background()
	cfg, err := app.LoadConfig(*configPath)
	if err != nil {
		log.Fatal(err)
	}
	if *parentID == "" {
		*parentID = cfg.Confluence.DefaultParentID
	}
	if *parentID == "" {
		log.Fatal("parent-id is required")
	}

	rt, err := app.NewRuntime(ctx, cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer rt.Close()

	if mode == "bootstrap" {
		err = rt.Ingest.Bootstrap(ctx, *parentID)
	} else {
		err = rt.Ingest.Sync(ctx, *parentID)
	}
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s completed for parent %s\n", mode, *parentID)
}

func runDigest() {
	fs := flag.NewFlagSet("digest", flag.ExitOnError)
	configPath := fs.String("config", "config/config.yaml", "path to config yaml")
	dateText := fs.String("date", time.Now().Format("2006-01-02"), "digest date")
	_ = fs.Parse(os.Args[2:])

	day, err := time.Parse("2006-01-02", *dateText)
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	cfg, err := app.LoadConfig(*configPath)
	if err != nil {
		log.Fatal(err)
	}
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
