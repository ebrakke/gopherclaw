package gateway

import (
	"time"

	"github.com/user/gopherclaw/internal/types"
)

// RunStatus represents the lifecycle state of a Run.
type RunStatus string

const (
	RunStatusQueued   RunStatus = "queued"
	RunStatusRunning  RunStatus = "running"
	RunStatusComplete RunStatus = "complete"
	RunStatusFailed   RunStatus = "failed"
)

// Run tracks a single execution of an inbound event against a session.
type Run struct {
	ID         types.RunID
	SessionID  types.SessionID
	Event      *types.InboundEvent
	Status     RunStatus
	Attempts   int
	CreatedAt  time.Time
	StartedAt  *time.Time
	EndedAt    *time.Time
	Error      error
	OnComplete func(response string)
}

// NewRun creates a Run in the Queued state for the given session and event.
func NewRun(sessionID types.SessionID, event *types.InboundEvent) *Run {
	return &Run{
		ID:        types.NewRunID(),
		SessionID: sessionID,
		Event:     event,
		Status:    RunStatusQueued,
		Attempts:  0,
		CreatedAt: time.Now(),
	}
}
