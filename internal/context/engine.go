// internal/context/engine.go
package context

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/pkoukk/tiktoken-go"

	"github.com/user/gopherclaw/internal/types"
	"github.com/user/gopherclaw/pkg/llm"
)

// Engine assembles token-budgeted prompts for the LLM.
type Engine struct {
	tokenizer *tiktoken.Tiktoken
	maxTokens int
	reserve   int
}

// New creates a context engine with the specified token budget.
// model is used to select the appropriate tokenizer (e.g. "gpt-4").
// maxTokens is the model's context window size.
// reserve is the number of tokens to reserve for the model's response.
func New(model string, maxTokens, reserve int) (*Engine, error) {
	enc, err := tiktoken.EncodingForModel(model)
	if err != nil {
		// Fallback to cl100k_base for unknown models
		enc, err = tiktoken.GetEncoding("cl100k_base")
		if err != nil {
			return nil, fmt.Errorf("get tokenizer: %w", err)
		}
	}
	return &Engine{
		tokenizer: enc,
		maxTokens: maxTokens,
		reserve:   reserve,
	}, nil
}

// countTokens returns the token count for a string.
func (e *Engine) countTokens(text string) int {
	return len(e.tokenizer.Encode(text, nil, nil))
}

// BuildPrompt assembles a token-budgeted prompt from session history.
// toolNames is an optional list of available tool names for the system prompt.
// artifacts can be nil when artifact excerpts are not needed.
func (e *Engine) BuildPrompt(
	ctx context.Context,
	session *types.SessionIndex,
	events []*types.Event,
	artifacts types.ArtifactStore,
	toolNames []string,
) ([]llm.Message, error) {
	inputBudget := e.maxTokens - e.reserve

	// 1. System prompt
	sysPrompt := buildSystemPrompt(session, toolNames)
	sysTokens := e.countTokens(sysPrompt)
	remaining := inputBudget - sysTokens

	// 70% for events, 10% safety margin (20% artifact budget unused for now)
	eventBudget := int(float64(remaining) * 0.7)

	// 2. Convert events to messages, respecting budget
	var eventMessages []llm.Message
	usedTokens := 0

	for _, event := range events {
		msg, err := eventToMessage(event)
		if err != nil {
			continue
		}

		msgTokens := e.countTokens(msg.Content)
		for _, tc := range msg.Tools {
			msgTokens += e.countTokens(tc.Function.Name)
			msgTokens += e.countTokens(string(tc.Function.Arguments))
		}

		if usedTokens+msgTokens > eventBudget {
			break
		}

		eventMessages = append(eventMessages, msg)
		usedTokens += msgTokens
	}

	// 3. Assemble: system + events (already in chronological order)
	messages := make([]llm.Message, 0, 1+len(eventMessages))
	messages = append(messages, llm.Message{Role: "system", Content: sysPrompt})
	messages = append(messages, eventMessages...)

	return messages, nil
}

func buildSystemPrompt(session *types.SessionIndex, toolNames []string) string {
	prompt := fmt.Sprintf(
		"You are a helpful assistant. Current time: %s. Session: %s.",
		time.Now().Format(time.RFC3339),
		string(session.SessionID),
	)
	if len(toolNames) > 0 {
		prompt += fmt.Sprintf(" You have access to the following tools: %v.", toolNames)
	}
	return prompt
}

type eventPayload struct {
	Text      string          `json:"text"`
	Tool      string          `json:"tool"`
	CallID    string          `json:"call_id"`
	Arguments json.RawMessage `json:"arguments"`
	Result    string          `json:"result"`
}

func eventToMessage(event *types.Event) (llm.Message, error) {
	var payload eventPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return llm.Message{}, err
	}

	switch event.Type {
	case "user_message":
		return llm.Message{Role: "user", Content: payload.Text}, nil

	case "assistant_message":
		return llm.Message{Role: "assistant", Content: payload.Text}, nil

	case "tool_call":
		return llm.Message{
			Role: "assistant",
			Tools: []llm.ToolCall{{
				ID:   payload.CallID,
				Type: "function",
				Function: llm.FunctionCall{
					Name:      payload.Tool,
					Arguments: payload.Arguments,
				},
			}},
		}, nil

	case "tool_result":
		return llm.Message{
			Role:    "tool",
			Content: payload.Result,
			Tools: []llm.ToolCall{{
				ID: payload.CallID,
			}},
		}, nil

	default:
		return llm.Message{}, fmt.Errorf("unknown event type: %s", event.Type)
	}
}
