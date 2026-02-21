package llm

import (
	"context"
	"testing"
)

// MockProvider is a test double that satisfies the Provider interface.
type MockProvider struct {
	CompleteFunc func(ctx context.Context, messages []Message, tools []Tool) (*Response, error)
	StreamFunc   func(ctx context.Context, messages []Message, tools []Tool) (<-chan Delta, error)
}

func (m *MockProvider) Complete(ctx context.Context, messages []Message, tools []Tool) (*Response, error) {
	if m.CompleteFunc != nil {
		return m.CompleteFunc(ctx, messages, tools)
	}
	return &Response{Content: "mock response"}, nil
}

func (m *MockProvider) Stream(ctx context.Context, messages []Message, tools []Tool) (<-chan Delta, error) {
	if m.StreamFunc != nil {
		return m.StreamFunc(ctx, messages, tools)
	}
	ch := make(chan Delta, 1)
	ch <- Delta{Content: "mock stream"}
	close(ch)
	return ch, nil
}

func TestProviderInterface(t *testing.T) {
	var provider Provider = &MockProvider{}
	ctx := context.Background()
	messages := []Message{{Role: "user", Content: "test"}}

	resp, err := provider.Complete(ctx, messages, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content == "" {
		t.Error("expected non-empty response")
	}

	stream, err := provider.Stream(ctx, messages, nil)
	if err != nil {
		t.Fatal(err)
	}
	delta := <-stream
	if delta.Content == "" {
		t.Error("expected non-empty delta")
	}
}

func TestMockProviderCustomComplete(t *testing.T) {
	mock := &MockProvider{
		CompleteFunc: func(ctx context.Context, messages []Message, tools []Tool) (*Response, error) {
			return &Response{
				Content: "custom response",
				Usage: Usage{
					InputTokens:  10,
					OutputTokens: 5,
					TotalTokens:  15,
				},
			}, nil
		},
	}

	var provider Provider = mock
	ctx := context.Background()
	messages := []Message{{Role: "user", Content: "hello"}}

	resp, err := provider.Complete(ctx, messages, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content != "custom response" {
		t.Errorf("expected 'custom response', got %q", resp.Content)
	}
	if resp.Usage.TotalTokens != 15 {
		t.Errorf("expected 15 total tokens, got %d", resp.Usage.TotalTokens)
	}
}

func TestMockProviderCustomStream(t *testing.T) {
	mock := &MockProvider{
		StreamFunc: func(ctx context.Context, messages []Message, tools []Tool) (<-chan Delta, error) {
			ch := make(chan Delta, 3)
			ch <- Delta{Content: "hello "}
			ch <- Delta{Content: "world"}
			ch <- Delta{Content: "!"}
			close(ch)
			return ch, nil
		},
	}

	var provider Provider = mock
	ctx := context.Background()
	messages := []Message{{Role: "user", Content: "test"}}

	stream, err := provider.Stream(ctx, messages, nil)
	if err != nil {
		t.Fatal(err)
	}

	var accumulated string
	for delta := range stream {
		accumulated += delta.Content
	}
	if accumulated != "hello world!" {
		t.Errorf("expected 'hello world!', got %q", accumulated)
	}
}
