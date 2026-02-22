package openai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/user/gopherclaw/pkg/llm"
)

func TestOpenAIClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("missing or invalid auth header")
		}

		resp := map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"role":    "assistant",
						"content": "test response",
					},
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     10,
				"completion_tokens": 5,
				"total_tokens":      15,
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	config := &llm.Config{
		BaseURL: server.URL,
		APIKey:  "test-key",
		Model:   "gpt-3.5-turbo",
	}
	client := New(config)

	ctx := context.Background()
	messages := []llm.Message{
		{Role: "user", Content: "hello"},
	}

	resp, err := client.Complete(ctx, messages, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content != "test response" {
		t.Errorf("expected 'test response', got %s", resp.Content)
	}
	if resp.Usage.InputTokens != 10 {
		t.Errorf("expected 10 input tokens, got %d", resp.Usage.InputTokens)
	}
	if resp.Usage.OutputTokens != 5 {
		t.Errorf("expected 5 output tokens, got %d", resp.Usage.OutputTokens)
	}
	if resp.Usage.TotalTokens != 15 {
		t.Errorf("expected 15 total tokens, got %d", resp.Usage.TotalTokens)
	}
}

func TestOpenAIClientRequestFormat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the request path: base_url includes /v1, client appends /chat/completions
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("expected path '/v1/chat/completions', got %q", r.URL.Path)
		}

		// Verify content type
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type 'application/json', got %q", r.Header.Get("Content-Type"))
		}

		// Parse and verify request body
		body, _ := io.ReadAll(r.Body)
		var reqBody map[string]any
		json.Unmarshal(body, &reqBody)

		if reqBody["model"] != "gpt-4" {
			t.Errorf("expected model 'gpt-4', got %v", reqBody["model"])
		}

		messages, ok := reqBody["messages"].([]any)
		if !ok || len(messages) != 1 {
			t.Errorf("expected 1 message, got %v", reqBody["messages"])
		}

		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"role": "assistant", "content": "ok"}},
			},
			"usage": map[string]any{
				"prompt_tokens":     1,
				"completion_tokens": 1,
				"total_tokens":      2,
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	config := &llm.Config{
		BaseURL: server.URL + "/v1",
		APIKey:  "key",
		Model:   "gpt-4",
	}
	client := New(config)

	_, err := client.Complete(context.Background(), []llm.Message{
		{Role: "user", Content: "test"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestOpenAIClientWithTools(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var reqBody map[string]any
		json.Unmarshal(body, &reqBody)

		tools, ok := reqBody["tools"].([]any)
		if !ok || len(tools) != 1 {
			t.Errorf("expected 1 tool, got %v", reqBody["tools"])
		}

		resp := map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"role":    "assistant",
						"content": "",
						"tool_calls": []map[string]any{
							{
								"id":   "call_123",
								"type": "function",
								"function": map[string]any{
									"name":      "get_weather",
									"arguments": `{"city":"NYC"}`,
								},
							},
						},
					},
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     20,
				"completion_tokens": 10,
				"total_tokens":      30,
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	config := &llm.Config{
		BaseURL: server.URL,
		APIKey:  "key",
		Model:   "gpt-4",
	}
	client := New(config)

	tools := []llm.Tool{
		{
			Type: "function",
			Function: llm.Function{
				Name:        "get_weather",
				Description: "Get the weather",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}}}`),
			},
		},
	}

	resp, err := client.Complete(context.Background(), []llm.Message{
		{Role: "user", Content: "What's the weather in NYC?"},
	}, tools)
	if err != nil {
		t.Fatal(err)
	}

	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Function.Name != "get_weather" {
		t.Errorf("expected tool call 'get_weather', got %q", resp.ToolCalls[0].Function.Name)
	}
}

func TestOpenAIClientAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"invalid api key"}}`))
	}))
	defer server.Close()

	config := &llm.Config{
		BaseURL: server.URL,
		APIKey:  "bad-key",
		Model:   "gpt-4",
	}
	client := New(config)

	_, err := client.Complete(context.Background(), []llm.Message{
		{Role: "user", Content: "hello"},
	}, nil)
	if err == nil {
		t.Fatal("expected error for 401 response")
	}
}

func TestOpenAIClientStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"role": "assistant", "content": "streamed response"}},
			},
			"usage": map[string]any{
				"prompt_tokens":     5,
				"completion_tokens": 3,
				"total_tokens":      8,
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	config := &llm.Config{
		BaseURL: server.URL,
		APIKey:  "key",
		Model:   "gpt-4",
	}
	client := New(config)

	stream, err := client.Stream(context.Background(), []llm.Message{
		{Role: "user", Content: "hello"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	var content string
	for delta := range stream {
		content += delta.Content
	}
	if content != "streamed response" {
		t.Errorf("expected 'streamed response', got %q", content)
	}
}

func TestOpenAIClientProviderInterface(t *testing.T) {
	// Verify Client satisfies the llm.Provider interface at compile time.
	var _ llm.Provider = (*Client)(nil)
}
