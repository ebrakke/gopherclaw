package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
)

const maxReadURLChars = 50000

// ReadURL fetches a URL and converts its HTML content to markdown.
type ReadURL struct {
	client *http.Client
}

// NewReadURL creates a new ReadURL tool.
func NewReadURL() *ReadURL {
	return &ReadURL{
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (r *ReadURL) Name() string        { return "read_url" }
func (r *ReadURL) Description() string { return "Fetch a URL and return its content as markdown" }
func (r *ReadURL) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"url": {"type": "string", "description": "The URL to fetch"}
		},
		"required": ["url"]
	}`)
}

func (r *ReadURL) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("parse args: %w", err)
	}
	if params.URL == "" {
		return "", fmt.Errorf("url is required")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, params.URL, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "Gopherclaw/1.0")

	resp, err := r.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP error: status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}

	md, err := htmltomarkdown.ConvertString(string(body))
	if err != nil {
		return "", fmt.Errorf("convert to markdown: %w", err)
	}

	if len(md) > maxReadURLChars {
		md = md[:maxReadURLChars] + "\n\n[Content truncated]"
	}

	return md, nil
}
