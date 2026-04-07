package diff

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

type ChangeType string

const (
	ChangeCreated ChangeType = "created"
	ChangeUpdated ChangeType = "updated"
	ChangeDeleted ChangeType = "deleted"
	ChangeMoved   ChangeType = "moved"
)

type PageState struct {
	PageID       string
	Title        string
	ParentPageID string
	Version      int
	BodyNormHash string
	Exists       bool
}

type Event struct {
	PageID     string
	Type       ChangeType
	OldVersion int
	NewVersion int
	OldParent  string
	NewParent  string
	OldTitle   string
	NewTitle   string
	Summary    string
}

func DetectChanges(oldPages, newPages []PageState) []Event {
	oldMap := make(map[string]PageState, len(oldPages))
	newMap := make(map[string]PageState, len(newPages))
	for _, p := range oldPages {
		oldMap[p.PageID] = p
	}
	for _, p := range newPages {
		newMap[p.PageID] = p
	}

	events := make([]Event, 0)
	for id, np := range newMap {
		op, ok := oldMap[id]
		if !ok {
			events = append(events, Event{PageID: id, Type: ChangeCreated, NewVersion: np.Version, NewParent: np.ParentPageID, NewTitle: np.Title, Summary: fmt.Sprintf("created page %s", np.Title)})
			continue
		}

		changed := op.Version != np.Version || op.BodyNormHash != np.BodyNormHash || op.Title != np.Title || op.ParentPageID != np.ParentPageID
		if changed {
			evtType := ChangeUpdated
			if op.ParentPageID != np.ParentPageID {
				evtType = ChangeMoved
			}
			events = append(events, Event{
				PageID:     id,
				Type:       evtType,
				OldVersion: op.Version,
				NewVersion: np.Version,
				OldParent:  op.ParentPageID,
				NewParent:  np.ParentPageID,
				OldTitle:   op.Title,
				NewTitle:   np.Title,
				Summary:    summarize(op, np, evtType),
			})
		}
	}

	for id, op := range oldMap {
		if _, ok := newMap[id]; !ok {
			events = append(events, Event{PageID: id, Type: ChangeDeleted, OldVersion: op.Version, OldParent: op.ParentPageID, OldTitle: op.Title, Summary: fmt.Sprintf("deleted page %s", op.Title)})
		}
	}

	sort.Slice(events, func(i, j int) bool {
		if events[i].Type == events[j].Type {
			return events[i].PageID < events[j].PageID
		}
		return events[i].Type < events[j].Type
	})
	return events
}

func summarize(oldState, newState PageState, ct ChangeType) string {
	switch ct {
	case ChangeMoved:
		if oldState.Title != newState.Title {
			return fmt.Sprintf("moved from parent %s to %s and renamed from %q to %q", oldState.ParentPageID, newState.ParentPageID, oldState.Title, newState.Title)
		}
		return fmt.Sprintf("moved from parent %s to %s", oldState.ParentPageID, newState.ParentPageID)
	case ChangeUpdated:
		if oldState.Title != newState.Title {
			return fmt.Sprintf("updated and renamed from %q to %q", oldState.Title, newState.Title)
		}
		return "content updated"
	default:
		return string(ct)
	}
}

var wsRE = regexp.MustCompile(`\s+`)

func NormalizeText(in string) string {
	trimmed := strings.TrimSpace(in)
	collapsed := wsRE.ReplaceAllString(trimmed, " ")
	return collapsed
}

func HashNormalizedText(in string) string {
	sum := sha256.Sum256([]byte(NormalizeText(in)))
	return hex.EncodeToString(sum[:])
}
