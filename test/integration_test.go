//go:build integration

package test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/user/gopherclaw/internal/gateway"
	"github.com/user/gopherclaw/internal/state"
	"github.com/user/gopherclaw/internal/types"
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
