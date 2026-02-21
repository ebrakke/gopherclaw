// internal/types/interfaces.go
package types

import (
	"context"
	"encoding/json"
)

type SessionStore interface {
	ResolveOrCreate(ctx context.Context, key SessionKey, agent string) (SessionID, error)
	Get(ctx context.Context, id SessionID) (*SessionIndex, error)
	List(ctx context.Context) ([]*SessionIndex, error)
	Update(ctx context.Context, session *SessionIndex) error
}

type EventStore interface {
	Append(ctx context.Context, event *Event) error
	Tail(ctx context.Context, sessionID SessionID, limit int) ([]*Event, error)
	Count(ctx context.Context, sessionID SessionID) (int64, error)
}

type ArtifactStore interface {
	Put(ctx context.Context, sessionID SessionID, runID RunID, tool string, data any) (ArtifactID, error)
	Get(ctx context.Context, id ArtifactID) (json.RawMessage, error)
	GetMeta(ctx context.Context, id ArtifactID) (*ArtifactMeta, error)
	Excerpt(ctx context.Context, id ArtifactID, query string, maxTokens int) (string, error)
}
