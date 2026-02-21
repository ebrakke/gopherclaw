// internal/types/models_test.go
package types

import (
	"encoding/json"
	"testing"
	"time"
)

func TestEventSerialization(t *testing.T) {
	event := Event{
		ID:        NewEventID(),
		SessionID: NewSessionID(),
		RunID:     NewRunID(),
		Seq:       1,
		Type:      "user_message",
		Source:    "telegram",
		At:        time.Now(),
		Payload:   json.RawMessage(`{"text":"hello"}`),
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}

	var decoded Event
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}

	if decoded.Type != event.Type {
		t.Errorf("expected type %s, got %s", event.Type, decoded.Type)
	}
}
