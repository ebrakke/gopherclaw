package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// BraveSearch searches the web via Brave Search API.
type BraveSearch struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

// NewBraveSearch creates a new Brave Search tool.
func NewBraveSearch(apiKey string) *BraveSearch {
	return &BraveSearch{
		apiKey:  apiKey,
		baseURL: "https://api.search.brave.com/res/v1/web/search",
		client:  &http.Client{Timeout: 15 * time.Second},
	}
}

func (b *BraveSearch) Name() string        { return "brave_search" }
func (b *BraveSearch) Description() string { return "Search the web using Brave Search" }
func (b *BraveSearch) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {"type": "string", "description": "Search query"},
			"count": {"type": "integer", "description": "Number of results (default: 5, max: 20)"}
		},
		"required": ["query"]
	}`)
}

type braveResponse struct {
	Web braveWeb `json:"web"`
}

type braveWeb struct {
	Results []braveResult `json:"results"`
}

type braveResult struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
}

func (b *BraveSearch) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Query string `json:"query"`
		Count int    `json:"count"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("parse args: %w", err)
	}
	if params.Query == "" {
		return "", fmt.Errorf("query is required")
	}
	if params.Count <= 0 {
		params.Count = 5
	}
	if params.Count > 20 {
		params.Count = 20
	}

	u, _ := url.Parse(b.baseURL)
	q := u.Query()
	q.Set("q", params.Query)
	q.Set("count", fmt.Sprintf("%d", params.Count))
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", b.apiKey)

	resp, err := b.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("search request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Brave API error (status %d): %s", resp.StatusCode, string(body))
	}

	var result braveResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}

	if len(result.Web.Results) == 0 {
		return "No results found.", nil
	}

	var sb strings.Builder
	for i, r := range result.Web.Results {
		fmt.Fprintf(&sb, "%d. %s\n   %s\n   %s\n\n", i+1, r.Title, r.URL, r.Description)
	}
	return sb.String(), nil
}
