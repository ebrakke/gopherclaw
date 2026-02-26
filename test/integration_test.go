//go:build integration

package test

import (
	"context"
	"fmt"
	"testing"
	"time"

	ctxengine "github.com/user/gopherclaw/internal/context"
	"github.com/user/gopherclaw/internal/gateway"
	"github.com/user/gopherclaw/internal/runtime"
	"github.com/user/gopherclaw/internal/state"
	"github.com/user/gopherclaw/internal/types"
	"github.com/user/gopherclaw/pkg/llm"
)

func TestEndToEnd(t *testing.T) {
	dir := t.TempDir()

	sessions := state.NewSessionStore(dir)
	events := state.NewEventStore(dir)
	artifacts := state.NewArtifactStore(dir)

	gw := gateway.New(sessions, events, artifacts)

	ctx := context.Background()
	gw.Start(ctx)
	defer gw.Stop()

	// Configure queue processor to record events
	gw.Queue.SetProcessor(func(run *gateway.Run) error {
		time.Sleep(10 * time.Millisecond)

		event := &types.Event{
			ID:        types.NewEventID(),
			SessionID: run.SessionID,
			RunID:     run.ID,
			Type:      "assistant_message",
			Source:    "system",
			At:        time.Now(),
		}
		return events.Append(ctx, event)
	})

	// Send multiple messages from same user
	for i := 0; i < 3; i++ {
		inbound := &types.InboundEvent{
			Source:     "test",
			SessionKey: types.NewSessionKey("test", "user1"),
			UserID:     "user1",
			Text:       fmt.Sprintf("message %d", i),
		}

		if err := gw.HandleInbound(ctx, inbound); err != nil {
			t.Fatal(err)
		}
	}

	// Wait for processing
	time.Sleep(200 * time.Millisecond)

	// Verify session was created
	sessionList, err := sessions.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessionList) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessionList))
	}

	// Verify events were recorded
	eventList, err := events.Tail(ctx, sessionList[0].SessionID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(eventList) != 3 {
		t.Errorf("expected 3 events, got %d", len(eventList))
	}

	// Verify sequential processing (FIFO ordering)
	for i, event := range eventList {
		if event.Seq != int64(i+1) {
			t.Errorf("expected seq %d, got %d", i+1, event.Seq)
		}
	}
}

// mockProvider is a test double that returns a canned LLM response.
type mockProvider struct {
	response *llm.Response
}

func (m *mockProvider) Complete(_ context.Context, _ []llm.Message, _ []llm.Tool) (*llm.Response, error) {
	return m.response, nil
}

func (m *mockProvider) Stream(_ context.Context, _ []llm.Message, _ []llm.Tool) (<-chan llm.Delta, error) {
	return nil, nil
}

func TestEndToEndWithRuntime(t *testing.T) {
	dir := t.TempDir()

	sessions := state.NewSessionStore(dir)
	events := state.NewEventStore(dir)
	artifacts := state.NewArtifactStore(dir)

	provider := &mockProvider{
		response: &llm.Response{Content: "Hello from the LLM!"},
	}

	engine, err := ctxengine.New("gpt-4", 128000, 4096, "")
	if err != nil {
		t.Fatal(err)
	}

	registry := runtime.NewRegistry()
	rt := runtime.New(provider, engine, sessions, events, artifacts, registry, 10)

	gw := gateway.New(sessions, events, artifacts)
	gw.Queue.SetProcessor(rt.ProcessRun)

	ctx := context.Background()
	gw.Start(ctx)
	defer gw.Stop()

	var response string
	done := make(chan struct{})

	inbound := &types.InboundEvent{
		Source:     "test",
		SessionKey: types.NewSessionKey("test", "user1"),
		UserID:     "user1",
		Text:       "hello",
	}

	err = gw.HandleInbound(ctx, inbound, gateway.WithOnComplete(func(resp string) {
		response = resp
		close(done)
	}))
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for response")
	}

	if response != "Hello from the LLM!" {
		t.Errorf("expected 'Hello from the LLM!', got %q", response)
	}

	sessionList, err := sessions.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	eventCount, err := events.Count(ctx, sessionList[0].SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if eventCount != 2 {
		t.Errorf("expected 2 events, got %d", eventCount)
	}
}
