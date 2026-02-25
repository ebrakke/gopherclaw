package webhook

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/user/gopherclaw/internal/state"
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
