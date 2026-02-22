package runtime

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	ctxengine "github.com/user/gopherclaw/internal/context"
	"github.com/user/gopherclaw/internal/gateway"
	"github.com/user/gopherclaw/internal/state"
	"github.com/user/gopherclaw/internal/types"
	"github.com/user/gopherclaw/pkg/llm"
)

// mockProvider returns pre-configured responses.
type mockProvider struct {
	mu        sync.Mutex
	responses []*llm.Response
	callCount int
}

func (m *mockProvider) Complete(_ context.Context, messages []llm.Message, tools []llm.Tool) (*llm.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	idx := m.callCount
	m.callCount++
	if idx < len(m.responses) {
		return m.responses[idx], nil
	}
	return &llm.Response{Content: "fallback"}, nil
}

func (m *mockProvider) Stream(_ context.Context, messages []llm.Message, tools []llm.Tool) (<-chan llm.Delta, error) {
	return nil, nil
}

func TestProcessRunSimpleResponse(t *testing.T) {
	dir := t.TempDir()
	sessions := state.NewSessionStore(dir)
	events := state.NewEventStore(dir)
	artifacts := state.NewArtifactStore(dir)

	ctx := context.Background()
	sid, err := sessions.ResolveOrCreate(ctx, types.NewSessionKey("test", "user1"), "default")
	if err != nil {
		t.Fatal(err)
	}

	provider := &mockProvider{
		responses: []*llm.Response{
			{Content: "Hello! How can I help?"},
		},
	}

	engine, err := ctxengine.New("gpt-4", 128000, 4096)
	if err != nil {
		t.Fatal(err)
	}

	registry := NewRegistry()
	rt := New(provider, engine, sessions, events, artifacts, registry, 10)

	var callbackResult string
	done := make(chan struct{})

	run := &gateway.Run{
		ID:        types.NewRunID(),
		SessionID: sid,
		Event: &types.InboundEvent{
			Source:     "test",
			SessionKey: types.NewSessionKey("test", "user1"),
			UserID:     "user1",
			Text:       "hi",
		},
		Status:    gateway.RunStatusRunning,
		CreatedAt: time.Now(),
		OnComplete: func(resp string) {
			callbackResult = resp
			close(done)
		},
	}

	err = rt.ProcessRun(run)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for callback")
	}

	if callbackResult != "Hello! How can I help?" {
		t.Errorf("expected callback result, got %q", callbackResult)
	}

	// Verify events were recorded: user_message + assistant_message
	count, err := events.Count(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("expected 2 events, got %d", count)
	}
}

func TestProcessRunWithToolCall(t *testing.T) {
	dir := t.TempDir()
	sessions := state.NewSessionStore(dir)
	events := state.NewEventStore(dir)
	artifacts := state.NewArtifactStore(dir)

	ctx := context.Background()
	sid, err := sessions.ResolveOrCreate(ctx, types.NewSessionKey("test", "user1"), "default")
	if err != nil {
		t.Fatal(err)
	}

	provider := &mockProvider{
		responses: []*llm.Response{
			// First call: LLM requests a tool call
			{
				ToolCalls: []llm.ToolCall{{
					ID:   "tc1",
					Type: "function",
					Function: llm.FunctionCall{
						Name:      "echo",
						Arguments: json.RawMessage(`{"text":"world"}`),
					},
				}},
			},
			// Second call: LLM gives final response
			{Content: "The echo returned: world"},
		},
	}

	engine, err := ctxengine.New("gpt-4", 128000, 4096)
	if err != nil {
		t.Fatal(err)
	}

	registry := NewRegistry()
	registry.Register(&echoTool{})

	rt := New(provider, engine, sessions, events, artifacts, registry, 10)

	var callbackResult string
	done := make(chan struct{})

	run := &gateway.Run{
		ID:        types.NewRunID(),
		SessionID: sid,
		Event: &types.InboundEvent{
			Source:     "test",
			SessionKey: types.NewSessionKey("test", "user1"),
			UserID:     "user1",
			Text:       "echo world",
		},
		Status:    gateway.RunStatusRunning,
		CreatedAt: time.Now(),
		OnComplete: func(resp string) {
			callbackResult = resp
			close(done)
		},
	}

	err = rt.ProcessRun(run)
	if err != nil {
		t.Fatal(err)
	}

	<-done

	if callbackResult != "The echo returned: world" {
		t.Errorf("expected 'The echo returned: world', got %q", callbackResult)
	}

	// Events: user_message + tool_call + tool_result + assistant_message = 4
	count, err := events.Count(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	if count != 4 {
		t.Errorf("expected 4 events, got %d", count)
	}
}

func TestProcessRunMaxRounds(t *testing.T) {
	dir := t.TempDir()
	sessions := state.NewSessionStore(dir)
	events := state.NewEventStore(dir)
	artifacts := state.NewArtifactStore(dir)

	ctx := context.Background()
	sid, err := sessions.ResolveOrCreate(ctx, types.NewSessionKey("test", "user1"), "default")
	if err != nil {
		t.Fatal(err)
	}

	// Provider always returns tool calls (infinite loop)
	infProvider := &mockProvider{
		responses: make([]*llm.Response, 20),
	}
	for i := range infProvider.responses {
		infProvider.responses[i] = &llm.Response{
			ToolCalls: []llm.ToolCall{{
				ID: "tc1", Type: "function",
				Function: llm.FunctionCall{Name: "echo", Arguments: json.RawMessage(`{"text":"loop"}`)},
			}},
		}
	}

	engine, _ := ctxengine.New("gpt-4", 128000, 4096)
	registry := NewRegistry()
	registry.Register(&echoTool{})

	rt := New(infProvider, engine, sessions, events, artifacts, registry, 3) // max 3 rounds

	run := &gateway.Run{
		ID:        types.NewRunID(),
		SessionID: sid,
		Event:     &types.InboundEvent{Source: "test", SessionKey: "test:u1", UserID: "u1", Text: "loop"},
		Status:    gateway.RunStatusRunning,
		CreatedAt: time.Now(),
	}

	err = rt.ProcessRun(run)
	if err == nil {
		t.Fatal("expected error for max rounds exceeded")
	}
}
