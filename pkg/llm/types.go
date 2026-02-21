package llm

import "encoding/json"

// Message represents a chat message in a conversation.
type Message struct {
	Role    string     `json:"role"`
	Content string     `json:"content"`
	Tools   []ToolCall `json:"tool_calls,omitempty"`
}

// ToolCall represents a tool invocation requested by the model.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// FunctionCall contains the function name and arguments for a tool call.
type FunctionCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// Tool describes a tool that can be provided to the model.
type Tool struct {
	Type     string   `json:"type"`
	Function Function `json:"function"`
}

// Function describes a callable function including its parameters schema.
type Function struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// Response represents a complete response from an LLM provider.
type Response struct {
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	Usage     Usage      `json:"usage"`
}

// Usage tracks token consumption for a request/response pair.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// Delta represents an incremental update during streaming.
type Delta struct {
	Content   string     `json:"content,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}
