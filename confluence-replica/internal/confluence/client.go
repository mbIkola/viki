package confluence

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"confluence-replica/internal/logx"
)

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func NewClient(baseURL, token string, timeout time.Duration) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

type Page struct {
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	Type      string     `json:"type"`
	Status    string     `json:"status"`
	Space     Space      `json:"space"`
	Version   Version    `json:"version"`
	Body      Body       `json:"body"`
	Ancestors []Ancestor `json:"ancestors"`
	Metadata  Metadata   `json:"metadata"`
	CreatedAt time.Time  `json:"createdAt"`
}

type Space struct {
	Key string `json:"key"`
}

type Version struct {
	Number int    `json:"number"`
	When   string `json:"when"`
	By     struct {
		AccountID string `json:"accountId"`
	} `json:"by"`
}

type Body struct {
	Storage struct {
		Value          string `json:"value"`
		Representation string `json:"representation"`
	} `json:"storage"`
}

type Ancestor struct {
	ID string `json:"id"`
}

type Metadata struct {
	Labels struct {
		Results []struct {
			Name string `json:"name"`
		} `json:"results"`
	} `json:"labels"`
}

type HTTPError struct {
	Method string
	URL    string
	Status int
	Body   string
}

func (he HTTPError) Error() string {
	body := strings.TrimSpace(he.Body)
	if body == "" {
		body = "(no body)"
	}
	return fmt.Sprintf("confluence http %d %s %s: %s", he.Status, he.Method, he.URL, body)
}

func IsVersionConflict(err error) bool {
	var httpErr *HTTPError
	return errors.As(err, &httpErr) && httpErr.Status == http.StatusConflict
}

type childResponse struct {
	Results []Page `json:"results"`
	Size    int    `json:"size"`
}

func (c *Client) GetPage(ctx context.Context, pageID string) (Page, error) {
	q := url.Values{}
	q.Set("expand", "body.storage,version,ancestors,metadata.labels")
	u := c.baseURL + "/rest/api/content/" + pageID + "?" + q.Encode()
	var p Page
	if err := c.get(ctx, u, &p); err != nil {
		return Page{}, err
	}
	return p, nil
}

func (c *Client) GetChildren(ctx context.Context, pageID string, start, limit int) ([]Page, error) {
	q := url.Values{}
	q.Set("type", "page")
	q.Set("expand", "body.storage,version,ancestors,metadata.labels")
	q.Set("start", strconv.Itoa(start))
	q.Set("limit", strconv.Itoa(limit))
	base, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, err
	}
	base.Path = path.Join(base.Path, "/rest/api/content", pageID, "child/page")
	base.RawQuery = q.Encode()

	var r childResponse
	if err := c.get(ctx, base.String(), &r); err != nil {
		return nil, err
	}
	return r.Results, nil
}

func (c *Client) WalkTree(ctx context.Context, parentID string) ([]Page, error) {
	logx.Infof("[confluence] walk_tree start parent_id=%s", parentID)
	root, err := c.GetPage(ctx, parentID)
	if err != nil {
		return nil, err
	}
	queue := []Page{root}
	out := make([]Page, 0, 64)
	seen := map[string]bool{}

	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		if seen[p.ID] {
			continue
		}
		seen[p.ID] = true
		out = append(out, p)

		start := 0
		for {
			kids, err := c.GetChildren(ctx, p.ID, start, 50)
			if err != nil {
				return nil, err
			}
			if len(kids) == 0 {
				break
			}
			queue = append(queue, kids...)
			if len(kids) < 50 {
				break
			}
			start += 50
		}
	}
	logx.Infof("[confluence] walk_tree done parent_id=%s pages=%d", parentID, len(out))
	return out, nil
}

func (c *Client) UpdatePage(ctx context.Context, pageID, title, bodyStorage, versionMessage string) (Page, error) {
	makePayload := func(current Page) map[string]any {
		version := map[string]any{
			"number": current.Version.Number + 1,
		}
		if versionMessage != "" {
			version["message"] = versionMessage
		}

		storageValue := current.Body.Storage.Value
		if bodyStorage != "" {
			storageValue = bodyStorage
		}

		titleValue := current.Title
		if title != "" {
			titleValue = title
		}

		return map[string]any{
			"id":      pageID,
			"type":    "page",
			"version": version,
			"title":   titleValue,
			"body": map[string]any{
				"storage": map[string]any{
					"value":          storageValue,
					"representation": "storage",
				},
			},
		}
	}

	current, err := c.GetPage(ctx, pageID)
	if err != nil {
		return Page{}, err
	}
	payload := makePayload(current)
	var lastErr error
	putURL := c.baseURL + "/rest/api/content/" + pageID
	for attempt := 0; attempt < 2; attempt++ {
		lastErr = c.doJSON(ctx, http.MethodPut, putURL, payload, nil)
		if lastErr == nil {
			return c.GetPage(ctx, pageID)
		}
		if !IsVersionConflict(lastErr) || attempt == 1 {
			return Page{}, lastErr
		}
		current, err = c.GetPage(ctx, pageID)
		if err != nil {
			return Page{}, err
		}
		payload = makePayload(current)
	}
	return Page{}, lastErr
}

func (c *Client) CreateChildPage(ctx context.Context, parentPageID, title, bodyStorage, versionMessage string) (Page, error) {
	if strings.TrimSpace(title) == "" {
		return Page{}, fmt.Errorf("title is required")
	}
	if strings.TrimSpace(bodyStorage) == "" {
		return Page{}, fmt.Errorf("bodyStorage is required")
	}

	parent, err := c.GetPage(ctx, parentPageID)
	if err != nil {
		return Page{}, err
	}

	payload := map[string]any{
		"type":  "page",
		"title": title,
		"ancestors": []map[string]string{
			{"id": parentPageID},
		},
		"space": map[string]string{
			"key": parent.Space.Key,
		},
		"body": map[string]any{
			"storage": map[string]any{
				"value":          bodyStorage,
				"representation": "storage",
			},
		},
		"version": map[string]any{
			"number": 1,
		},
	}
	if versionMessage != "" {
		payload["version"].(map[string]any)["message"] = versionMessage
	}

	var created Page
	if err := c.doJSON(ctx, http.MethodPost, c.baseURL+"/rest/api/content", payload, &created); err != nil {
		return Page{}, err
	}
	return c.GetPage(ctx, created.ID)
}

func (c *Client) doJSON(ctx context.Context, method, u string, payload any, out any) error {
	started := time.Now()
	var body io.Reader
	if payload != nil {
		buf := bytes.NewBuffer(nil)
		if err := json.NewEncoder(buf).Encode(payload); err != nil {
			return err
		}
		body = buf
	}

	logx.Debugf("[confluence] request method=%s url=%s", method, u)
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		logx.Errorf("[confluence] request_failed url=%s error=%v", u, err)
		return err
	}
	defer resp.Body.Close()
	logx.Debugf("[confluence] response url=%s status=%d duration_ms=%d", u, resp.StatusCode, time.Since(started).Milliseconds())

	if resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
		return &HTTPError{
			Method: method,
			URL:    u,
			Status: resp.StatusCode,
			Body:   strings.TrimSpace(string(bodyBytes)),
		}
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	io.Copy(io.Discard, resp.Body)
	return nil
}

func (c *Client) get(ctx context.Context, u string, out any) error {
	return c.doJSON(ctx, http.MethodGet, u, nil, out)
}
