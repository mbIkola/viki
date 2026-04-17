package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"confluence-replica/internal/confluence"
	mcpserver "confluence-replica/internal/mcp"
)

func TestUpdatePage_WriteDisabled(t *testing.T) {
	backend := runtimeBackend{
		writeEnabled: false,
	}

	_, err := backend.UpdatePage(context.Background(), mcpserver.UpdatePageRequest{
		PageID: "123",
		Title:  ptr("new-title"),
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "write_disabled") {
		t.Fatalf("expected write_disabled, got %v", err)
	}
}

func TestClassifyWriteError_Conflict(t *testing.T) {
	err := classifyWriteError(&confluence.HTTPError{
		Method: http.MethodPut,
		URL:    "http://example.invalid/rest/api/content/123",
		Status: http.StatusConflict,
		Body:   "conflict",
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "version_conflict") {
		t.Fatalf("expected version_conflict, got %v", err)
	}
}

func TestClassifyWriteError_Auth(t *testing.T) {
	tests := []struct {
		name   string
		status int
	}{
		{name: "unauthorized", status: http.StatusUnauthorized},
		{name: "forbidden", status: http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := classifyWriteError(&confluence.HTTPError{
				Method: http.MethodPut,
				URL:    "http://example.invalid/rest/api/content/123",
				Status: tt.status,
				Body:   "auth denied",
			})
			if err == nil {
				t.Fatalf("expected error")
			}
			if !strings.Contains(err.Error(), "auth_error") {
				t.Fatalf("expected auth_error, got %v", err)
			}
		})
	}
}

func TestClassifyWriteError_UpstreamHTTP(t *testing.T) {
	err := classifyWriteError(&confluence.HTTPError{
		Method: http.MethodPut,
		URL:    "http://example.invalid/rest/api/content/123",
		Status: http.StatusInternalServerError,
		Body:   "server boom",
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "upstream_error") {
		t.Fatalf("expected upstream_error, got %v", err)
	}
}

func TestClassifyWriteError_UpstreamGeneric(t *testing.T) {
	err := classifyWriteError(errors.New("dial tcp timeout"))
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "upstream_error") {
		t.Fatalf("expected upstream_error, got %v", err)
	}
}

func TestUpdatePage_LocalRefreshFailedAfterRemoteSuccess(t *testing.T) {
	fakeClient := &fakeConfluenceWriteClient{
		updatePage: confluence.Page{
			ID:    "123",
			Title: "updated-title",
			Space: confluence.Space{Key: "ENG"},
			Version: confluence.Version{
				Number: 5,
			},
		},
	}
	backend := runtimeBackend{
		writeEnabled: true,
		client:       fakeClient,
		upsertPage: func(context.Context, confluence.Page) error {
			return errors.New("sqlite write failed")
		},
	}

	_, err := backend.UpdatePage(context.Background(), mcpserver.UpdatePageRequest{
		PageID:      "123",
		Title:       ptr("updated-title"),
		BodyStorage: ptr("<p>body</p>"),
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "local_refresh_failed") {
		t.Fatalf("expected local_refresh_failed, got %v", err)
	}
	if !strings.Contains(msg, "remote_applied=true local_refreshed=false") {
		t.Fatalf("expected remote/local markers, got %v", err)
	}
}

func TestCreateChildPage_LocalRefreshFailedAfterRemoteSuccess(t *testing.T) {
	fakeClient := &fakeConfluenceWriteClient{
		createChildPage: confluence.Page{
			ID:    "child",
			Title: "child-title",
			Space: confluence.Space{Key: "ENG"},
			Version: confluence.Version{
				Number: 1,
			},
			Ancestors: []confluence.Ancestor{{ID: "parent"}},
		},
		getPages: map[string]confluence.Page{
			"parent": {
				ID:    "parent",
				Title: "parent",
				Space: confluence.Space{Key: "ENG"},
				Version: confluence.Version{
					Number: 8,
				},
			},
		},
	}

	upsertCalls := 0
	backend := runtimeBackend{
		writeEnabled: true,
		client:       fakeClient,
		upsertPage: func(_ context.Context, p confluence.Page) error {
			upsertCalls++
			if p.ID == "parent" {
				return errors.New("sqlite write failed")
			}
			return nil
		},
	}

	_, err := backend.CreateChildPage(context.Background(), mcpserver.CreateChildPageRequest{
		ParentPageID: "parent",
		Title:        "child-title",
		BodyStorage:  "<p>child body</p>",
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "local_refresh_failed") {
		t.Fatalf("expected local_refresh_failed, got %v", err)
	}
	if !strings.Contains(msg, "remote_applied=true local_refreshed=false") {
		t.Fatalf("expected remote/local markers, got %v", err)
	}
	if upsertCalls != 2 {
		t.Fatalf("expected 2 upsert attempts (child + parent), got %d", upsertCalls)
	}
}

type fakeConfluenceWriteClient struct {
	updatePage      confluence.Page
	updatePageErr   error
	createChildPage confluence.Page
	createChildErr  error
	getPages        map[string]confluence.Page
	getPageErr      map[string]error
}

func (f *fakeConfluenceWriteClient) UpdatePage(_ context.Context, _ string, _ string, _ string, _ string) (confluence.Page, error) {
	if f.updatePageErr != nil {
		return confluence.Page{}, f.updatePageErr
	}
	return f.updatePage, nil
}

func (f *fakeConfluenceWriteClient) CreateChildPage(_ context.Context, _ string, _ string, _ string, _ string) (confluence.Page, error) {
	if f.createChildErr != nil {
		return confluence.Page{}, f.createChildErr
	}
	return f.createChildPage, nil
}

func (f *fakeConfluenceWriteClient) GetPage(_ context.Context, pageID string) (confluence.Page, error) {
	if err, ok := f.getPageErr[pageID]; ok {
		return confluence.Page{}, err
	}
	if page, ok := f.getPages[pageID]; ok {
		return page, nil
	}
	return confluence.Page{}, errors.New("missing page")
}

func ptr(s string) *string {
	return &s
}
