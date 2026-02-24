// internal/state/task.go
package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Task represents a named prompt that can be triggered on a schedule or via webhook.
type Task struct {
	Name       string `json:"name"`
	Prompt     string `json:"prompt"`
	Schedule   string `json:"schedule,omitempty"`
	SessionKey string `json:"session_key"`
	Enabled    bool   `json:"enabled"`
}

// TaskStore is a JSON-file-backed store for tasks.
type TaskStore struct {
	path string
	mu   sync.RWMutex
}

// NewTaskStore creates a new file-backed TaskStore at the given file path.
func NewTaskStore(path string) *TaskStore {
	return &TaskStore{path: path}
}

// Path returns the file path used by this store.
func (s *TaskStore) Path() string {
	return s.path
}

// List returns all tasks. Returns an empty slice if the file doesn't exist.
func (s *TaskStore) List() ([]*Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tasks, err := s.load()
	if err != nil {
		return nil, err
	}
	if tasks == nil {
		return []*Task{}, nil
	}
	return tasks, nil
}

// Get finds a task by name. Returns an error if not found.
func (s *TaskStore) Get(name string) (*Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tasks, err := s.load()
	if err != nil {
		return nil, err
	}

	for _, task := range tasks {
		if task.Name == name {
			return task, nil
		}
	}
	return nil, fmt.Errorf("task not found: %s", name)
}

// Add appends a task. Returns an error if a task with the same name already exists.
func (s *TaskStore) Add(task *Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tasks, err := s.load()
	if err != nil {
		return err
	}

	for _, existing := range tasks {
		if existing.Name == task.Name {
			return fmt.Errorf("task already exists: %s", task.Name)
		}
	}

	tasks = append(tasks, task)
	return s.save(tasks)
}

// Remove deletes a task by name. Returns an error if not found.
func (s *TaskStore) Remove(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tasks, err := s.load()
	if err != nil {
		return err
	}

	for i, task := range tasks {
		if task.Name == name {
			tasks = append(tasks[:i], tasks[i+1:]...)
			return s.save(tasks)
		}
	}
	return fmt.Errorf("task not found: %s", name)
}

// SetEnabled toggles the enabled flag for a task. Returns an error if not found.
func (s *TaskStore) SetEnabled(name string, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tasks, err := s.load()
	if err != nil {
		return err
	}

	for _, task := range tasks {
		if task.Name == name {
			task.Enabled = enabled
			return s.save(tasks)
		}
	}
	return fmt.Errorf("task not found: %s", name)
}

// load reads the JSON file and returns the task list. Returns nil if the file doesn't exist.
func (s *TaskStore) load() ([]*Task, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read tasks file: %w", err)
	}

	var tasks []*Task
	if err := json.Unmarshal(data, &tasks); err != nil {
		return nil, fmt.Errorf("unmarshal tasks: %w", err)
	}
	return tasks, nil
}

// save writes the task list to disk using atomic write (temp file + rename).
func (s *TaskStore) save(tasks []*Task) error {
	data, err := json.MarshalIndent(tasks, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal tasks: %w", err)
	}

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create tasks dir: %w", err)
	}

	// Atomic write: write to temp file then rename
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write temp tasks file: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename temp tasks file: %w", err)
	}
	return nil
}
