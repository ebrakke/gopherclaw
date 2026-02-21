// internal/types/models.go
package types

import (
	"encoding/json"
	"time"
)

type Event struct {
	ID        EventID         `json:"id"`
	SessionID SessionID       `json:"session_id"`
	RunID     RunID           `json:"run_id,omitempty"`
	Seq       int64           `json:"seq"`
	Type      string          `json:"type"`
	Source    string          `json:"source"`
	At        time.Time       `json:"at"`
	Payload   json.RawMessage `json:"payload"`
}

type SessionIndex struct {
	SessionID    SessionID  `json:"session_id"`
	SessionKey   SessionKey `json:"session_key"`
	Agent        string     `json:"agent"`
	Status       string     `json:"status"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	LastRunID    RunID      `json:"last_run_id,omitempty"`
	LastEventSeq int64      `json:"last_event_seq"`
}

type ArtifactMeta struct {
	ID        ArtifactID `json:"id"`
	SessionID SessionID  `json:"session_id"`
	RunID     RunID      `json:"run_id"`
	Tool      string     `json:"tool"`
	CreatedAt time.Time  `json:"created_at"`
	MimeType  string     `json:"mime_type,omitempty"`
}

type InboundEvent struct {
	Source     string          `json:"source"`
	SessionKey SessionKey     `json:"session_key"`
	UserID     string         `json:"user_id"`
	Text       string         `json:"text"`
	Metadata   json.RawMessage `json:"metadata,omitempty"`
}
