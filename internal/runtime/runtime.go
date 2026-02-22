package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	ctxengine "github.com/user/gopherclaw/internal/context"
	"github.com/user/gopherclaw/internal/gateway"
	"github.com/user/gopherclaw/internal/types"
	"github.com/user/gopherclaw/pkg/llm"
)

// Runtime implements the agentic turn loop.
type Runtime struct {
	provider  llm.Provider
	engine    *ctxengine.Engine
	sessions  types.SessionStore
	events    types.EventStore
	artifacts types.ArtifactStore
	registry  *Registry
	maxRounds int
}

// New creates a Runtime with the given dependencies.
func New(
	provider llm.Provider,
	engine *ctxengine.Engine,
	sessions types.SessionStore,
	events types.EventStore,
	artifacts types.ArtifactStore,
	registry *Registry,
	maxRounds int,
) *Runtime {
	return &Runtime{
		provider:  provider,
		engine:    engine,
		sessions:  sessions,
		events:    events,
		artifacts: artifacts,
		registry:  registry,
		maxRounds: maxRounds,
	}
}

const artifactThreshold = 2000

// ProcessRun executes the agentic turn loop for a single run.
// This is the function passed to Queue.SetProcessor.
func (rt *Runtime) ProcessRun(run *gateway.Run) error {
	ctx := run.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	// 1. Record user_message event
	userPayload, _ := json.Marshal(map[string]string{"text": run.Event.Text})
	if err := rt.events.Append(ctx, &types.Event{
		ID:        types.NewEventID(),
		SessionID: run.SessionID,
		RunID:     run.ID,
		Type:      "user_message",
		Source:    run.Event.Source,
		At:        time.Now(),
		Payload:   userPayload,
	}); err != nil {
		return fmt.Errorf("record user message: %w", err)
	}

	// Collect tool names for system prompt
	var toolNames []string
	for _, t := range rt.registry.All() {
		toolNames = append(toolNames, t.Name())
	}

	for round := 0; round < rt.maxRounds; round++ {
		// 2. Load session
		session, err := rt.sessions.Get(ctx, run.SessionID)
		if err != nil {
			return fmt.Errorf("load session: %w", err)
		}

		// 3. Load recent events
		events, err := rt.events.Tail(ctx, run.SessionID, 100)
		if err != nil {
			return fmt.Errorf("load events: %w", err)
		}

		// 4. Build prompt
		messages, err := rt.engine.BuildPrompt(ctx, session, events, rt.artifacts, toolNames)
		if err != nil {
			return fmt.Errorf("build prompt: %w", err)
		}

		// 5. Call LLM
		resp, err := rt.provider.Complete(ctx, messages, rt.registry.AsLLMTools())
		if err != nil {
			return fmt.Errorf("LLM call: %w", err)
		}

		// 6. If tool calls, execute them
		if len(resp.ToolCalls) > 0 {
			for _, tc := range resp.ToolCalls {
				// Record tool_call event
				tcPayload, _ := json.Marshal(map[string]any{
					"tool":      tc.Function.Name,
					"call_id":   tc.ID,
					"arguments": tc.Function.Arguments,
				})
				if err := rt.events.Append(ctx, &types.Event{
					ID:        types.NewEventID(),
					SessionID: run.SessionID,
					RunID:     run.ID,
					Type:      "tool_call",
					Source:    "runtime",
					At:        time.Now(),
					Payload:   tcPayload,
				}); err != nil {
					return fmt.Errorf("record tool call: %w", err)
				}

				// Execute tool
				tool, ok := rt.registry.Get(tc.Function.Name)
				var result string
				if !ok {
					result = fmt.Sprintf("error: unknown tool %q", tc.Function.Name)
				} else {
					var execErr error
					result, execErr = tool.Execute(ctx, tc.Function.Arguments)
					if execErr != nil {
						result = fmt.Sprintf("error: %v", execErr)
					}
				}

				// Store as artifact if large
				trPayload := map[string]any{
					"tool":    tc.Function.Name,
					"call_id": tc.ID,
					"result":  result,
				}
				if len(result) > artifactThreshold {
					artID, err := rt.artifacts.Put(ctx, run.SessionID, run.ID, tc.Function.Name, result)
					if err == nil {
						trPayload["artifact_id"] = string(artID)
						trPayload["result"] = result[:artifactThreshold] + "\n[truncated, see artifact " + string(artID) + "]"
					}
				}

				trPayloadJSON, _ := json.Marshal(trPayload)
				if err := rt.events.Append(ctx, &types.Event{
					ID:        types.NewEventID(),
					SessionID: run.SessionID,
					RunID:     run.ID,
					Type:      "tool_result",
					Source:    "runtime",
					At:        time.Now(),
					Payload:   trPayloadJSON,
				}); err != nil {
					return fmt.Errorf("record tool result: %w", err)
				}
			}
			continue // Loop back for next LLM call
		}

		// 7. Text response -- done
		if resp.Content != "" {
			aPayload, _ := json.Marshal(map[string]string{"text": resp.Content})
			if err := rt.events.Append(ctx, &types.Event{
				ID:        types.NewEventID(),
				SessionID: run.SessionID,
				RunID:     run.ID,
				Type:      "assistant_message",
				Source:    "runtime",
				At:        time.Now(),
				Payload:   aPayload,
			}); err != nil {
				return fmt.Errorf("record assistant message: %w", err)
			}
			if run.OnComplete != nil {
				run.OnComplete(resp.Content)
			}
			return nil
		}

		// Empty response (no content, no tool calls) -- treat as done
		if run.OnComplete != nil {
			run.OnComplete("")
		}
		return nil
	}

	errPayload, _ := json.Marshal(map[string]string{"error": fmt.Sprintf("max tool rounds (%d) exceeded", rt.maxRounds)})
	rt.events.Append(ctx, &types.Event{
		ID:        types.NewEventID(),
		SessionID: run.SessionID,
		RunID:     run.ID,
		Type:      "error",
		Source:    "runtime",
		At:        time.Now(),
		Payload:   errPayload,
	})
	return fmt.Errorf("max tool rounds (%d) exceeded", rt.maxRounds)
}
