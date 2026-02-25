// internal/webhook/server.go
package webhook

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/user/gopherclaw/internal/state"
)

// TaskHandler is a callback that processes a prompt within the given session.
type TaskHandler func(sessionKey, prompt string) (string, error)

// Server is a lightweight HTTP handler for webhook endpoints.
type Server struct {
	store   *state.TaskStore
	handler TaskHandler
	mux     *http.ServeMux
}

// NewServer creates a new webhook Server with the given task store and handler callback.
func NewServer(store *state.TaskStore, handler TaskHandler) *Server {
	s := &Server{
		store:   store,
		handler: handler,
		mux:     http.NewServeMux(),
	}
	s.mux.HandleFunc("GET /health", s.handleHealth)
	s.mux.HandleFunc("POST /webhook", s.handleAdHoc)
	s.mux.HandleFunc("POST /webhook/", s.handleNamedTask)
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
