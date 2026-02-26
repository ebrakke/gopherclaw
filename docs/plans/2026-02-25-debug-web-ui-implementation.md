# Debug Web UI Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a browser-based debug UI to gopherclaw that displays all sessions and their event history via the existing webhook HTTP server.

**Architecture:** Extend `internal/webhook/server.go` to accept SessionStore, EventStore, and ArtifactStore. Add JSON API endpoints (`/api/sessions`, `/api/sessions/{id}/events`, `/api/artifacts/{id}`). Embed a single HTML file with inline CSS/JS via Go `embed` and serve it at `/`.

**Tech Stack:** Go stdlib (`net/http`, `embed`, `encoding/json`), vanilla HTML/CSS/JS (no frameworks, no build tools)

**Design doc:** `docs/plans/2026-02-25-debug-web-ui-design.md`

---

### Task 1: Expand Server struct and constructor to accept stores

**Files:**
- Modify: `internal/webhook/server.go:17-34`
- Modify: `internal/webhook/server_test.go:26-36`

**Step 1: Write the failing test**

Add a test to `internal/webhook/server_test.go` that calls the new constructor signature and hits `/api/sessions` expecting a JSON array:

```go
func TestAPISessionsList(t *testing.T) {
	mock := &mockGateway{response: "unused"}
	dir := t.TempDir()
	taskStore := state.NewTaskStore(filepath.Join(dir, "tasks.json"))
	sessions := state.NewSessionStore(dir)
	events := state.NewEventStore(dir)
	artifacts := state.NewArtifactStore(dir)

	// Create a session so there's something to list
	ctx := context.Background()
	sid, err := sessions.ResolveOrCreate(ctx, "test:key", "default")
	if err != nil {
		t.Fatal(err)
	}

	srv := NewServer(taskStore, mock.HandleTask, sessions, events, artifacts)

	req := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 session, got %d", len(result))
	}
	if result[0]["session_id"] != string(sid) {
		t.Errorf("expected session_id %s, got %v", sid, result[0]["session_id"])
	}
}
```

Add `"context"` to the test file imports.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/webhook/ -run TestAPISessionsList -v`
Expected: Compilation failure — `NewServer` signature doesn't match yet.

**Step 3: Update Server struct and constructor**

In `internal/webhook/server.go`, change the `Server` struct and `NewServer`:

```go
import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/user/gopherclaw/internal/state"
	"github.com/user/gopherclaw/internal/types"
)

type Server struct {
	store     *state.TaskStore
	handler   TaskHandler
	sessions  types.SessionStore
	events    types.EventStore
	artifacts types.ArtifactStore
	mux       *http.ServeMux
}

func NewServer(store *state.TaskStore, handler TaskHandler, sessions types.SessionStore, events types.EventStore, artifacts types.ArtifactStore) *Server {
	s := &Server{
		store:     store,
		handler:   handler,
		sessions:  sessions,
		events:    events,
		artifacts: artifacts,
		mux:       http.NewServeMux(),
	}
	s.mux.HandleFunc("GET /health", s.handleHealth)
	s.mux.HandleFunc("POST /webhook", s.handleAdHoc)
	s.mux.HandleFunc("POST /webhook/", s.handleNamedTask)
	s.mux.HandleFunc("GET /api/sessions", s.handleAPISessions)
	s.mux.HandleFunc("GET /api/sessions/", s.handleAPISessionEvents)
	s.mux.HandleFunc("GET /api/artifacts/", s.handleAPIArtifact)
	s.mux.HandleFunc("GET /", s.handleIndex)
	return s
}
```

Add the `handleAPISessions` handler:

```go
type sessionResponse struct {
	SessionID  string `json:"session_id"`
	SessionKey string `json:"session_key"`
	Agent      string `json:"agent"`
	Status     string `json:"status"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
	EventCount int64  `json:"event_count"`
}

func (s *Server) handleAPISessions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessions, err := s.sessions.List(ctx)
	if err != nil {
		slog.Error("list sessions failed", "error", err)
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	result := make([]sessionResponse, 0, len(sessions))
	for _, sess := range sessions {
		count, _ := s.events.Count(ctx, sess.SessionID)
		result = append(result, sessionResponse{
			SessionID:  string(sess.SessionID),
			SessionKey: string(sess.SessionKey),
			Agent:      sess.Agent,
			Status:     sess.Status,
			CreatedAt:  sess.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			UpdatedAt:  sess.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
			EventCount: count,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].UpdatedAt > result[j].UpdatedAt
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
```

Add stub handlers so the code compiles (they'll be implemented in later tasks):

```go
func (s *Server) handleAPISessionEvents(w http.ResponseWriter, r *http.Request) {
	http.Error(w, `{"error":"not implemented"}`, http.StatusNotImplemented)
}

func (s *Server) handleAPIArtifact(w http.ResponseWriter, r *http.Request) {
	http.Error(w, `{"error":"not implemented"}`, http.StatusNotImplemented)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte("<html><body>debug ui placeholder</body></html>"))
}
```

**Step 4: Fix existing test helper**

Update `setupServer` in `server_test.go` to pass nil stores (existing tests don't need them):

```go
func setupServer(t *testing.T, mock *mockGateway, tasks ...*state.Task) *Server {
	t.Helper()
	dir := t.TempDir()
	store := state.NewTaskStore(filepath.Join(dir, "tasks.json"))
	for _, task := range tasks {
		if err := store.Add(task); err != nil {
			t.Fatal(err)
		}
	}
	return NewServer(store, mock.HandleTask, nil, nil, nil)
}
```

**Step 5: Update cmd_serve.go call site**

In `cmd/gopherclaw/cmd_serve.go`, change the `NewServer` call to pass the stores:

```go
webhookSrv := webhook.NewServer(taskStore, processTask, sessions, events, artifacts)
```

**Step 6: Run tests to verify they pass**

Run: `go test ./internal/webhook/ -v`
Expected: All tests pass including TestAPISessionsList.

Run: `go build ./cmd/gopherclaw/`
Expected: Build succeeds.

**Step 7: Commit**

```bash
git add internal/webhook/server.go internal/webhook/server_test.go cmd/gopherclaw/cmd_serve.go
git commit -m "feat: add /api/sessions endpoint to webhook server"
```

---

### Task 2: Implement /api/sessions/{id}/events endpoint

**Files:**
- Modify: `internal/webhook/server.go` (replace `handleAPISessionEvents` stub)
- Modify: `internal/webhook/server_test.go`

**Step 1: Write the failing test**

Add to `server_test.go`:

```go
func TestAPISessionEvents(t *testing.T) {
	mock := &mockGateway{response: "unused"}
	dir := t.TempDir()
	taskStore := state.NewTaskStore(filepath.Join(dir, "tasks.json"))
	sessions := state.NewSessionStore(dir)
	events := state.NewEventStore(dir)
	artifacts := state.NewArtifactStore(dir)

	ctx := context.Background()
	sid, err := sessions.ResolveOrCreate(ctx, "test:key", "default")
	if err != nil {
		t.Fatal(err)
	}

	// Append two events
	evt1 := &types.Event{
		ID:        types.NewEventID(),
		SessionID: sid,
		RunID:     types.NewRunID(),
		Type:      "user_message",
		Source:    "test",
		At:        time.Now(),
		Payload:   json.RawMessage(`{"text":"hello"}`),
	}
	evt2 := &types.Event{
		ID:        types.NewEventID(),
		SessionID: sid,
		RunID:     evt1.RunID,
		Type:      "assistant_message",
		Source:    "runtime",
		At:        time.Now(),
		Payload:   json.RawMessage(`{"text":"hi there"}`),
	}
	if err := events.Append(ctx, evt1); err != nil {
		t.Fatal(err)
	}
	if err := events.Append(ctx, evt2); err != nil {
		t.Fatal(err)
	}

	srv := NewServer(taskStore, mock.HandleTask, sessions, events, artifacts)

	req := httptest.NewRequest(http.MethodGet, "/api/sessions/"+string(sid)+"/events", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 events, got %d", len(result))
	}
	// Events should be in chronological order (seq 1 first)
	if result[0]["type"] != "user_message" {
		t.Errorf("expected first event type 'user_message', got %v", result[0]["type"])
	}
	if result[1]["type"] != "assistant_message" {
		t.Errorf("expected second event type 'assistant_message', got %v", result[1]["type"])
	}
}

func TestAPISessionEventsWithLimit(t *testing.T) {
	mock := &mockGateway{response: "unused"}
	dir := t.TempDir()
	taskStore := state.NewTaskStore(filepath.Join(dir, "tasks.json"))
	sessions := state.NewSessionStore(dir)
	events := state.NewEventStore(dir)
	artifacts := state.NewArtifactStore(dir)

	ctx := context.Background()
	sid, err := sessions.ResolveOrCreate(ctx, "test:key", "default")
	if err != nil {
		t.Fatal(err)
	}

	// Append 5 events
	runID := types.NewRunID()
	for i := 0; i < 5; i++ {
		evt := &types.Event{
			ID:        types.NewEventID(),
			SessionID: sid,
			RunID:     runID,
			Type:      "user_message",
			Source:    "test",
			At:        time.Now(),
			Payload:   json.RawMessage(`{"text":"msg"}`),
		}
		if err := events.Append(ctx, evt); err != nil {
			t.Fatal(err)
		}
	}

	srv := NewServer(taskStore, mock.HandleTask, sessions, events, artifacts)

	req := httptest.NewRequest(http.MethodGet, "/api/sessions/"+string(sid)+"/events?limit=3", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if len(result) != 3 {
		t.Fatalf("expected 3 events, got %d", len(result))
	}
}

func TestAPISessionEventsNotFound(t *testing.T) {
	mock := &mockGateway{response: "unused"}
	dir := t.TempDir()
	taskStore := state.NewTaskStore(filepath.Join(dir, "tasks.json"))
	sessions := state.NewSessionStore(dir)
	events := state.NewEventStore(dir)
	artifacts := state.NewArtifactStore(dir)

	srv := NewServer(taskStore, mock.HandleTask, sessions, events, artifacts)

	req := httptest.NewRequest(http.MethodGet, "/api/sessions/nonexistent-id/events", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	// Should return 200 with empty array (no events file = no events)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}
```

Add `"time"` and `"github.com/user/gopherclaw/internal/types"` to the test file imports.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/webhook/ -run TestAPISessionEvents -v`
Expected: FAIL — stub returns 501.

**Step 3: Implement the handler**

Replace the `handleAPISessionEvents` stub in `server.go`:

```go
func (s *Server) handleAPISessionEvents(w http.ResponseWriter, r *http.Request) {
	// Path: /api/sessions/{id}/events
	path := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) < 2 || parts[1] != "events" {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	sessionID := types.SessionID(parts[0])

	limit := 200
	if q := r.URL.Query().Get("limit"); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 {
			limit = n
		}
	}

	events, err := s.events.Tail(r.Context(), sessionID, limit)
	if err != nil {
		slog.Error("tail events failed", "session_id", sessionID, "error", err)
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}
	if events == nil {
		events = []*types.Event{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(events)
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/webhook/ -v`
Expected: All tests pass.

**Step 5: Commit**

```bash
git add internal/webhook/server.go internal/webhook/server_test.go
git commit -m "feat: add /api/sessions/{id}/events endpoint"
```

---

### Task 3: Implement /api/artifacts/{id} endpoint

**Files:**
- Modify: `internal/webhook/server.go` (replace `handleAPIArtifact` stub)
- Modify: `internal/webhook/server_test.go`

**Step 1: Write the failing test**

Add to `server_test.go`:

```go
func TestAPIArtifact(t *testing.T) {
	mock := &mockGateway{response: "unused"}
	dir := t.TempDir()
	taskStore := state.NewTaskStore(filepath.Join(dir, "tasks.json"))
	sessions := state.NewSessionStore(dir)
	events := state.NewEventStore(dir)
	artifacts := state.NewArtifactStore(dir)

	ctx := context.Background()
	sid, err := sessions.ResolveOrCreate(ctx, "test:key", "default")
	if err != nil {
		t.Fatal(err)
	}

	aid, err := artifacts.Put(ctx, sid, types.NewRunID(), "bash", "hello world output")
	if err != nil {
		t.Fatal(err)
	}

	srv := NewServer(taskStore, mock.HandleTask, sessions, events, artifacts)

	req := httptest.NewRequest(http.MethodGet, "/api/artifacts/"+string(aid), nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result string
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result != "hello world output" {
		t.Errorf("expected 'hello world output', got %q", result)
	}
}

func TestAPIArtifactNotFound(t *testing.T) {
	mock := &mockGateway{response: "unused"}
	dir := t.TempDir()
	taskStore := state.NewTaskStore(filepath.Join(dir, "tasks.json"))
	sessions := state.NewSessionStore(dir)
	events := state.NewEventStore(dir)
	artifacts := state.NewArtifactStore(dir)

	srv := NewServer(taskStore, mock.HandleTask, sessions, events, artifacts)

	req := httptest.NewRequest(http.MethodGet, "/api/artifacts/nonexistent-id", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/webhook/ -run TestAPIArtifact -v`
Expected: FAIL — stub returns 501.

**Step 3: Implement the handler**

Replace the `handleAPIArtifact` stub in `server.go`:

```go
func (s *Server) handleAPIArtifact(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/artifacts/")
	if id == "" {
		http.Error(w, `{"error":"artifact id required"}`, http.StatusBadRequest)
		return
	}

	data, err := s.artifacts.Get(r.Context(), types.ArtifactID(id))
	if err != nil {
		http.Error(w, `{"error":"artifact not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/webhook/ -v`
Expected: All tests pass.

**Step 5: Commit**

```bash
git add internal/webhook/server.go internal/webhook/server_test.go
git commit -m "feat: add /api/artifacts/{id} endpoint"
```

---

### Task 4: Create the embedded HTML debug UI

**Files:**
- Create: `internal/webhook/static/index.html`
- Modify: `internal/webhook/server.go` (replace `handleIndex` stub with embed)

**Step 1: Create the static directory and HTML file**

Create `internal/webhook/static/index.html` — a single HTML file with inline CSS and JS. The UI has:

- Left sidebar: session list (fetched from `/api/sessions`)
- Right panel: session metadata header + scrollable event list (fetched from `/api/sessions/{id}/events`)
- Tool calls/results rendered as collapsible `<details>` elements
- Artifact data loaded lazily via `/api/artifacts/{id}` on click
- Refresh button in the header
- Dark theme, monospace font

The HTML file content is provided in full in the next step.

**Step 2: Write the HTML file**

Create `internal/webhook/static/index.html` with the full debug UI. Key sections:

- **CSS**: Dark theme (`#1a1a2e` background, `#e0e0e0` text), two-column flexbox layout, chat-bubble-style messages, collapsible tool blocks
- **JS `loadSessions()`**: Fetches `/api/sessions`, renders sidebar list sorted by updated_at desc
- **JS `loadEvents(sessionId)`**: Fetches `/api/sessions/{id}/events`, renders each event type:
  - `user_message`: Blue-tinted chat bubble, shows payload.text
  - `assistant_message`: Green-tinted chat bubble, shows payload.text
  - `tool_call`: Collapsible `<details>` with tool name as summary, arguments in `<pre>` block
  - `tool_result`: Collapsible `<details>` with tool name + status, result in `<pre>`. If `artifact_id` present, show "Load artifact" link
- **JS `loadArtifact(id, el)`**: Fetches `/api/artifacts/{id}`, replaces the link with artifact content in `<pre>`
- **JS `timeAgo(dateStr)`**: Formats relative timestamps ("2m ago", "3h ago", etc.)

**Step 3: Add embed directive to server.go**

Add to the top of `server.go`, after the package declaration:

```go
import "embed"

//go:embed static/index.html
var indexHTML []byte
```

Replace the `handleIndex` stub:

```go
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(indexHTML)
}
```

**Step 4: Run build to verify embed works**

Run: `go build ./cmd/gopherclaw/`
Expected: Build succeeds.

**Step 5: Run all tests**

Run: `go test ./internal/webhook/ -v`
Expected: All tests pass.

Run: `go test ./... 2>&1 | tail -20`
Expected: All project tests pass.

**Step 6: Commit**

```bash
git add internal/webhook/static/index.html internal/webhook/server.go
git commit -m "feat: add embedded debug web UI at /"
```

---

### Task 5: Manual smoke test

**Step 1: Build and run**

```bash
go build -o gopherclaw ./cmd/gopherclaw/ && ./gopherclaw serve
```

**Step 2: Test in browser**

Open `http://localhost:8484/` in a browser. Verify:
- Session list loads in the left sidebar
- Clicking a session shows its events in the right panel
- User/assistant messages render as chat bubbles
- Tool calls are collapsible
- Artifacts load when clicked
- Refresh button works

**Step 3: Test API endpoints directly**

```bash
curl http://localhost:8484/api/sessions | jq .
curl http://localhost:8484/api/sessions/<session-id>/events | jq .
```

**Step 4: Fix any issues found during smoke test**

If anything is broken, fix it and re-run tests.

**Step 5: Final commit if there were fixes**

```bash
git add -A && git commit -m "fix: debug UI smoke test fixes"
```
