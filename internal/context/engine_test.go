package context

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/user/gopherclaw/internal/types"
)

func TestNewEngine(t *testing.T) {
	e, err := New("gpt-4", 128000, 4096)
	if err != nil {
		t.Fatal(err)
	}
	if e == nil {
		t.Fatal("expected non-nil engine")
	}
}

func TestBuildPromptBasic(t *testing.T) {
	e, err := New("gpt-4", 128000, 4096)
	if err != nil {
		t.Fatal(err)
	}

	session := &types.SessionIndex{
		SessionID: "test-session",
		Agent:     "default",
		Status:    "active",
	}

	userPayload, _ := json.Marshal(map[string]string{"text": "hello"})
	assistantPayload, _ := json.Marshal(map[string]string{"text": "hi there"})

	events := []*types.Event{
		{ID: "e1", SessionID: "test-session", Seq: 1, Type: "user_message", Source: "telegram", At: time.Now(), Payload: userPayload},
		{ID: "e2", SessionID: "test-session", Seq: 2, Type: "assistant_message", Source: "runtime", At: time.Now(), Payload: assistantPayload},
	}

	messages, err := e.BuildPrompt(context.Background(), session, events, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Should have: system prompt + 2 event messages
	if len(messages) < 3 {
		t.Fatalf("expected at least 3 messages, got %d", len(messages))
	}
	if messages[0].Role != "system" {
		t.Errorf("expected system message first, got %q", messages[0].Role)
	}
	if messages[1].Role != "user" {
		t.Errorf("expected user message, got %q", messages[1].Role)
	}
	if messages[1].Content != "hello" {
		t.Errorf("expected 'hello', got %q", messages[1].Content)
	}
	if messages[2].Role != "assistant" {
		t.Errorf("expected assistant message, got %q", messages[2].Role)
	}
}

func TestBuildPromptToolCallEvents(t *testing.T) {
	e, err := New("gpt-4", 128000, 4096)
	if err != nil {
		t.Fatal(err)
	}

	session := &types.SessionIndex{SessionID: "test-session", Agent: "default", Status: "active"}

	tcPayload, _ := json.Marshal(map[string]any{
		"tool": "bash", "call_id": "tc1",
		"arguments": map[string]string{"command": "echo hi"},
	})
	trPayload, _ := json.Marshal(map[string]any{
		"tool": "bash", "call_id": "tc1", "result": "hi\n",
	})

	events := []*types.Event{
		{ID: "e1", Seq: 1, Type: "user_message", Source: "telegram", Payload: json.RawMessage(`{"text":"run echo"}`)},
		{ID: "e2", Seq: 2, Type: "tool_call", Source: "runtime", Payload: tcPayload},
		{ID: "e3", Seq: 3, Type: "tool_result", Source: "runtime", Payload: trPayload},
		{ID: "e4", Seq: 4, Type: "assistant_message", Source: "runtime", Payload: json.RawMessage(`{"text":"done"}`)},
	}

	messages, err := e.BuildPrompt(context.Background(), session, events, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// system + user + assistant(tool_call) + tool_result + assistant
	if len(messages) < 5 {
		t.Fatalf("expected at least 5 messages, got %d", len(messages))
	}
}

func TestBuildPromptBudgetTruncation(t *testing.T) {
	// Tiny budget: only 500 tokens total, 100 reserve
	e, err := New("gpt-4", 500, 100)
	if err != nil {
		t.Fatal(err)
	}

	session := &types.SessionIndex{SessionID: "test-session", Agent: "default", Status: "active"}

	// Create many events that exceed the budget
	events := make([]*types.Event, 50)
	for i := range events {
		payload, _ := json.Marshal(map[string]string{"text": "This is a message that takes up tokens in the context window budget."})
		events[i] = &types.Event{
			ID: types.EventID(fmt.Sprintf("e%d", i)), Seq: int64(i + 1),
			Type: "user_message", Source: "test", Payload: payload,
		}
	}

	messages, err := e.BuildPrompt(context.Background(), session, events, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Should have fewer messages than events due to budget
	if len(messages) >= 51 {
		t.Errorf("expected truncation, got %d messages for 50 events", len(messages))
	}
	// Must have at least system prompt
	if len(messages) < 1 {
		t.Fatal("expected at least system prompt")
	}
}
