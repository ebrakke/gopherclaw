// internal/state/event_test.go
package state

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/user/gopherclaw/internal/types"
)

func TestEventStore(t *testing.T) {
	dir := t.TempDir()
	store := NewEventStore(dir)
	ctx := context.Background()

	sessionID := types.NewSessionID()

	// Test append
	event1 := &types.Event{
		ID:        types.NewEventID(),
		SessionID: sessionID,
		RunID:     types.NewRunID(),
		Seq:       0, // Will be auto-assigned
		Type:      "user_message",
		Source:    "test",
		At:        time.Now(),
		Payload:   json.RawMessage(`{"text":"hello"}`),
	}

	if err := store.Append(ctx, event1); err != nil {
		t.Fatal(err)
	}

	// Test tail
	events, err := store.Tail(ctx, sessionID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
	if events[0].Seq != 1 {
		t.Errorf("expected seq 1, got %d", events[0].Seq)
	}

	// Test count
	count, err := store.Count(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("expected count 1, got %d", count)
	}
}
