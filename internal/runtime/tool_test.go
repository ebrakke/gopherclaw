package runtime

import (
	"context"
	"encoding/json"
	"testing"
)

type echoTool struct{}

func (e *echoTool) Name() string        { return "echo" }
func (e *echoTool) Description() string  { return "Echoes input" }
func (e *echoTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}},"required":["text"]}`)
}
func (e *echoTool) Execute(_ context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Text string `json:"text"`
	}
	json.Unmarshal(args, &p)
	return p.Text, nil
}

func TestRegistryRegisterAndGet(t *testing.T) {
	r := NewRegistry()
	r.Register(&echoTool{})

	tool, ok := r.Get("echo")
	if !ok {
		t.Fatal("expected to find echo tool")
	}
	if tool.Name() != "echo" {
		t.Errorf("expected name 'echo', got %q", tool.Name())
	}
}

func TestRegistryGetMissing(t *testing.T) {
	r := NewRegistry()
	_, ok := r.Get("missing")
	if ok {
		t.Fatal("expected not to find missing tool")
	}
}

func TestRegistryAll(t *testing.T) {
	r := NewRegistry()
	r.Register(&echoTool{})
	tools := r.All()
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
}

func TestRegistryAsLLMTools(t *testing.T) {
	r := NewRegistry()
	r.Register(&echoTool{})
	llmTools := r.AsLLMTools()
	if len(llmTools) != 1 {
		t.Fatalf("expected 1 llm tool, got %d", len(llmTools))
	}
	if llmTools[0].Function.Name != "echo" {
		t.Errorf("expected function name 'echo', got %q", llmTools[0].Function.Name)
	}
	if llmTools[0].Type != "function" {
		t.Errorf("expected type 'function', got %q", llmTools[0].Type)
	}
}
