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

// SessionStore is a JSON-file-backed session store.
// It stores session index data in sessions/sessions.json and creates
// per-session directories at sessions/<sessionID>/.
type SessionStore struct {
	root string
	mu   sync.RWMutex
}

// NewSessionStore creates a new file-backed SessionStore rooted at the given directory.
func NewSessionStore(root string) *SessionStore {
	return &SessionStore{root: root}
}

func (s *SessionStore) indexPath() string {
	return filepath.Join(s.root, "sessions", "sessions.json")
}

func (s *SessionStore) sessionsDir() string {
	return filepath.Join(s.root, "sessions")
}

func (s *SessionStore) sessionDir(id types.SessionID) string {
	return filepath.Join(s.root, "sessions", string(id))
}

// loadIndex reads sessions.json and returns a map keyed by SessionKey.
func (s *SessionStore) loadIndex() (map[types.SessionKey]*types.SessionIndex, error) {
	data, err := os.ReadFile(s.indexPath())
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[types.SessionKey]*types.SessionIndex), nil
		}
		return nil, fmt.Errorf("read session index: %w", err)
	}

	var sessions []*types.SessionIndex
	if err := json.Unmarshal(data, &sessions); err != nil {
		return nil, fmt.Errorf("unmarshal session index: %w", err)
	}

	index := make(map[types.SessionKey]*types.SessionIndex, len(sessions))
	for _, sess := range sessions {
		index[sess.SessionKey] = sess
	}
	return index, nil
}

// saveIndex converts the map to a slice, marshals with indentation, and writes atomically.
func (s *SessionStore) saveIndex(index map[types.SessionKey]*types.SessionIndex) error {
	sessions := make([]*types.SessionIndex, 0, len(index))
	for _, sess := range index {
		sessions = append(sessions, sess)
	}

	data, err := json.MarshalIndent(sessions, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session index: %w", err)
	}

	dir := s.sessionsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create sessions dir: %w", err)
	}

	// Atomic write: write to temp file then rename
	tmp := s.indexPath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write temp index: %w", err)
	}
	if err := os.Rename(tmp, s.indexPath()); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename temp index: %w", err)
	}
	return nil
}

// ResolveOrCreate returns the SessionID for the given key, creating a new session if needed.
func (s *SessionStore) ResolveOrCreate(_ context.Context, key types.SessionKey, agent string) (types.SessionID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	index, err := s.loadIndex()
	if err != nil {
		return "", err
	}

	if existing, ok := index[key]; ok {
		return existing.SessionID, nil
	}

	now := time.Now()
	id := types.NewSessionID()
	session := &types.SessionIndex{
		SessionID:  id,
		SessionKey: key,
		Agent:      agent,
		Status:     "active",
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	index[key] = session

	if err := s.saveIndex(index); err != nil {
		return "", err
	}

	// Create session directory on demand
	if err := os.MkdirAll(s.sessionDir(id), 0o755); err != nil {
		return "", fmt.Errorf("create session dir: %w", err)
	}

	return id, nil
}

// Get returns the session with the given ID.
func (s *SessionStore) Get(_ context.Context, id types.SessionID) (*types.SessionIndex, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	index, err := s.loadIndex()
	if err != nil {
		return nil, err
	}

	for _, sess := range index {
		if sess.SessionID == id {
			return sess, nil
		}
	}
	return nil, fmt.Errorf("session not found: %s", id)
}

// List returns all sessions.
func (s *SessionStore) List(_ context.Context) ([]*types.SessionIndex, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	index, err := s.loadIndex()
	if err != nil {
		return nil, err
	}

	sessions := make([]*types.SessionIndex, 0, len(index))
	for _, sess := range index {
		sessions = append(sessions, sess)
	}
	return sessions, nil
}

// Update persists changes to the given session, setting UpdatedAt to now.
func (s *SessionStore) Update(_ context.Context, session *types.SessionIndex) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	index, err := s.loadIndex()
	if err != nil {
		return err
	}

	if _, ok := index[session.SessionKey]; !ok {
		return fmt.Errorf("session not found: %s", session.SessionKey)
	}

	session.UpdatedAt = time.Now()
	index[session.SessionKey] = session

	return s.saveIndex(index)
}
