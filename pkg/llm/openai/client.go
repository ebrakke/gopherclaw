package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/user/gopherclaw/pkg/llm"
)

// Client implements the llm.Provider interface for OpenAI-compatible APIs.
type Client struct {
	config     *llm.Config
	httpClient *http.Client
}

// New creates a new OpenAI-compatible client with the given configuration.
func New(config *llm.Config) *Client {
	return &Client{
		config: config,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// chatRequest is the OpenAI chat completions request body.
type chatRequest struct {
	Model       string           `json:"model"`
	Messages    []requestMessage `json:"messages"`
	Tools       []llm.Tool       `json:"tools,omitempty"`
	MaxTokens   int              `json:"max_tokens,omitempty"`
	Temperature *float32         `json:"temperature,omitempty"`
}

// requestMessage is the OpenAI message format for requests.
type requestMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content"`
	ToolCalls  []llm.ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

// chatResponse is the OpenAI chat completions response body.
type chatResponse struct {
	Choices []choice      `json:"choices"`
	Usage   responseUsage `json:"usage"`
}

// choice represents a single completion choice.
type choice struct {
	Message responseMessage `json:"message"`
}

// responseMessage is the OpenAI message format in responses.
type responseMessage struct {
	Role      string         `json:"role"`
	Content   string         `json:"content"`
	ToolCalls []llm.ToolCall `json:"tool_calls,omitempty"`
}

// responseUsage is the OpenAI token usage format.
type responseUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Complete sends a chat completion request and returns the full response.
func (c *Client) Complete(ctx context.Context, messages []llm.Message, tools []llm.Tool) (*llm.Response, error) {
	reqMessages := make([]requestMessage, len(messages))
	for i, msg := range messages {
		rm := requestMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
		if msg.Role == "tool" && len(msg.Tools) > 0 {
			rm.ToolCallID = msg.Tools[0].ID
		} else if len(msg.Tools) > 0 {
			rm.ToolCalls = msg.Tools
		}
		reqMessages[i] = rm
	}

	reqBody := chatRequest{
		Model:    c.config.Model,
		Messages: reqMessages,
	}

	if len(tools) > 0 {
		reqBody.Tools = tools
	}

	if c.config.MaxTokens > 0 {
		reqBody.MaxTokens = c.config.MaxTokens
	}

	if c.config.Temperature != 0 {
		temp := c.config.Temperature
		reqBody.Temperature = &temp
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	url := c.config.BaseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.config.APIKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	choice := chatResp.Choices[0]
	return &llm.Response{
		Content:   choice.Message.Content,
		ToolCalls: choice.Message.ToolCalls,
		Usage: llm.Usage{
			InputTokens:  chatResp.Usage.PromptTokens,
			OutputTokens: chatResp.Usage.CompletionTokens,
			TotalTokens:  chatResp.Usage.TotalTokens,
		},
	}, nil
}

// Stream sends a chat completion request and returns a channel of incremental deltas.
// In v1, this is a simple wrapper over Complete that sends the complete response as a
// single delta, then closes the channel.
func (c *Client) Stream(ctx context.Context, messages []llm.Message, tools []llm.Tool) (<-chan llm.Delta, error) {
	resp, err := c.Complete(ctx, messages, tools)
	if err != nil {
		return nil, err
	}

	ch := make(chan llm.Delta, 1)
	ch <- llm.Delta{
		Content:   resp.Content,
		ToolCalls: resp.ToolCalls,
	}
	close(ch)

	return ch, nil
}
