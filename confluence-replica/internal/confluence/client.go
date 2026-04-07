package confluence

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
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
	return out, nil
}

func (c *Client) get(ctx context.Context, u string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
		return fmt.Errorf("confluence http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
