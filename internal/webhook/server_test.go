package webhook

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/user/gopherclaw/internal/state"
	"github.com/user/gopherclaw/internal/types"
)

type mockGateway struct {
	lastSessionKey string
	lastPrompt     string
	response       string
}

func (m *mockGateway) HandleTask(sessionKey, prompt string) (string, error) {
	m.lastSessionKey = sessionKey
	m.lastPrompt = prompt
	return m.response, nil
}

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

func TestHealthEndpoint(t *testing.T) {
	mock := &mockGateway{response: "unused"}
	srv := setupServer(t, mock)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["status"] != "ok" {
		t.Errorf("expected status ok, got %s", resp["status"])
	}
}

func TestWebhookAdHoc(t *testing.T) {
	mock := &mockGateway{response: "hello from LLM"}
	srv := setupServer(t, mock)

	body := `{"prompt":"say hi","session_key":"http:test"}`
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["response"] != "hello from LLM" {
		t.Errorf("expected 'hello from LLM', got %q", resp["response"])
	}
	if mock.lastSessionKey != "http:test" {
		t.Errorf("expected session key 'http:test', got %q", mock.lastSessionKey)
	}
	if mock.lastPrompt != "say hi" {
		t.Errorf("expected prompt 'say hi', got %q", mock.lastPrompt)
	}
}

func TestWebhookAdHocMissingFields(t *testing.T) {
	mock := &mockGateway{response: "unused"}
	srv := setupServer(t, mock)

	// Missing session_key
	body := `{"prompt":"say hi"}`
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}
}

func TestWebhookNamedTask(t *testing.T) {
	mock := &mockGateway{response: "greetings!"}
	task := &state.Task{
		Name:       "greet",
		Prompt:     "say hello",
		SessionKey: "http:greet-session",
		Enabled:    true,
	}
	srv := setupServer(t, mock, task)

	req := httptest.NewRequest(http.MethodPost, "/webhook/greet", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["response"] != "greetings!" {
		t.Errorf("expected 'greetings!', got %q", resp["response"])
	}
	if mock.lastSessionKey != "http:greet-session" {
		t.Errorf("expected session key 'http:greet-session', got %q", mock.lastSessionKey)
	}
	if mock.lastPrompt != "say hello" {
		t.Errorf("expected prompt 'say hello', got %q", mock.lastPrompt)
	}
}

func TestWebhookNamedTaskNotFound(t *testing.T) {
	mock := &mockGateway{response: "unused"}
	srv := setupServer(t, mock)

	req := httptest.NewRequest(http.MethodPost, "/webhook/nonexistent", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", w.Code)
	}
}

func TestWebhookNamedTaskDisabled(t *testing.T) {
	mock := &mockGateway{response: "unused"}
	task := &state.Task{
		Name:       "off",
		Prompt:     "disabled task",
		SessionKey: "http:off-session",
		Enabled:    false,
	}
	srv := setupServer(t, mock, task)

	req := httptest.NewRequest(http.MethodPost, "/webhook/off", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", w.Code)
	}
}

func TestWebhookNamedTaskOverridePrompt(t *testing.T) {
	mock := &mockGateway{response: "custom response"}
	task := &state.Task{
		Name:       "flex",
		Prompt:     "default prompt",
		SessionKey: "http:flex-session",
		Enabled:    true,
	}
	srv := setupServer(t, mock, task)

	body := `{"prompt":"override prompt"}`
	req := httptest.NewRequest(http.MethodPost, "/webhook/flex", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["response"] != "custom response" {
		t.Errorf("expected 'custom response', got %q", resp["response"])
	}
	if mock.lastPrompt != "override prompt" {
		t.Errorf("expected prompt 'override prompt', got %q", mock.lastPrompt)
	}
	if mock.lastSessionKey != "http:flex-session" {
		t.Errorf("expected session key 'http:flex-session', got %q", mock.lastSessionKey)
	}
}

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

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}
