// internal/state/event.go
package state

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/user/gopherclaw/internal/types"
)

// EventStore is a JSONL-backed append-only event store.
// Events are stored per-session in sessions/<sessionID>/events.jsonl.
type EventStore struct {
	root  string
	mu    sync.Mutex
	locks map[types.SessionID]*sync.Mutex
}

// NewEventStore creates a new file-backed EventStore rooted at the given directory.
func NewEventStore(root string) *EventStore {
	return &EventStore{
		root:  root,
		locks: make(map[types.SessionID]*sync.Mutex),
	}
}

// getLock returns the per-session mutex, creating one if it doesn't exist.
func (e *EventStore) getLock(sessionID types.SessionID) *sync.Mutex {
	e.mu.Lock()
	defer e.mu.Unlock()

	if lock, ok := e.locks[sessionID]; ok {
		return lock
	}
	lock := &sync.Mutex{}
	e.locks[sessionID] = lock
	return lock
}

func (e *EventStore) eventsPath(sessionID types.SessionID) string {
	return filepath.Join(e.root, "sessions", string(sessionID), "events.jsonl")
}

// count reads the event file and counts lines. Caller must hold the session lock.
func (e *EventStore) count(sessionID types.SessionID) (int64, error) {
	f, err := os.Open(e.eventsPath(sessionID))
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("open events file: %w", err)
	}
	defer f.Close()

	var count int64
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		count++
	}
	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("scan events file: %w", err)
	}
	return count, nil
}

// Append adds an event to the session's event log with an auto-incremented sequence number.
func (e *EventStore) Append(_ context.Context, event *types.Event) error {
	lock := e.getLock(event.SessionID)
	lock.Lock()
	defer lock.Unlock()

	// Ensure the session directory exists
	dir := filepath.Dir(e.eventsPath(event.SessionID))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create session dir: %w", err)
	}

	// Count existing events to determine sequence number
	existing, err := e.count(event.SessionID)
	if err != nil {
		return err
	}
	event.Seq = existing + 1

	// Marshal the event to JSON
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	// Append to the events file
	f, err := os.OpenFile(e.eventsPath(event.SessionID), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open events file: %w", err)
	}
	defer f.Close()

	data = append(data, '\n')
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("write event: %w", err)
	}

	return nil
}

// Tail returns the last N events for the given session.
func (e *EventStore) Tail(_ context.Context, sessionID types.SessionID, limit int) ([]*types.Event, error) {
	lock := e.getLock(sessionID)
	lock.Lock()
	defer lock.Unlock()

	f, err := os.Open(e.eventsPath(sessionID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open events file: %w", err)
	}
	defer f.Close()

	var events []*types.Event
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var event types.Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return nil, fmt.Errorf("unmarshal event: %w", err)
		}
		events = append(events, &event)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan events file: %w", err)
	}

	// Return last N events
	if len(events) > limit {
		events = events[len(events)-limit:]
	}

	return events, nil
}

// Count returns the number of events for the given session.
func (e *EventStore) Count(_ context.Context, sessionID types.SessionID) (int64, error) {
	lock := e.getLock(sessionID)
	lock.Lock()
	defer lock.Unlock()

	return e.count(sessionID)
}
