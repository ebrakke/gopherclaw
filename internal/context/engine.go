// internal/context/engine.go
package context

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"text/template"
	"time"

	"github.com/pkoukk/tiktoken-go"

	"github.com/user/gopherclaw/internal/types"
	"github.com/user/gopherclaw/pkg/llm"
)

// Engine assembles token-budgeted prompts for the LLM.
type Engine struct {
	tokenizer  *tiktoken.Tiktoken
	maxTokens  int
	reserve    int
	promptTmpl *template.Template
	memoryPath string
}

// PromptData holds the dynamic values injected into the system prompt template.
type PromptData struct {
	Time      string
	SessionID string
	Tools     string
	ToolList  []string
	Memory    string
}

// New creates a context engine with the specified token budget.
// model is used to select the appropriate tokenizer (e.g. "gpt-4").
// maxTokens is the model's context window size.
// reserve is the number of tokens to reserve for the model's response.
// promptPath is the path to a system prompt template file. If empty or the
// file does not exist, the built-in default prompt is used.
func New(model string, maxTokens, reserve int, promptPath string) (*Engine, error) {
	enc, err := tiktoken.EncodingForModel(model)
	if err != nil {
		// Fallback to cl100k_base for unknown models
		enc, err = tiktoken.GetEncoding("cl100k_base")
		if err != nil {
			return nil, fmt.Errorf("get tokenizer: %w", err)
		}
	}

	tmpl, err := loadPromptTemplate(promptPath)
	if err != nil {
		return nil, fmt.Errorf("load system prompt: %w", err)
	}

	return &Engine{
		tokenizer:  enc,
		maxTokens:  maxTokens,
		reserve:    reserve,
		promptTmpl: tmpl,
	}, nil
}

// SetMemoryPath configures the path to the persistent memory file.
func (e *Engine) SetMemoryPath(path string) {
	e.memoryPath = path
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
	sysPrompt := e.buildSystemPrompt(session, toolNames)
	sysTokens := e.countTokens(sysPrompt)
	remaining := inputBudget - sysTokens

	// 70% for events, 10% safety margin (20% artifact budget unused for now)
	eventBudget := int(float64(remaining) * 0.7)

	// 2. Convert events to messages, walking newest-first to prioritize recent context
	var eventMessages []llm.Message
	usedTokens := 0

	for i := len(events) - 1; i >= 0; i-- {
		msg, err := eventToMessage(events[i])
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

	// 3. Reverse to chronological order and assemble
	for i, j := 0, len(eventMessages)-1; i < j; i, j = i+1, j-1 {
		eventMessages[i], eventMessages[j] = eventMessages[j], eventMessages[i]
	}

	messages := make([]llm.Message, 0, 1+len(eventMessages))
	messages = append(messages, llm.Message{Role: "system", Content: sysPrompt})
	messages = append(messages, eventMessages...)

	return messages, nil
}

func (e *Engine) buildSystemPrompt(session *types.SessionIndex, toolNames []string) string {
	memory := ""
	if e.memoryPath != "" {
		if data, err := os.ReadFile(e.memoryPath); err == nil {
			content := strings.TrimSpace(string(data))
			if content != "" {
				memory = content
			}
		}
	}

	data := PromptData{
		Time:      time.Now().Format(time.RFC3339),
		SessionID: string(session.SessionID),
		ToolList:  toolNames,
		Tools:     strings.Join(toolNames, ", "),
		Memory:    memory,
	}

	var buf bytes.Buffer
	if err := e.promptTmpl.Execute(&buf, data); err != nil {
		slog.Error("execute system prompt template", "error", err)
		// Fallback to a minimal prompt
		return fmt.Sprintf("You are a helpful assistant. Current time: %s.", data.Time)
	}
	return buf.String()
}

// ContextSummary holds token budget stats for debugging context assembly.
type ContextSummary struct {
	MaxTokens         int
	Reserve           int
	InputBudget       int
	SystemPromptTokens int
	SystemPromptText  string
	EventBudget       int
	EventTokensUsed   int
	EventsIncluded    int
	EventsTotal       int
	BudgetRemaining   int
}

// Summarize computes context budget stats for the given session and events
// without building the full prompt. toolNames should match what the runtime
// passes to BuildPrompt.
func (e *Engine) Summarize(
	session *types.SessionIndex,
	events []*types.Event,
	toolNames []string,
) *ContextSummary {
	inputBudget := e.maxTokens - e.reserve

	sysPrompt := e.buildSystemPrompt(session, toolNames)
	sysTokens := e.countTokens(sysPrompt)
	remaining := inputBudget - sysTokens

	eventBudget := int(float64(remaining) * 0.7)

	usedTokens := 0
	included := 0
	for i := len(events) - 1; i >= 0; i-- {
		msg, err := eventToMessage(events[i])
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
		usedTokens += msgTokens
		included++
	}

	return &ContextSummary{
		MaxTokens:         e.maxTokens,
		Reserve:           e.reserve,
		InputBudget:       inputBudget,
		SystemPromptTokens: sysTokens,
		SystemPromptText:  sysPrompt,
		EventBudget:       eventBudget,
		EventTokensUsed:   usedTokens,
		EventsIncluded:    included,
		EventsTotal:       len(events),
		BudgetRemaining:   inputBudget - sysTokens - usedTokens,
	}
}

// loadPromptTemplate loads the system prompt template from a file, or returns
// the built-in default if the path is empty or the file doesn't exist.
func loadPromptTemplate(path string) (*template.Template, error) {
	if path != "" {
		if data, err := os.ReadFile(path); err == nil {
			tmpl, err := template.New("system").Parse(string(data))
			if err != nil {
				return nil, fmt.Errorf("parse prompt template %s: %w", path, err)
			}
			slog.Info("loaded system prompt", "path", path)
			return tmpl, nil
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("read prompt file %s: %w", path, err)
		}
		// File doesn't exist — fall through to default
		slog.Info("system prompt file not found, using default", "path", path)
	}

	tmpl, err := template.New("system").Parse(DefaultPrompt)
	if err != nil {
		return nil, fmt.Errorf("parse default prompt: %w", err)
	}
	return tmpl, nil
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
