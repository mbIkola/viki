package ingest

import (
	"context"
	"fmt"
	"strings"
	"time"

	"confluence-replica/internal/store"
)

type DigestService struct {
	store store.Store
}

func NewDigestService(st store.Store) *DigestService {
	return &DigestService{store: st}
}

func (d *DigestService) Generate(ctx context.Context, day time.Time) (string, error) {
	events, err := d.store.ListChangeEventsForDate(ctx, day)
	if err != nil {
		return "", err
	}

	var created, updated, moved, deleted []store.ChangeEvent
	for _, e := range events {
		switch e.Type {
		case "created":
			created = append(created, e)
		case "moved":
			moved = append(moved, e)
		case "deleted":
			deleted = append(deleted, e)
		default:
			updated = append(updated, e)
		}
	}

	b := &strings.Builder{}
	fmt.Fprintf(b, "# Confluence digest for %s\n\n", day.Format("2006-01-02"))
	writeSection(b, "New pages", created)
	writeSection(b, "Updated pages", updated)
	writeSection(b, "Moved pages", moved)
	writeSection(b, "Deleted pages", deleted)

	md := b.String()
	stats := map[string]any{
		"new":     len(created),
		"updated": len(updated),
		"moved":   len(moved),
		"deleted": len(deleted),
		"total":   len(events),
	}
	if err := d.store.SaveDigest(ctx, day, md, stats); err != nil {
		return "", err
	}
	return md, nil
}

func writeSection(b *strings.Builder, name string, events []store.ChangeEvent) {
	fmt.Fprintf(b, "## %s (%d)\n", name, len(events))
	if len(events) == 0 {
		b.WriteString("- none\n\n")
		return
	}
	for _, e := range events {
		fmt.Fprintf(b, "- `%s` page `%s` (v%d -> v%d): %s\n", e.Type, e.PageID, e.OldVersion, e.NewVersion, e.Summary)
	}
	b.WriteString("\n")
}
