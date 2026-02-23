package context

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/user/gopherclaw/internal/types"
)

func TestNewEngine(t *testing.T) {
	e, err := New("gpt-4", 128000, 4096, "")
	if err != nil {
		t.Fatal(err)
	}
	if e == nil {
		t.Fatal("expected non-nil engine")
	}
}

func TestBuildPromptBasic(t *testing.T) {
	e, err := New("gpt-4", 128000, 4096, "")
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
	e, err := New("gpt-4", 128000, 4096, "")
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
	e, err := New("gpt-4", 500, 100, "")
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

func TestDefaultPromptContainsIdentity(t *testing.T) {
	e, err := New("gpt-4", 128000, 4096, "")
	if err != nil {
		t.Fatal(err)
	}

	session := &types.SessionIndex{SessionID: "test-123", Agent: "default", Status: "active"}
	messages, err := e.BuildPrompt(context.Background(), session, nil, nil, []string{"bash", "brave_search"})
	if err != nil {
		t.Fatal(err)
	}

	sysPrompt := messages[0].Content
	if !strings.Contains(sysPrompt, "Gopherclaw") {
		t.Error("default prompt should contain 'Gopherclaw'")
	}
	if !strings.Contains(sysPrompt, "test-123") {
		t.Error("default prompt should contain session ID")
	}
	if !strings.Contains(sysPrompt, "bash") {
		t.Error("default prompt should contain tool names")
	}
	if !strings.Contains(sysPrompt, "brave_search") {
		t.Error("default prompt should contain tool names")
	}
}

func TestCustomPromptFromFile(t *testing.T) {
	dir := t.TempDir()
	promptPath := filepath.Join(dir, "prompt.txt")
	err := os.WriteFile(promptPath, []byte("You are {{.SessionID}} bot with {{.Tools}} at {{.Time}}."), 0644)
	if err != nil {
		t.Fatal(err)
	}

	e, err := New("gpt-4", 128000, 4096, promptPath)
	if err != nil {
		t.Fatal(err)
	}

	session := &types.SessionIndex{SessionID: "custom-sess", Agent: "default", Status: "active"}
	messages, err := e.BuildPrompt(context.Background(), session, nil, nil, []string{"bash"})
	if err != nil {
		t.Fatal(err)
	}

	sysPrompt := messages[0].Content
	if !strings.Contains(sysPrompt, "custom-sess") {
		t.Errorf("custom prompt should contain session ID, got %q", sysPrompt)
	}
	if !strings.Contains(sysPrompt, "bash") {
		t.Errorf("custom prompt should contain tool name, got %q", sysPrompt)
	}
	// Should NOT contain default prompt content
	if strings.Contains(sysPrompt, "Gopherclaw") {
		t.Error("custom prompt should not contain default identity")
	}
}

func TestMissingPromptFileFallsBackToDefault(t *testing.T) {
	e, err := New("gpt-4", 128000, 4096, "/nonexistent/path/prompt.txt")
	if err != nil {
		t.Fatal(err)
	}

	session := &types.SessionIndex{SessionID: "test-456", Agent: "default", Status: "active"}
	messages, err := e.BuildPrompt(context.Background(), session, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	sysPrompt := messages[0].Content
	if !strings.Contains(sysPrompt, "Gopherclaw") {
		t.Error("missing file should fall back to default prompt")
	}
}

func TestInvalidTemplateReturnsError(t *testing.T) {
	dir := t.TempDir()
	promptPath := filepath.Join(dir, "bad.txt")
	err := os.WriteFile(promptPath, []byte("{{.Invalid"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	_, err = New("gpt-4", 128000, 4096, promptPath)
	if err == nil {
		t.Fatal("expected error for invalid template")
	}
}

func TestBuildPromptIncludesMemory(t *testing.T) {
	dir := t.TempDir()
	memPath := filepath.Join(dir, "memory.md")
	os.WriteFile(memPath, []byte("- User prefers dark mode\n- Name is Alex\n"), 0644)

	e, err := New("gpt-4", 128000, 4096, "")
	if err != nil {
		t.Fatal(err)
	}
	e.SetMemoryPath(memPath)

	session := &types.SessionIndex{SessionID: "test-session", Agent: "default", Status: "active"}
	messages, err := e.BuildPrompt(context.Background(), session, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	sysPrompt := messages[0].Content
	if !strings.Contains(sysPrompt, "User prefers dark mode") {
		t.Error("system prompt should contain memory content")
	}
	if !strings.Contains(sysPrompt, "Name is Alex") {
		t.Error("system prompt should contain all memories")
	}
}

func TestBuildPromptNoMemoryFile(t *testing.T) {
	e, err := New("gpt-4", 128000, 4096, "")
	if err != nil {
		t.Fatal(err)
	}
	e.SetMemoryPath("/nonexistent/memory.md")

	session := &types.SessionIndex{SessionID: "test-session", Agent: "default", Status: "active"}
	messages, err := e.BuildPrompt(context.Background(), session, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	sysPrompt := messages[0].Content
	if strings.Contains(sysPrompt, "## Memories") {
		t.Error("should not include memories section when file doesn't exist")
	}
}

func TestSummarizeIncludesMemoryTokens(t *testing.T) {
	dir := t.TempDir()
	memPath := filepath.Join(dir, "memory.md")
	os.WriteFile(memPath, []byte("- Fact one\n- Fact two\n"), 0644)

	e, err := New("gpt-4", 128000, 4096, "")
	if err != nil {
		t.Fatal(err)
	}
	e.SetMemoryPath(memPath)

	session := &types.SessionIndex{SessionID: "test-session", Agent: "default", Status: "active"}
	withMem := e.Summarize(session, nil, nil)

	e2, _ := New("gpt-4", 128000, 4096, "")
	withoutMem := e2.Summarize(session, nil, nil)

	if withMem.SystemPromptTokens <= withoutMem.SystemPromptTokens {
		t.Errorf("memory should increase system prompt tokens: with=%d without=%d",
			withMem.SystemPromptTokens, withoutMem.SystemPromptTokens)
	}
}
