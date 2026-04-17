package confluence

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestUpdatePage_RetriesOnConflict(t *testing.T) {
	state := struct {
		putCount         int
		conflictVersion  int
		refreshedVersion int
		secondPutVersion int
	}{
		conflictVersion:  1,
		refreshedVersion: 10,
	}

	client := NewClient("http://example.invalid", "", time.Second)
	client.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch {
			case req.Method == http.MethodGet && req.URL.Path == "/rest/api/content/p":
				var version int
				switch {
				case state.putCount == 0:
					version = state.conflictVersion
				case state.putCount == 1:
					version = state.refreshedVersion
				default:
					version = state.secondPutVersion
				}
				page := Page{
					ID:    "p",
					Title: "t",
					Version: Version{
						Number: version,
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
				state.putCount++
				var payload map[string]any
				if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
					t.Fatalf("decode put payload: %v", err)
				}
				versionMap, ok := payload["version"].(map[string]any)
				if !ok {
					t.Fatalf("missing version object")
				}
				versionNumber := int(versionMap["number"].(float64))
				if state.putCount == 1 {
					expected := state.conflictVersion + 1
					if versionNumber != expected {
						t.Fatalf("first put version=%d want=%d", versionNumber, expected)
					}
					return &http.Response{
						StatusCode: http.StatusConflict,
						Body:       io.NopCloser(strings.NewReader("conflict")),
						Header:     http.Header{"Content-Type": {"text/plain"}},
					}, nil
				}
				expected := state.refreshedVersion + 1
				if versionNumber != expected {
					t.Fatalf("second put version=%d want=%d", versionNumber, expected)
				}
				state.secondPutVersion = versionNumber
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
	if updated.Version.Number != state.secondPutVersion {
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
				if payload["title"] != "child" {
					t.Fatalf("title=%v", payload["title"])
				}
				body := payload["body"].(map[string]any)
				storage := body["storage"].(map[string]any)
				if storage["value"] != "body" {
					t.Fatalf("storage value=%v", storage["value"])
				}
				if storage["representation"] != "storage" {
					t.Fatalf("storage repr=%v", storage["representation"])
				}
				if payload["space"].(map[string]any)["key"] != "XYZ" {
					t.Fatalf("space=%v", payload["space"])
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
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(bytes.NewReader(b)),
		Header:     http.Header{"Content-Type": {"application/json"}},
	}
}
