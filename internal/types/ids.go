// internal/types/ids.go
package types

import (
	"strings"

	"github.com/google/uuid"
)

type SessionKey string
type SessionID string
type RunID string
type EventID string
type ArtifactID string
type AutomationID string

func NewSessionID() SessionID {
	return SessionID(uuid.New().String())
}

func NewRunID() RunID {
	return RunID(uuid.New().String())
}

func NewEventID() EventID {
	return EventID(uuid.New().String())
}

func NewArtifactID() ArtifactID {
	return ArtifactID(uuid.New().String())
}

func NewAutomationID() AutomationID {
	return AutomationID(uuid.New().String())
}

func NewSessionKey(parts ...string) SessionKey {
	return SessionKey(strings.Join(parts, ":"))
}
