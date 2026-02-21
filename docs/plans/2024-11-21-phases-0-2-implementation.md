# Gopherclaw Phases 0-2 Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build the foundation of gopherclaw with core types, storage layer, and gateway orchestration.

**Architecture:** Domain-driven Go packages with filesystem-based storage and concurrent session handling via gateway/queue pattern.

**Tech Stack:** Go 1.21+, OpenAI-compatible API client, filesystem storage with JSONL events

---

## Task 1: Project Setup

**Files:**
- Create: `go.mod`
- Create: `Makefile`
- Create: `.gitignore`

**Step 1: Initialize Go module**

```bash
go mod init github.com/user/gopherclaw
```

**Step 2: Create Makefile**

```makefile
# Makefile
.PHONY: build test run clean

build:
	go build -o bin/gopherclaw cmd/gopherclaw/main.go

test:
	go test -v ./...

run:
	go run cmd/gopherclaw/main.go

clean:
	rm -rf bin/
```

**Step 3: Create .gitignore**

```gitignore
# .gitignore
bin/
*.exe
*.test
*.prof
.env
```

**Step 4: Commit**

```bash
git add go.mod Makefile .gitignore
git commit -m "chore: initialize Go module and build setup"
```

---

## Task 2: Core Types and IDs

**Files:**
- Create: `internal/types/ids.go`
- Create: `internal/types/ids_test.go`

**Step 1: Write the failing test**

```go
// internal/types/ids_test.go
package types

import (
	"testing"
)

func TestNewSessionID(t *testing.T) {
	id := NewSessionID()
	if id == "" {
		t.Error("expected non-empty SessionID")
	}
	if len(string(id)) != 36 { // UUID length
		t.Errorf("expected UUID format, got %s", id)
	}
}

func TestSessionKeyFormat(t *testing.T) {
	key := NewSessionKey("telegram", "123", "456")
	expected := SessionKey("telegram:123:456")
	if key != expected {
		t.Errorf("expected %s, got %s", expected, key)
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/types -v
```
Expected: FAIL with "undefined: NewSessionID"

**Step 3: Write minimal implementation**

```go
// internal/types/ids.go
package types

import (
	"fmt"
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
```

**Step 4: Add UUID dependency**

```bash
go get github.com/google/uuid
```

**Step 5: Run test to verify it passes**

```bash
go test ./internal/types -v
```
Expected: PASS

**Step 6: Commit**

```bash
git add internal/types/
git commit -m "feat: add core ID types with UUID generation"
```

---

## Task 3: Core Models

**Files:**
- Create: `internal/types/models.go`
- Create: `internal/types/models_test.go`

**Step 1: Write the failing test**

```go
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
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/types -v
```
Expected: FAIL with "undefined: Event"

**Step 3: Write minimal implementation**

```go
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
	SessionKey SessionKey      `json:"session_key"`
	UserID     string          `json:"user_id"`
	Text       string          `json:"text"`
	Metadata   json.RawMessage `json:"metadata,omitempty"`
}
```

**Step 4: Run test to verify it passes**

```bash
go test ./internal/types -v
```
Expected: PASS

**Step 5: Commit**

```bash
git add internal/types/
git commit -m "feat: add core data models"
```

---

## Task 4: Storage Interfaces

**Files:**
- Create: `internal/types/interfaces.go`
- Create: `internal/types/interfaces_test.go`

**Step 1: Write the interfaces**

```go
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
```

**Step 2: Write interface compliance test**

```go
// internal/types/interfaces_test.go
package types

import "testing"

func TestInterfaceCompilation(t *testing.T) {
	// This test just ensures interfaces compile
	var _ SessionStore
	var _ EventStore
	var _ ArtifactStore
}
```

**Step 3: Run test**

```bash
go test ./internal/types -v
```
Expected: PASS

**Step 4: Commit**

```bash
git add internal/types/
git commit -m "feat: add storage interfaces"
```

---

## Task 5: Session Store Implementation

**Files:**
- Create: `internal/state/session.go`
- Create: `internal/state/session_test.go`

**Step 1: Write the failing test**

```go
// internal/state/session_test.go
package state

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/user/gopherclaw/internal/types"
)

func TestSessionStore(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStore(dir)
	ctx := context.Background()

	// Test resolve or create
	key := types.NewSessionKey("test", "123")
	id, err := store.ResolveOrCreate(ctx, key, "default")
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Error("expected non-empty session ID")
	}

	// Test get
	session, err := store.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if session.SessionKey != key {
		t.Errorf("expected key %s, got %s", key, session.SessionKey)
	}

	// Test idempotency
	id2, err := store.ResolveOrCreate(ctx, key, "default")
	if err != nil {
		t.Fatal(err)
	}
	if id != id2 {
		t.Error("expected same session ID for same key")
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/state -v
```
Expected: FAIL with "undefined: NewSessionStore"

**Step 3: Write minimal implementation**

```go
// internal/state/session.go
package state

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/user/gopherclaw/internal/types"
)

type SessionStore struct {
	dir string
	mu  sync.RWMutex
}

func NewSessionStore(dir string) *SessionStore {
	return &SessionStore{dir: dir}
}

func (s *SessionStore) indexPath() string {
	return filepath.Join(s.dir, "sessions", "sessions.json")
}

func (s *SessionStore) loadIndex() (map[types.SessionKey]*types.SessionIndex, error) {
	data, err := os.ReadFile(s.indexPath())
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[types.SessionKey]*types.SessionIndex), nil
		}
		return nil, err
	}

	var sessions []*types.SessionIndex
	if err := json.Unmarshal(data, &sessions); err != nil {
		return nil, err
	}

	index := make(map[types.SessionKey]*types.SessionIndex)
	for _, session := range sessions {
		index[session.SessionKey] = session
	}
	return index, nil
}

func (s *SessionStore) saveIndex(index map[types.SessionKey]*types.SessionIndex) error {
	// Convert map to slice
	var sessions []*types.SessionIndex
	for _, session := range index {
		sessions = append(sessions, session)
	}

	data, err := json.MarshalIndent(sessions, "", "  ")
	if err != nil {
		return err
	}

	// Ensure directory exists
	dir := filepath.Dir(s.indexPath())
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Atomic write via temp file
	tmpPath := s.indexPath() + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmpPath, s.indexPath())
}

func (s *SessionStore) ResolveOrCreate(ctx context.Context, key types.SessionKey, agent string) (types.SessionID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	index, err := s.loadIndex()
	if err != nil {
		return "", err
	}

	// Check if exists
	if session, exists := index[key]; exists {
		return session.SessionID, nil
	}

	// Create new
	now := time.Now()
	session := &types.SessionIndex{
		SessionID:    types.NewSessionID(),
		SessionKey:   key,
		Agent:        agent,
		Status:       "active",
		CreatedAt:    now,
		UpdatedAt:    now,
		LastEventSeq: 0,
	}

	// Create session directory
	sessionDir := filepath.Join(s.dir, "sessions", string(session.SessionID))
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		return "", err
	}

	// Update index
	index[key] = session
	if err := s.saveIndex(index); err != nil {
		return "", err
	}

	return session.SessionID, nil
}

func (s *SessionStore) Get(ctx context.Context, id types.SessionID) (*types.SessionIndex, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	index, err := s.loadIndex()
	if err != nil {
		return nil, err
	}

	for _, session := range index {
		if session.SessionID == id {
			return session, nil
		}
	}

	return nil, fmt.Errorf("session not found: %s", id)
}

func (s *SessionStore) List(ctx context.Context) ([]*types.SessionIndex, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	index, err := s.loadIndex()
	if err != nil {
		return nil, err
	}

	var sessions []*types.SessionIndex
	for _, session := range index {
		sessions = append(sessions, session)
	}
	return sessions, nil
}

func (s *SessionStore) Update(ctx context.Context, session *types.SessionIndex) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	index, err := s.loadIndex()
	if err != nil {
		return err
	}

	session.UpdatedAt = time.Now()
	index[session.SessionKey] = session
	return s.saveIndex(index)
}
```

**Step 4: Run test to verify it passes**

```bash
go test ./internal/state -v
```
Expected: PASS

**Step 5: Commit**

```bash
git add internal/state/
git commit -m "feat: add session store implementation"
```

---

## Task 6: Event Store Implementation

**Files:**
- Create: `internal/state/event.go`
- Create: `internal/state/event_test.go`

**Step 1: Write the failing test**

```go
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
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/state -v
```
Expected: FAIL with "undefined: NewEventStore"

**Step 3: Write minimal implementation**

```go
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

type EventStore struct {
	dir   string
	locks map[types.SessionID]*sync.Mutex
	mu    sync.RWMutex
}

func NewEventStore(dir string) *EventStore {
	return &EventStore{
		dir:   dir,
		locks: make(map[types.SessionID]*sync.Mutex),
	}
}

func (s *EventStore) getLock(sessionID types.SessionID) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()

	if lock, exists := s.locks[sessionID]; exists {
		return lock
	}

	lock := &sync.Mutex{}
	s.locks[sessionID] = lock
	return lock
}

func (s *EventStore) eventPath(sessionID types.SessionID) string {
	return filepath.Join(s.dir, "sessions", string(sessionID), "events.jsonl")
}

func (s *EventStore) Append(ctx context.Context, event *types.Event) error {
	lock := s.getLock(event.SessionID)
	lock.Lock()
	defer lock.Unlock()

	// Ensure directory exists
	dir := filepath.Dir(s.eventPath(event.SessionID))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Auto-assign sequence number
	count, _ := s.count(event.SessionID)
	event.Seq = count + 1

	// Open file for append
	file, err := os.OpenFile(s.eventPath(event.SessionID), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	// Write event as JSONL
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	_, err = file.Write(data)
	return err
}

func (s *EventStore) Tail(ctx context.Context, sessionID types.SessionID, limit int) ([]*types.Event, error) {
	lock := s.getLock(sessionID)
	lock.Lock()
	defer lock.Unlock()

	path := s.eventPath(sessionID)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return []*types.Event{}, nil
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var events []*types.Event
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var event types.Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			continue // Skip malformed lines
		}
		events = append(events, &event)
	}

	// Return tail
	if len(events) > limit {
		return events[len(events)-limit:], nil
	}
	return events, nil
}

func (s *EventStore) count(sessionID types.SessionID) (int64, error) {
	path := s.eventPath(sessionID)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return 0, nil
	}

	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	var count int64
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		count++
	}
	return count, scanner.Err()
}

func (s *EventStore) Count(ctx context.Context, sessionID types.SessionID) (int64, error) {
	lock := s.getLock(sessionID)
	lock.Lock()
	defer lock.Unlock()

	return s.count(sessionID)
}
```

**Step 4: Run test to verify it passes**

```bash
go test ./internal/state -v
```
Expected: PASS

**Step 5: Commit**

```bash
git add internal/state/
git commit -m "feat: add event store implementation"
```

---

## Task 7: Artifact Store Implementation

**Files:**
- Create: `internal/state/artifact.go`
- Create: `internal/state/artifact_test.go`

**Step 1: Write the failing test**

```go
// internal/state/artifact_test.go
package state

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/user/gopherclaw/internal/types"
)

func TestArtifactStore(t *testing.T) {
	dir := t.TempDir()
	store := NewArtifactStore(dir)
	ctx := context.Background()

	sessionID := types.NewSessionID()
	runID := types.NewRunID()

	// Test put
	data := map[string]any{
		"output": "test result",
		"lines":  []string{"line1", "line2"},
	}

	artifactID, err := store.Put(ctx, sessionID, runID, "test-tool", data)
	if err != nil {
		t.Fatal(err)
	}
	if artifactID == "" {
		t.Error("expected non-empty artifact ID")
	}

	// Test get
	raw, err := store.Get(ctx, artifactID)
	if err != nil {
		t.Fatal(err)
	}

	var retrieved map[string]any
	if err := json.Unmarshal(raw, &retrieved); err != nil {
		t.Fatal(err)
	}
	if retrieved["output"] != "test result" {
		t.Error("data mismatch")
	}

	// Test get meta
	meta, err := store.GetMeta(ctx, artifactID)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Tool != "test-tool" {
		t.Errorf("expected tool test-tool, got %s", meta.Tool)
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/state -v
```
Expected: FAIL with "undefined: NewArtifactStore"

**Step 3: Write minimal implementation**

```go
// internal/state/artifact.go
package state

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/user/gopherclaw/internal/types"
)

type ArtifactStore struct {
	dir string
}

func NewArtifactStore(dir string) *ArtifactStore {
	return &ArtifactStore{dir: dir}
}

func (s *ArtifactStore) artifactPath(sessionID types.SessionID, artifactID types.ArtifactID) string {
	return filepath.Join(s.dir, "sessions", string(sessionID), "artifacts", string(artifactID)+".json")
}

func (s *ArtifactStore) Put(ctx context.Context, sessionID types.SessionID, runID types.RunID, tool string, data any) (types.ArtifactID, error) {
	artifactID := types.NewArtifactID()

	// Create artifact metadata
	meta := &types.ArtifactMeta{
		ID:        artifactID,
		SessionID: sessionID,
		RunID:     runID,
		Tool:      tool,
		CreatedAt: time.Now(),
		MimeType:  "application/json",
	}

	// Create wrapper with metadata and data
	wrapper := map[string]any{
		"meta": meta,
		"data": data,
	}

	// Ensure directory exists
	path := s.artifactPath(sessionID, artifactID)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}

	// Marshal and save
	content, err := json.MarshalIndent(wrapper, "", "  ")
	if err != nil {
		return "", err
	}

	// Atomic write
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, content, 0644); err != nil {
		return "", err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return "", err
	}

	return artifactID, nil
}

func (s *ArtifactStore) Get(ctx context.Context, id types.ArtifactID) (json.RawMessage, error) {
	// Find artifact file
	pattern := filepath.Join(s.dir, "sessions", "*", "artifacts", string(id)+".json")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("artifact not found: %s", id)
	}

	// Read file
	content, err := os.ReadFile(matches[0])
	if err != nil {
		return nil, err
	}

	// Extract data portion
	var wrapper map[string]json.RawMessage
	if err := json.Unmarshal(content, &wrapper); err != nil {
		return nil, err
	}

	return wrapper["data"], nil
}

func (s *ArtifactStore) GetMeta(ctx context.Context, id types.ArtifactID) (*types.ArtifactMeta, error) {
	// Find artifact file
	pattern := filepath.Join(s.dir, "sessions", "*", "artifacts", string(id)+".json")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("artifact not found: %s", id)
	}

	// Read file
	content, err := os.ReadFile(matches[0])
	if err != nil {
		return nil, err
	}

	// Extract meta portion
	var wrapper struct {
		Meta *types.ArtifactMeta `json:"meta"`
	}
	if err := json.Unmarshal(content, &wrapper); err != nil {
		return nil, err
	}

	return wrapper.Meta, nil
}

func (s *ArtifactStore) Excerpt(ctx context.Context, id types.ArtifactID, query string, maxTokens int) (string, error) {
	raw, err := s.Get(ctx, id)
	if err != nil {
		return "", err
	}

	// For now, just return first N characters
	// TODO: Implement smart extraction based on query
	content := string(raw)

	// Rough token estimation (4 chars per token)
	maxChars := maxTokens * 4
	if len(content) > maxChars {
		content = content[:maxChars] + "..."
	}

	// If query provided, try to find relevant portion
	if query != "" {
		idx := strings.Index(strings.ToLower(content), strings.ToLower(query))
		if idx >= 0 {
			start := idx - 100
			if start < 0 {
				start = 0
			}
			end := idx + maxChars - 100
			if end > len(content) {
				end = len(content)
			}
			return "..." + content[start:end] + "...", nil
		}
	}

	return content, nil
}
```

**Step 4: Run test to verify it passes**

```bash
go test ./internal/state -v
```
Expected: PASS

**Step 5: Commit**

```bash
git add internal/state/
git commit -m "feat: add artifact store implementation"
```

---

## Task 8: Gateway Core Structure

**Files:**
- Create: `internal/gateway/gateway.go`
- Create: `internal/gateway/gateway_test.go`
- Create: `internal/gateway/run.go`

**Step 1: Write run model**

```go
// internal/gateway/run.go
package gateway

import (
	"time"

	"github.com/user/gopherclaw/internal/types"
)

type RunStatus string

const (
	RunStatusQueued   RunStatus = "queued"
	RunStatusRunning  RunStatus = "running"
	RunStatusComplete RunStatus = "complete"
	RunStatusFailed   RunStatus = "failed"
)

type Run struct {
	ID        types.RunID
	SessionID types.SessionID
	Event     *types.InboundEvent
	Status    RunStatus
	Attempts  int
	CreatedAt time.Time
	StartedAt *time.Time
	EndedAt   *time.Time
	Error     error
}

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
```

**Step 2: Write the failing test**

```go
// internal/gateway/gateway_test.go
package gateway

import (
	"context"
	"testing"
	"time"

	"github.com/user/gopherclaw/internal/state"
	"github.com/user/gopherclaw/internal/types"
)

func TestGatewayHandleInbound(t *testing.T) {
	dir := t.TempDir()

	// Create stores
	sessions := state.NewSessionStore(dir)
	events := state.NewEventStore(dir)
	artifacts := state.NewArtifactStore(dir)

	// Create gateway
	gw := New(sessions, events, artifacts)
	ctx := context.Background()

	// Start gateway
	go gw.Start(ctx)
	defer gw.Stop()

	// Send inbound event
	inbound := &types.InboundEvent{
		Source:     "test",
		SessionKey: types.NewSessionKey("test", "123"),
		UserID:     "user1",
		Text:       "hello",
	}

	err := gw.HandleInbound(ctx, inbound)
	if err != nil {
		t.Fatal(err)
	}

	// Give it time to process
	time.Sleep(100 * time.Millisecond)

	// Verify session was created
	sessions2, err := sessions.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions2) != 1 {
		t.Errorf("expected 1 session, got %d", len(sessions2))
	}
}
```

**Step 3: Run test to verify it fails**

```bash
go test ./internal/gateway -v
```
Expected: FAIL with "undefined: New"

**Step 4: Write minimal implementation**

```go
// internal/gateway/gateway.go
package gateway

import (
	"context"
	"fmt"
	"sync"

	"github.com/user/gopherclaw/internal/types"
)

type Gateway struct {
	sessions  types.SessionStore
	events    types.EventStore
	artifacts types.ArtifactStore
	queue     *Queue
	retry     *RetryPolicy

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func New(sessions types.SessionStore, events types.EventStore, artifacts types.ArtifactStore) *Gateway {
	return &Gateway{
		sessions:  sessions,
		events:    events,
		artifacts: artifacts,
		queue:     NewQueue(2), // Max 2 concurrent runs
		retry:     DefaultRetryPolicy(),
	}
}

func (g *Gateway) Start(ctx context.Context) {
	g.ctx, g.cancel = context.WithCancel(ctx)
	g.queue.Start(g.ctx)
}

func (g *Gateway) Stop() {
	if g.cancel != nil {
		g.cancel()
	}
	g.queue.Stop()
	g.wg.Wait()
}

func (g *Gateway) HandleInbound(ctx context.Context, event *types.InboundEvent) error {
	// Resolve or create session
	sessionID, err := g.sessions.ResolveOrCreate(ctx, event.SessionKey, "default")
	if err != nil {
		return fmt.Errorf("resolve session: %w", err)
	}

	// Create run
	run := NewRun(sessionID, event)

	// Enqueue for processing
	return g.queue.Enqueue(run)
}
```

**Step 5: Commit**

```bash
git add internal/gateway/
git commit -m "feat: add gateway core structure"
```

---

## Task 9: Queue Implementation

**Files:**
- Create: `internal/gateway/queue.go`
- Create: `internal/gateway/queue_test.go`

**Step 1: Write the failing test**

```go
// internal/gateway/queue_test.go
package gateway

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/user/gopherclaw/internal/types"
)

func TestQueueConcurrency(t *testing.T) {
	queue := NewQueue(2) // Max 2 concurrent

	ctx := context.Background()
	queue.Start(ctx)
	defer queue.Stop()

	var running int32
	var maxSeen int32

	// Process function that tracks concurrency
	queue.processor = func(run *Run) error {
		current := atomic.AddInt32(&running, 1)
		if current > atomic.LoadInt32(&maxSeen) {
			atomic.StoreInt32(&maxSeen, current)
		}

		time.Sleep(50 * time.Millisecond)
		atomic.AddInt32(&running, -1)
		return nil
	}

	// Enqueue 5 runs
	for i := 0; i < 5; i++ {
		run := &Run{
			ID:        types.NewRunID(),
			SessionID: types.SessionID(fmt.Sprintf("session-%d", i)),
			Status:    RunStatusQueued,
		}
		if err := queue.Enqueue(run); err != nil {
			t.Fatal(err)
		}
	}

	// Wait for processing
	time.Sleep(200 * time.Millisecond)

	// Should have seen max 2 concurrent
	if maxSeen > 2 {
		t.Errorf("expected max 2 concurrent, saw %d", maxSeen)
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/gateway -v
```
Expected: FAIL with compile errors

**Step 3: Write minimal implementation**

```go
// internal/gateway/queue.go
package gateway

import (
	"context"
	"fmt"
	"sync"

	"golang.org/x/sync/semaphore"
	"github.com/user/gopherclaw/internal/types"
)

type Queue struct {
	lanes     map[types.SessionID]chan *Run
	semaphore *semaphore.Weighted
	processor func(*Run) error

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.RWMutex
}

func NewQueue(maxConcurrent int64) *Queue {
	return &Queue{
		lanes:     make(map[types.SessionID]chan *Run),
		semaphore: semaphore.NewWeighted(maxConcurrent),
	}
}

func (q *Queue) Start(ctx context.Context) {
	q.ctx, q.cancel = context.WithCancel(ctx)
}

func (q *Queue) Stop() {
	if q.cancel != nil {
		q.cancel()
	}

	// Close all lanes
	q.mu.Lock()
	for _, lane := range q.lanes {
		close(lane)
	}
	q.mu.Unlock()

	q.wg.Wait()
}

func (q *Queue) Enqueue(run *Run) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Get or create lane for session
	lane, exists := q.lanes[run.SessionID]
	if !exists {
		lane = make(chan *Run, 100)
		q.lanes[run.SessionID] = lane

		// Start processor for this lane
		q.wg.Add(1)
		go q.processLane(run.SessionID, lane)
	}

	select {
	case lane <- run:
		return nil
	default:
		return fmt.Errorf("queue full for session %s", run.SessionID)
	}
}

func (q *Queue) processLane(sessionID types.SessionID, lane chan *Run) {
	defer q.wg.Done()

	for {
		select {
		case run, ok := <-lane:
			if !ok {
				return // Lane closed
			}

			// Acquire semaphore
			if err := q.semaphore.Acquire(q.ctx, 1); err != nil {
				return // Context cancelled
			}

			// Process run
			q.wg.Add(1)
			go func() {
				defer q.wg.Done()
				defer q.semaphore.Release(1)

				if q.processor != nil {
					q.processor(run)
				}
			}()

		case <-q.ctx.Done():
			return
		}
	}
}

func (q *Queue) SetProcessor(fn func(*Run) error) {
	q.processor = fn
}
```

**Step 4: Add semaphore dependency**

```bash
go get golang.org/x/sync
```

**Step 5: Fix test import**

```go
// internal/gateway/queue_test.go - add import
import (
	"fmt"
	// ... other imports
)
```

**Step 6: Run test to verify it passes**

```bash
go test ./internal/gateway -v
```
Expected: PASS

**Step 7: Commit**

```bash
git add internal/gateway/ go.mod go.sum
git commit -m "feat: add queue implementation with concurrency control"
```

---

## Task 10: Retry Policy

**Files:**
- Create: `internal/gateway/retry.go`
- Create: `internal/gateway/retry_test.go`

**Step 1: Write the failing test**

```go
// internal/gateway/retry_test.go
package gateway

import (
	"errors"
	"testing"
	"time"
)

func TestRetryPolicy(t *testing.T) {
	policy := DefaultRetryPolicy()

	// Test retryable error
	if !policy.ShouldRetry(errors.New("connection refused"), 1) {
		t.Error("expected connection error to be retryable")
	}

	// Test max attempts
	if policy.ShouldRetry(errors.New("error"), 4) {
		t.Error("should not retry after max attempts")
	}

	// Test delay calculation
	delay := policy.NextDelay(1)
	if delay != 1*time.Second {
		t.Errorf("expected 1s delay, got %v", delay)
	}

	delay = policy.NextDelay(2)
	if delay != 2*time.Second {
		t.Errorf("expected 2s delay, got %v", delay)
	}

	delay = policy.NextDelay(3)
	if delay != 4*time.Second {
		t.Errorf("expected 4s delay, got %v", delay)
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/gateway -v
```
Expected: FAIL with "undefined: DefaultRetryPolicy"

**Step 3: Write minimal implementation**

```go
// internal/gateway/retry.go
package gateway

import (
	"math"
	"strings"
	"time"
)

type RetryPolicy struct {
	MaxAttempts  int
	InitialDelay time.Duration
	Multiplier   float64
	MaxDelay     time.Duration
}

func DefaultRetryPolicy() *RetryPolicy {
	return &RetryPolicy{
		MaxAttempts:  3,
		InitialDelay: 1 * time.Second,
		Multiplier:   2.0,
		MaxDelay:     30 * time.Second,
	}
}

func (p *RetryPolicy) ShouldRetry(err error, attempt int) bool {
	if attempt > p.MaxAttempts {
		return false
	}

	// Check if error is retryable
	return p.isRetryable(err)
}

func (p *RetryPolicy) isRetryable(err error) bool {
	if err == nil {
		return false
	}

	msg := strings.ToLower(err.Error())

	// Network errors
	if strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "temporary failure") {
		return true
	}

	// Don't retry validation/auth errors
	if strings.Contains(msg, "invalid") ||
		strings.Contains(msg, "unauthorized") ||
		strings.Contains(msg, "forbidden") {
		return false
	}

	// Default to retry
	return true
}

func (p *RetryPolicy) NextDelay(attempt int) time.Duration {
	delay := float64(p.InitialDelay) * math.Pow(p.Multiplier, float64(attempt-1))

	if delay > float64(p.MaxDelay) {
		return p.MaxDelay
	}

	return time.Duration(delay)
}

func (p *RetryPolicy) Execute(fn func() error) error {
	var lastErr error

	for attempt := 1; attempt <= p.MaxAttempts; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}

		lastErr = err

		if !p.ShouldRetry(err, attempt) {
			return err
		}

		if attempt < p.MaxAttempts {
			time.Sleep(p.NextDelay(attempt))
		}
	}

	return lastErr
}
```

**Step 4: Run test to verify it passes**

```bash
go test ./internal/gateway -v
```
Expected: PASS

**Step 5: Commit**

```bash
git add internal/gateway/
git commit -m "feat: add retry policy with exponential backoff"
```

---

## Task 11: LLM Provider Interface

**Files:**
- Create: `pkg/llm/provider.go`
- Create: `pkg/llm/types.go`
- Create: `pkg/llm/provider_test.go`

**Step 1: Write the types**

```go
// pkg/llm/types.go
package llm

import "encoding/json"

type Message struct {
	Role    string     `json:"role"`    // system, user, assistant
	Content string     `json:"content"`
	Tools   []ToolCall `json:"tool_calls,omitempty"`
}

type ToolCall struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"` // "function"
	Function FunctionCall    `json:"function"`
}

type FunctionCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type Tool struct {
	Type     string   `json:"type"` // "function"
	Function Function `json:"function"`
}

type Function struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type Response struct {
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	Usage     Usage      `json:"usage"`
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

type Delta struct {
	Content   string     `json:"content,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}
```

**Step 2: Write the provider interface**

```go
// pkg/llm/provider.go
package llm

import "context"

type Provider interface {
	// Complete sends messages and returns complete response
	Complete(ctx context.Context, messages []Message, tools []Tool) (*Response, error)

	// Stream sends messages and returns streaming response
	Stream(ctx context.Context, messages []Message, tools []Tool) (<-chan Delta, error)
}

type Config struct {
	BaseURL  string
	APIKey   string
	Model    string
	MaxTokens int
	Temperature float32
}
```

**Step 3: Write the test**

```go
// pkg/llm/provider_test.go
package llm

import (
	"context"
	"testing"
)

// MockProvider for testing
type MockProvider struct {
	CompleteFunc func(ctx context.Context, messages []Message, tools []Tool) (*Response, error)
	StreamFunc   func(ctx context.Context, messages []Message, tools []Tool) (<-chan Delta, error)
}

func (m *MockProvider) Complete(ctx context.Context, messages []Message, tools []Tool) (*Response, error) {
	if m.CompleteFunc != nil {
		return m.CompleteFunc(ctx, messages, tools)
	}
	return &Response{Content: "mock response"}, nil
}

func (m *MockProvider) Stream(ctx context.Context, messages []Message, tools []Tool) (<-chan Delta, error) {
	if m.StreamFunc != nil {
		return m.StreamFunc(ctx, messages, tools)
	}
	ch := make(chan Delta, 1)
	ch <- Delta{Content: "mock stream"}
	close(ch)
	return ch, nil
}

func TestProviderInterface(t *testing.T) {
	var provider Provider = &MockProvider{}

	ctx := context.Background()
	messages := []Message{{Role: "user", Content: "test"}}

	// Test complete
	resp, err := provider.Complete(ctx, messages, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content == "" {
		t.Error("expected non-empty response")
	}

	// Test stream
	stream, err := provider.Stream(ctx, messages, nil)
	if err != nil {
		t.Fatal(err)
	}

	delta := <-stream
	if delta.Content == "" {
		t.Error("expected non-empty delta")
	}
}
```

**Step 4: Run test to verify it passes**

```bash
go test ./pkg/llm -v
```
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/llm/
git commit -m "feat: add LLM provider interface and types"
```

---

## Task 12: OpenAI Client Implementation

**Files:**
- Create: `pkg/llm/openai/client.go`
- Create: `pkg/llm/openai/client_test.go`

**Step 1: Write the failing test**

```go
// pkg/llm/openai/client_test.go
package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/user/gopherclaw/pkg/llm"
)

func TestOpenAIClient(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify headers
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("missing or invalid auth header")
		}

		// Return mock response
		resp := map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"role":    "assistant",
						"content": "test response",
					},
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     10,
				"completion_tokens": 5,
				"total_tokens":      15,
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Create client
	config := &llm.Config{
		BaseURL: server.URL,
		APIKey:  "test-key",
		Model:   "gpt-3.5-turbo",
	}
	client := New(config)

	// Test complete
	ctx := context.Background()
	messages := []llm.Message{
		{Role: "user", Content: "hello"},
	}

	resp, err := client.Complete(ctx, messages, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content != "test response" {
		t.Errorf("expected 'test response', got %s", resp.Content)
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./pkg/llm/openai -v
```
Expected: FAIL with "undefined: New"

**Step 3: Write minimal implementation**

```go
// pkg/llm/openai/client.go
package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/user/gopherclaw/pkg/llm"
)

type Client struct {
	config     *llm.Config
	httpClient *http.Client
}

func New(config *llm.Config) *Client {
	return &Client{
		config: config,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

type openAIRequest struct {
	Model       string          `json:"model"`
	Messages    []openAIMessage `json:"messages"`
	Tools       []llm.Tool      `json:"tools,omitempty"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Temperature float32         `json:"temperature,omitempty"`
	Stream      bool            `json:"stream"`
}

type openAIMessage struct {
	Role      string         `json:"role"`
	Content   string         `json:"content"`
	ToolCalls []llm.ToolCall `json:"tool_calls,omitempty"`
}

type openAIResponse struct {
	Choices []struct {
		Message openAIMessage `json:"message"`
		Delta   openAIMessage `json:"delta,omitempty"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

func (c *Client) Complete(ctx context.Context, messages []llm.Message, tools []llm.Tool) (*llm.Response, error) {
	// Convert messages
	openAIMessages := make([]openAIMessage, len(messages))
	for i, msg := range messages {
		openAIMessages[i] = openAIMessage{
			Role:      msg.Role,
			Content:   msg.Content,
			ToolCalls: msg.Tools,
		}
	}

	// Build request
	req := openAIRequest{
		Model:       c.config.Model,
		Messages:    openAIMessages,
		Tools:       tools,
		MaxTokens:   c.config.MaxTokens,
		Temperature: c.config.Temperature,
		Stream:      false,
	}

	// Send request
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.config.BaseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.config.APIKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, body)
	}

	// Parse response
	var openAIResp openAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&openAIResp); err != nil {
		return nil, err
	}

	if len(openAIResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	// Convert to our format
	choice := openAIResp.Choices[0].Message
	return &llm.Response{
		Content:   choice.Content,
		ToolCalls: choice.ToolCalls,
		Usage: llm.Usage{
			InputTokens:  openAIResp.Usage.PromptTokens,
			OutputTokens: openAIResp.Usage.CompletionTokens,
			TotalTokens:  openAIResp.Usage.TotalTokens,
		},
	}, nil
}

func (c *Client) Stream(ctx context.Context, messages []llm.Message, tools []llm.Tool) (<-chan llm.Delta, error) {
	// TODO: Implement streaming
	// For now, just convert Complete to stream
	ch := make(chan llm.Delta, 1)

	go func() {
		defer close(ch)

		resp, err := c.Complete(ctx, messages, tools)
		if err != nil {
			return
		}

		ch <- llm.Delta{
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		}
	}()

	return ch, nil
}
```

**Step 4: Run test to verify it passes**

```bash
go test ./pkg/llm/openai -v
```
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/llm/openai/
git commit -m "feat: add OpenAI client implementation"
```

---

## Task 13: Main Entry Point

**Files:**
- Create: `cmd/gopherclaw/main.go`
- Create: `internal/config/config.go`

**Step 1: Write config loader**

```go
// internal/config/config.go
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	DataDir      string `json:"data_dir"`
	MaxConcurrent int    `json:"max_concurrent"`
	LLM          struct {
		Provider    string  `json:"provider"`
		BaseURL     string  `json:"base_url"`
		APIKey      string  `json:"api_key"`
		Model       string  `json:"model"`
		MaxTokens   int     `json:"max_tokens"`
		Temperature float32 `json:"temperature"`
	} `json:"llm"`
}

func Load(path string) (*Config, error) {
	// Default config
	cfg := &Config{
		DataDir:       filepath.Join(os.Getenv("HOME"), ".gopherclaw"),
		MaxConcurrent: 2,
	}
	cfg.LLM.Provider = "openai"
	cfg.LLM.BaseURL = "https://api.openai.com"
	cfg.LLM.Model = "gpt-3.5-turbo"
	cfg.LLM.MaxTokens = 2000
	cfg.LLM.Temperature = 0.7

	// Override from env
	if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
		cfg.LLM.APIKey = apiKey
	}
	if baseURL := os.Getenv("OPENAI_BASE_URL"); baseURL != "" {
		cfg.LLM.BaseURL = baseURL
	}

	// Load from file if exists
	if _, err := os.Stat(path); err == nil {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(data, cfg); err != nil {
			return nil, err
		}
	}

	return cfg, nil
}
```

**Step 2: Write main entry point**

```go
// cmd/gopherclaw/main.go
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/user/gopherclaw/internal/config"
	"github.com/user/gopherclaw/internal/gateway"
	"github.com/user/gopherclaw/internal/state"
)

func main() {
	// Parse flags
	configPath := flag.String("config", filepath.Join(os.Getenv("HOME"), ".gopherclaw", "config.json"), "config file path")
	flag.Parse()

	// Load config
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Ensure data directory exists
	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		log.Fatalf("Failed to create data dir: %v", err)
	}

	// Create stores
	sessions := state.NewSessionStore(cfg.DataDir)
	events := state.NewEventStore(cfg.DataDir)
	artifacts := state.NewArtifactStore(cfg.DataDir)

	// Create gateway
	gw := gateway.New(sessions, events, artifacts)

	// Start gateway
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	gw.Start(ctx)
	defer gw.Stop()

	fmt.Printf("Gopherclaw started\n")
	fmt.Printf("Data directory: %s\n", cfg.DataDir)
	fmt.Printf("Max concurrent runs: %d\n", cfg.MaxConcurrent)
	fmt.Printf("LLM provider: %s\n", cfg.LLM.Provider)
	fmt.Printf("Model: %s\n", cfg.LLM.Model)

	// Wait for interrupt
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nShutting down...")
}
```

**Step 3: Build and test**

```bash
go build -o bin/gopherclaw cmd/gopherclaw/main.go
```
Expected: Build succeeds

**Step 4: Commit**

```bash
git add cmd/ internal/config/
git commit -m "feat: add main entry point and config loader"
```

---

## Task 14: Integration Test

**Files:**
- Create: `test/integration_test.go`

**Step 1: Write integration test**

```go
// test/integration_test.go
//go:build integration

package test

import (
	"context"
	"testing"
	"time"

	"github.com/user/gopherclaw/internal/gateway"
	"github.com/user/gopherclaw/internal/state"
	"github.com/user/gopherclaw/internal/types"
)

func TestEndToEnd(t *testing.T) {
	dir := t.TempDir()

	// Create stores
	sessions := state.NewSessionStore(dir)
	events := state.NewEventStore(dir)
	artifacts := state.NewArtifactStore(dir)

	// Create gateway
	gw := gateway.New(sessions, events, artifacts)

	ctx := context.Background()
	gw.Start(ctx)
	defer gw.Stop()

	// Configure queue processor
	gw.Queue.SetProcessor(func(run *gateway.Run) error {
		// Simulate processing
		time.Sleep(10 * time.Millisecond)

		// Record event
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

	// Send multiple messages
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
	time.Sleep(100 * time.Millisecond)

	// Verify events were recorded
	sessionList, err := sessions.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessionList) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessionList))
	}

	eventList, err := events.Tail(ctx, sessionList[0].SessionID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(eventList) != 3 {
		t.Errorf("expected 3 events, got %d", len(eventList))
	}

	// Verify sequential processing
	for i, event := range eventList {
		if event.Seq != int64(i+1) {
			t.Errorf("expected seq %d, got %d", i+1, event.Seq)
		}
	}
}
```

**Step 2: Add missing import in integration test**

```go
// test/integration_test.go - add import
import (
	"fmt"
	// ... other imports
)
```

**Step 3: Run integration test**

```bash
go test -tags=integration ./test -v
```
Expected: PASS

**Step 4: Run all tests**

```bash
go test ./...
```
Expected: All PASS

**Step 5: Final commit**

```bash
git add test/
git commit -m "feat: add integration test suite

Complete implementation of Phases 0-2:
- Core types and interfaces
- File-based storage layer
- Gateway with queue and retry
- LLM provider abstraction
- OpenAI-compatible client

Generated with Claude Code
via Happy

Co-Authored-By: Claude <noreply@anthropic.com>
Co-Authored-By: Happy <yesreply@happy.engineering>"
```

---

## Summary

Implementation complete! The foundation is now in place with:

✅ **Phase 0**: Core contracts, types, and LLM abstraction
✅ **Phase 1**: File-based storage with session, event, and artifact stores
✅ **Phase 2**: Gateway orchestration with queue and retry policies

The system is ready for:
- Phase 3: Runtime and tools
- Phase 4: Context engine
- Phase 5: Telegram adapter
- Phase 6: Automations

To run the system:
```bash
make build
OPENAI_API_KEY=your-key ./bin/gopherclaw
```

To run tests:
```bash
make test
```