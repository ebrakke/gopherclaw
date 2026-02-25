// internal/webhook/server.go
package webhook

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/user/gopherclaw/internal/state"
	"github.com/user/gopherclaw/internal/types"
)

// TaskHandler is a callback that processes a prompt within the given session.
type TaskHandler func(sessionKey, prompt string) (string, error)

// Server is a lightweight HTTP handler for webhook endpoints.
type Server struct {
	store     *state.TaskStore
	handler   TaskHandler
	sessions  types.SessionStore
	events    types.EventStore
	artifacts types.ArtifactStore
	mux       *http.ServeMux
}

// NewServer creates a new webhook Server with the given task store, handler callback, and stores.
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

// ServeHTTP delegates to the internal mux, implementing http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// adHocRequest is the JSON body for POST /webhook.
type adHocRequest struct {
	Prompt     string `json:"prompt"`
	SessionKey string `json:"session_key"`
}

func (s *Server) handleAdHoc(w http.ResponseWriter, r *http.Request) {
	var req adHocRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	if req.Prompt == "" || req.SessionKey == "" {
		http.Error(w, `{"error":"prompt and session_key are required"}`, http.StatusBadRequest)
		return
	}

	resp, err := s.handler(req.SessionKey, req.Prompt)
	if err != nil {
		slog.Error("webhook ad-hoc handler failed", "error", err)
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"response": resp})
}

// namedTaskRequest is the optional JSON body for POST /webhook/{name}.
type namedTaskRequest struct {
	Prompt string `json:"prompt"`
}

func (s *Server) handleNamedTask(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/webhook/")
	if name == "" {
		http.Error(w, `{"error":"task name required"}`, http.StatusBadRequest)
		return
	}

	task, err := s.store.Get(name)
	if err != nil {
		http.Error(w, `{"error":"task not found"}`, http.StatusNotFound)
		return
	}

	if !task.Enabled {
		http.Error(w, `{"error":"task is disabled"}`, http.StatusForbidden)
		return
	}

	prompt := task.Prompt
	sessionKey := task.SessionKey

	// Allow body to override the prompt
	var body namedTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err == nil && body.Prompt != "" {
		prompt = body.Prompt
	}

	resp, err := s.handler(sessionKey, prompt)
	if err != nil {
		slog.Error("webhook named task handler failed", "task", name, "error", err)
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"response": resp})
}

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
	if s.sessions == nil || s.events == nil {
		http.Error(w, `{"error":"debug API not configured"}`, http.StatusServiceUnavailable)
		return
	}
	ctx := r.Context()
	sessions, err := s.sessions.List(ctx)
	if err != nil {
		slog.Error("list sessions failed", "error", err)
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	result := make([]sessionResponse, 0, len(sessions))
	for _, sess := range sessions {
		count, err := s.events.Count(ctx, sess.SessionID)
		if err != nil {
			slog.Warn("count events failed", "session_id", sess.SessionID, "error", err)
		}
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

func (s *Server) handleAPISessionEvents(w http.ResponseWriter, r *http.Request) {
	if s.events == nil {
		http.Error(w, `{"error":"debug API not configured"}`, http.StatusServiceUnavailable)
		return
	}

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

func (s *Server) handleAPIArtifact(w http.ResponseWriter, r *http.Request) {
	http.Error(w, `{"error":"not implemented"}`, http.StatusNotImplemented)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte("<html><body>debug ui placeholder</body></html>"))
}
