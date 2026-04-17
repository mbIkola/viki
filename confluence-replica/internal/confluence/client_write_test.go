package confluence

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestUpdatePage_RetriesOnConflict(t *testing.T) {
	state := struct {
		attempt int
		version int
	}{version: 1}

	client := NewClient("http://example.invalid", "", time.Second)
	client.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch {
			case req.Method == http.MethodGet && req.URL.Path == "/rest/api/content/p":
				page := Page{
					ID:    "p",
					Title: "t",
					Version: Version{
						Number: state.version,
					},
					Body: Body{
						Storage: struct {
							Value          string `json:"value"`
							Representation string `json:"representation"`
						}{
							Value:          "orig",
							Representation: "storage",
						},
					},
					Space: Space{Key: "S"},
				}
				return jsonResponse(http.StatusOK, page), nil
			case req.Method == http.MethodPut && req.URL.Path == "/rest/api/content/p":
				state.attempt++
				if state.attempt == 1 {
					return &http.Response{
						StatusCode: http.StatusConflict,
						Body:       io.NopCloser(bytes.NewBufferString("conflict")),
						Header:     http.Header{"Content-Type": {"text/plain"}},
					}, nil
				}
				state.version++
				return jsonResponse(http.StatusOK, struct{}{}), nil
			default:
				t.Fatalf("unexpected request %s %s", req.Method, req.URL.Path)
				return nil, nil
			}
		}),
	}

	updated, err := client.UpdatePage(context.Background(), "p", "", "new", "")
	if err != nil {
		t.Fatalf("update page: %v", err)
	}
	if updated.Version.Number != 2 {
		t.Fatalf("version=%d", updated.Version.Number)
	}
}

func TestCreateChildPage_IncludesParent(t *testing.T) {
	client := NewClient("http://example.invalid", "", time.Second)
	callCount := 0
	client.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch {
			case req.Method == http.MethodGet && req.URL.Path == "/rest/api/content/parent":
				return jsonResponse(http.StatusOK, Page{ID: "parent", Space: Space{Key: "XYZ"}}), nil
			case req.Method == http.MethodPost && req.URL.Path == "/rest/api/content":
				callCount++
				var payload map[string]any
				if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
					t.Fatalf("decode payload: %v", err)
				}
				space := payload["space"].(map[string]any)
				if space["key"] != "XYZ" {
					t.Fatalf("space=%v", space)
				}
				ancestors := payload["ancestors"].([]any)
				if len(ancestors) != 1 {
					t.Fatalf("ancestors=%v", ancestors)
				}
				if ancestors[0].(map[string]any)["id"] != "parent" {
					t.Fatalf("ancestor=%v", ancestors[0])
				}
				return jsonResponse(http.StatusOK, Page{ID: "child"}), nil
			case req.Method == http.MethodGet && req.URL.Path == "/rest/api/content/child":
				return jsonResponse(http.StatusOK, Page{ID: "child", Space: Space{Key: "XYZ"}}), nil
			default:
				t.Fatalf("unexpected request %s %s", req.Method, req.URL.Path)
				return nil, nil
			}
		}),
	}

	child, err := client.CreateChildPage(context.Background(), "parent", "child", "body", "")
	if err != nil {
		t.Fatalf("create child: %v", err)
	}
	if child.ID != "child" {
		t.Fatalf("child=%s", child.ID)
	}
	if callCount != 1 {
		t.Fatalf("post called %d times", callCount)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func jsonResponse(status int, payload any) *http.Response {
	b, _ := json.Marshal(payload)
	resp := &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(bytes.NewReader(b)),
		Header:     http.Header{"Content-Type": {"application/json"}},
	}
	return resp
}
