# Cron & Webhook System Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add scheduled recurring prompts (cron) and HTTP webhook triggers that flow through the existing gateway.

**Architecture:** Tasks are stored in `~/.gopherclaw/tasks.json`. A scheduler goroutine evaluates cron expressions and submits `InboundEvent`s to the gateway. A lightweight HTTP server accepts ad-hoc prompts and named task triggers. Both reuse the existing gateway/session/runtime pipeline. A delivery registry lets the Telegram adapter receive responses for cron-triggered tasks.

**Tech Stack:** `robfig/cron/v3` for cron parsing, `net/http` stdlib for the webhook server, existing `gateway.HandleInbound()` for task execution.

---

### Task 1: Task model and store

**Files:**
- Create: `internal/state/task.go`
- Create: `internal/state/task_test.go`

**Step 1: Write the failing test**

```go
// internal/state/task_test.go
package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTaskStoreAddAndList(t *testing.T) {
	dir := t.TempDir()
	store := NewTaskStore(filepath.Join(dir, "tasks.json"))

	task := &Task{
		Name:       "test-task",
		Prompt:     "say hello",
		SessionKey: "telegram:123:123",
		Enabled:    true,
	}

	if err := store.Add(task); err != nil {
		t.Fatal(err)
	}

	tasks, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].Name != "test-task" {
		t.Fatalf("expected name test-task, got %s", tasks[0].Name)
	}
}

func TestTaskStoreAddDuplicate(t *testing.T) {
	dir := t.TempDir()
	store := NewTaskStore(filepath.Join(dir, "tasks.json"))

	task := &Task{Name: "dup", Prompt: "hello", Enabled: true}
	if err := store.Add(task); err != nil {
		t.Fatal(err)
	}
	if err := store.Add(task); err == nil {
		t.Fatal("expected error for duplicate name")
	}
}

func TestTaskStoreRemove(t *testing.T) {
	dir := t.TempDir()
	store := NewTaskStore(filepath.Join(dir, "tasks.json"))

	store.Add(&Task{Name: "rm-me", Prompt: "hello", Enabled: true})
	if err := store.Remove("rm-me"); err != nil {
		t.Fatal(err)
	}
	tasks, _ := store.List()
	if len(tasks) != 0 {
		t.Fatalf("expected 0 tasks, got %d", len(tasks))
	}
}

func TestTaskStoreRemoveNotFound(t *testing.T) {
	dir := t.TempDir()
	store := NewTaskStore(filepath.Join(dir, "tasks.json"))

	if err := store.Remove("nope"); err == nil {
		t.Fatal("expected error for missing task")
	}
}

func TestTaskStoreGet(t *testing.T) {
	dir := t.TempDir()
	store := NewTaskStore(filepath.Join(dir, "tasks.json"))

	store.Add(&Task{Name: "find-me", Prompt: "hello", Enabled: true})
	task, err := store.Get("find-me")
	if err != nil {
		t.Fatal(err)
	}
	if task.Name != "find-me" {
		t.Fatalf("expected find-me, got %s", task.Name)
	}
}

func TestTaskStoreSetEnabled(t *testing.T) {
	dir := t.TempDir()
	store := NewTaskStore(filepath.Join(dir, "tasks.json"))

	store.Add(&Task{Name: "toggle", Prompt: "hello", Enabled: true})
	if err := store.SetEnabled("toggle", false); err != nil {
		t.Fatal(err)
	}
	task, _ := store.Get("toggle")
	if task.Enabled {
		t.Fatal("expected disabled")
	}
}

func TestTaskStoreListEmpty(t *testing.T) {
	dir := t.TempDir()
	store := NewTaskStore(filepath.Join(dir, "tasks.json"))

	tasks, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 0 {
		t.Fatalf("expected 0 tasks, got %d", len(tasks))
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/state/ -run TestTaskStore -v`
Expected: Compilation errors (types not defined yet)

**Step 3: Write implementation**

```go
// internal/state/task.go
package state

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// Task represents a named, optionally scheduled prompt.
type Task struct {
	Name       string `json:"name"`
	Prompt     string `json:"prompt"`
	Schedule   string `json:"schedule,omitempty"`
	SessionKey string `json:"session_key"`
	Enabled    bool   `json:"enabled"`
}

// TaskStore manages tasks in a JSON file.
type TaskStore struct {
	path string
	mu   sync.RWMutex
}

// NewTaskStore creates a TaskStore backed by the given file path.
func NewTaskStore(path string) *TaskStore {
	return &TaskStore{path: path}
}

// Path returns the file path of the task store.
func (s *TaskStore) Path() string {
	return s.path
}

func (s *TaskStore) load() ([]*Task, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read tasks: %w", err)
	}
	var tasks []*Task
	if err := json.Unmarshal(data, &tasks); err != nil {
		return nil, fmt.Errorf("parse tasks: %w", err)
	}
	return tasks, nil
}

func (s *TaskStore) save(tasks []*Task) error {
	data, err := json.MarshalIndent(tasks, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal tasks: %w", err)
	}
	data = append(data, '\n')
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("write tasks: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename tasks: %w", err)
	}
	return nil
}

// List returns all tasks.
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

// Get returns the task with the given name.
func (s *TaskStore) Get(name string) (*Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tasks, err := s.load()
	if err != nil {
		return nil, err
	}
	for _, t := range tasks {
		if t.Name == name {
			return t, nil
		}
	}
	return nil, fmt.Errorf("task not found: %s", name)
}

// Add appends a new task. Returns an error if the name already exists.
func (s *TaskStore) Add(task *Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tasks, err := s.load()
	if err != nil {
		return err
	}
	for _, t := range tasks {
		if t.Name == task.Name {
			return fmt.Errorf("task already exists: %s", task.Name)
		}
	}
	tasks = append(tasks, task)
	return s.save(tasks)
}

// Remove deletes a task by name.
func (s *TaskStore) Remove(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tasks, err := s.load()
	if err != nil {
		return err
	}
	for i, t := range tasks {
		if t.Name == name {
			tasks = append(tasks[:i], tasks[i+1:]...)
			return s.save(tasks)
		}
	}
	return fmt.Errorf("task not found: %s", name)
}

// SetEnabled toggles a task's enabled state.
func (s *TaskStore) SetEnabled(name string, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tasks, err := s.load()
	if err != nil {
		return err
	}
	for _, t := range tasks {
		if t.Name == name {
			t.Enabled = enabled
			return s.save(tasks)
		}
	}
	return fmt.Errorf("task not found: %s", name)
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/state/ -run TestTaskStore -v`
Expected: All PASS

**Step 5: Commit**

```bash
git add internal/state/task.go internal/state/task_test.go
git commit -m "feat: add task model and file-backed store"
```

---

### Task 2: CLI `task` subcommands

**Files:**
- Create: `cmd/gopherclaw/cmd_task.go`

**Step 1: Write implementation**

```go
// cmd/gopherclaw/cmd_task.go
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/user/gopherclaw/internal/state"
)

func init() {
	rootCmd.AddCommand(taskCmd)
	taskCmd.AddCommand(taskAddCmd, taskListCmd, taskRemoveCmd, taskEnableCmd, taskDisableCmd)

	taskAddCmd.Flags().String("name", "", "Task name (required)")
	taskAddCmd.Flags().String("prompt", "", "Prompt text (required)")
	taskAddCmd.Flags().String("schedule", "", "Cron expression (optional)")
	taskAddCmd.Flags().String("session-key", "", "Session key for delivery (required)")
	taskAddCmd.MarkFlagRequired("name")
	taskAddCmd.MarkFlagRequired("prompt")
	taskAddCmd.MarkFlagRequired("session-key")
}

func taskStore() *state.TaskStore {
	cfg := loadConfig()
	return state.NewTaskStore(filepath.Join(cfg.DataDir, "tasks.json"))
}

var taskCmd = &cobra.Command{
	Use:   "task",
	Short: "Manage scheduled tasks and webhooks",
}

var taskAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a new task",
	RunE: func(cmd *cobra.Command, args []string) error {
		name, _ := cmd.Flags().GetString("name")
		prompt, _ := cmd.Flags().GetString("prompt")
		schedule, _ := cmd.Flags().GetString("schedule")
		sessionKey, _ := cmd.Flags().GetString("session-key")

		task := &state.Task{
			Name:       name,
			Prompt:     prompt,
			Schedule:   schedule,
			SessionKey: sessionKey,
			Enabled:    true,
		}

		if err := taskStore().Add(task); err != nil {
			return err
		}
		fmt.Printf("Task %q added.\n", name)
		if schedule != "" {
			fmt.Printf("Schedule: %s\n", schedule)
		}
		fmt.Println("Webhook: POST /webhook/" + name)
		return nil
	},
}

var taskListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all tasks",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		tasks, err := taskStore().List()
		if err != nil {
			return err
		}
		if len(tasks) == 0 {
			fmt.Println("No tasks configured.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tSCHEDULE\tENABLED\tSESSION KEY")
		for _, t := range tasks {
			sched := t.Schedule
			if sched == "" {
				sched = "(webhook only)"
			}
			fmt.Fprintf(w, "%s\t%s\t%v\t%s\n", t.Name, sched, t.Enabled, t.SessionKey)
		}
		return w.Flush()
	},
}

var taskRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove a task",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := taskStore().Remove(args[0]); err != nil {
			return err
		}
		fmt.Printf("Task %q removed.\n", args[0])
		return nil
	},
}

var taskEnableCmd = &cobra.Command{
	Use:   "enable <name>",
	Short: "Enable a task",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := taskStore().SetEnabled(args[0], true); err != nil {
			return err
		}
		fmt.Printf("Task %q enabled.\n", args[0])
		return nil
	},
}

var taskDisableCmd = &cobra.Command{
	Use:   "disable <name>",
	Short: "Disable a task",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := taskStore().SetEnabled(args[0], false); err != nil {
			return err
		}
		fmt.Printf("Task %q disabled.\n", args[0])
		return nil
	},
}
```

**Step 2: Verify it compiles**

Run: `go build ./cmd/gopherclaw/`

**Step 3: Smoke test**

Run: `go run ./cmd/gopherclaw/ task list`
Expected: "No tasks configured."

Run: `go run ./cmd/gopherclaw/ task add --name test --prompt "hello" --session-key "telegram:1:1"`
Expected: `Task "test" added.`

Run: `go run ./cmd/gopherclaw/ task list`
Expected: Table with test task

Run: `go run ./cmd/gopherclaw/ task remove test`
Expected: `Task "test" removed.`

**Step 4: Commit**

```bash
git add cmd/gopherclaw/cmd_task.go
git commit -m "feat: add task CLI subcommands (add/list/remove/enable/disable)"
```

---

### Task 3: Scheduler

**Files:**
- Create: `internal/scheduler/scheduler.go`
- Create: `internal/scheduler/scheduler_test.go`

**Step 1: Add `robfig/cron/v3` dependency**

Run: `go get github.com/robfig/cron/v3`

**Step 2: Write the failing test**

```go
// internal/scheduler/scheduler_test.go
package scheduler

import (
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/user/gopherclaw/internal/state"
)

func TestSchedulerFiresTask(t *testing.T) {
	dir := t.TempDir()
	store := state.NewTaskStore(filepath.Join(dir, "tasks.json"))

	// Schedule to fire every second
	store.Add(&state.Task{
		Name:       "every-sec",
		Prompt:     "ping",
		Schedule:   "* * * * * *", // every second (6-field with seconds)
		SessionKey: "test:1:1",
		Enabled:    true,
	})

	var fired atomic.Int32
	handler := func(sessionKey, prompt string) {
		fired.Add(1)
	}

	s := New(store, handler)
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}

	time.Sleep(2500 * time.Millisecond)
	s.Stop()

	count := fired.Load()
	if count < 1 {
		t.Fatalf("expected at least 1 fire, got %d", count)
	}
}

func TestSchedulerSkipsDisabled(t *testing.T) {
	dir := t.TempDir()
	store := state.NewTaskStore(filepath.Join(dir, "tasks.json"))

	store.Add(&state.Task{
		Name:       "disabled-task",
		Prompt:     "ping",
		Schedule:   "* * * * * *",
		SessionKey: "test:1:1",
		Enabled:    false,
	})

	var fired atomic.Int32
	handler := func(sessionKey, prompt string) {
		fired.Add(1)
	}

	s := New(store, handler)
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}

	time.Sleep(2500 * time.Millisecond)
	s.Stop()

	if fired.Load() != 0 {
		t.Fatalf("expected 0 fires for disabled task, got %d", fired.Load())
	}
}

func TestSchedulerNoScheduleTasks(t *testing.T) {
	dir := t.TempDir()
	store := state.NewTaskStore(filepath.Join(dir, "tasks.json"))

	store.Add(&state.Task{
		Name:       "webhook-only",
		Prompt:     "ping",
		Schedule:   "",
		SessionKey: "test:1:1",
		Enabled:    true,
	})

	var fired atomic.Int32
	handler := func(sessionKey, prompt string) {
		fired.Add(1)
	}

	s := New(store, handler)
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}

	time.Sleep(2500 * time.Millisecond)
	s.Stop()

	if fired.Load() != 0 {
		t.Fatalf("expected 0 fires for webhook-only task, got %d", fired.Load())
	}
}
```

**Step 3: Run test to verify it fails**

Run: `go test ./internal/scheduler/ -run TestScheduler -v`
Expected: Compilation errors

**Step 4: Write implementation**

```go
// internal/scheduler/scheduler.go
package scheduler

import (
	"log/slog"

	"github.com/robfig/cron/v3"
	"github.com/user/gopherclaw/internal/state"
)

// Handler is called when a scheduled task fires.
type Handler func(sessionKey, prompt string)

// Scheduler evaluates cron expressions from the task store and fires
// tasks through the provided handler.
type Scheduler struct {
	store   *state.TaskStore
	handler Handler
	cron    *cron.Cron
}

// New creates a Scheduler.
func New(store *state.TaskStore, handler Handler) *Scheduler {
	return &Scheduler{
		store:   store,
		handler: handler,
		cron:    cron.New(cron.WithSeconds()),
	}
}

// Start loads tasks and begins the cron scheduler.
func (s *Scheduler) Start() error {
	tasks, err := s.store.List()
	if err != nil {
		return err
	}

	for _, task := range tasks {
		if task.Schedule == "" || !task.Enabled {
			continue
		}
		t := task // capture for closure
		_, err := s.cron.AddFunc(t.Schedule, func() {
			slog.Info("cron firing task", "name", t.Name, "session_key", t.SessionKey)
			s.handler(t.SessionKey, t.Prompt)
		})
		if err != nil {
			slog.Error("invalid cron schedule", "name", t.Name, "schedule", t.Schedule, "error", err)
			continue
		}
		slog.Info("scheduled task", "name", t.Name, "schedule", t.Schedule)
	}

	s.cron.Start()
	return nil
}

// Reload stops existing cron entries and reloads from the task store.
func (s *Scheduler) Reload() error {
	s.cron.Stop()
	s.cron = cron.New(cron.WithSeconds())
	return s.Start()
}

// Stop halts the cron scheduler.
func (s *Scheduler) Stop() {
	s.cron.Stop()
}
```

**Step 5: Run tests to verify they pass**

Run: `go test ./internal/scheduler/ -run TestScheduler -v -timeout 30s`
Expected: All PASS

**Step 6: Commit**

```bash
git add internal/scheduler/scheduler.go internal/scheduler/scheduler_test.go
git commit -m "feat: add cron scheduler for recurring tasks"
```

---

### Task 4: HTTP config and webhook server

**Files:**
- Modify: `internal/config/config.go:10-32` (add HTTP block to Config struct)
- Create: `internal/webhook/server.go`
- Create: `internal/webhook/server_test.go`

**Step 1: Add HTTP config**

Add to the `Config` struct in `internal/config/config.go` after the `Telegram` block (line 31):

```go
	HTTP struct {
		Enabled bool   `json:"enabled"`
		Listen  string `json:"listen"`
	} `json:"http"`
```

Add default in `Load()` after `cfg.LLM.OutputReserve = 4096` (after line 47):

```go
	cfg.HTTP.Listen = "127.0.0.1:8484"
```

**Step 2: Write the failing test**

```go
// internal/webhook/server_test.go
package webhook

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/user/gopherclaw/internal/state"
)

// mockGateway captures inbound events for testing.
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

func TestHealthEndpoint(t *testing.T) {
	dir := t.TempDir()
	store := state.NewTaskStore(filepath.Join(dir, "tasks.json"))
	gw := &mockGateway{response: "ok"}

	srv := New(store, gw.HandleTask)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestWebhookAdHoc(t *testing.T) {
	dir := t.TempDir()
	store := state.NewTaskStore(filepath.Join(dir, "tasks.json"))
	gw := &mockGateway{response: "hello back"}

	srv := New(store, gw.HandleTask)
	body := `{"prompt": "say hello", "session_key": "test:1:1"}`
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["response"] != "hello back" {
		t.Fatalf("expected 'hello back', got %q", resp["response"])
	}
	if gw.lastPrompt != "say hello" {
		t.Fatalf("expected prompt 'say hello', got %q", gw.lastPrompt)
	}
}

func TestWebhookAdHocMissingFields(t *testing.T) {
	dir := t.TempDir()
	store := state.NewTaskStore(filepath.Join(dir, "tasks.json"))
	gw := &mockGateway{response: "ok"}

	srv := New(store, gw.HandleTask)
	body := `{"prompt": "hello"}`
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestWebhookNamedTask(t *testing.T) {
	dir := t.TempDir()
	store := state.NewTaskStore(filepath.Join(dir, "tasks.json"))
	store.Add(&state.Task{
		Name:       "greet",
		Prompt:     "say hi",
		SessionKey: "test:1:1",
		Enabled:    true,
	})
	gw := &mockGateway{response: "hi there"}

	srv := New(store, gw.HandleTask)
	req := httptest.NewRequest(http.MethodPost, "/webhook/greet", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if gw.lastPrompt != "say hi" {
		t.Fatalf("expected 'say hi', got %q", gw.lastPrompt)
	}
}

func TestWebhookNamedTaskNotFound(t *testing.T) {
	dir := t.TempDir()
	store := state.NewTaskStore(filepath.Join(dir, "tasks.json"))
	gw := &mockGateway{response: "ok"}

	srv := New(store, gw.HandleTask)
	req := httptest.NewRequest(http.MethodPost, "/webhook/nonexistent", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestWebhookNamedTaskDisabled(t *testing.T) {
	dir := t.TempDir()
	store := state.NewTaskStore(filepath.Join(dir, "tasks.json"))
	store.Add(&state.Task{
		Name:       "off",
		Prompt:     "hello",
		SessionKey: "test:1:1",
		Enabled:    false,
	})
	gw := &mockGateway{response: "ok"}

	srv := New(store, gw.HandleTask)
	req := httptest.NewRequest(http.MethodPost, "/webhook/off", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestWebhookNamedTaskOverridePrompt(t *testing.T) {
	dir := t.TempDir()
	store := state.NewTaskStore(filepath.Join(dir, "tasks.json"))
	store.Add(&state.Task{
		Name:       "flex",
		Prompt:     "default prompt",
		SessionKey: "test:1:1",
		Enabled:    true,
	})
	gw := &mockGateway{response: "custom response"}

	srv := New(store, gw.HandleTask)
	body := `{"prompt": "override prompt"}`
	req := httptest.NewRequest(http.MethodPost, "/webhook/flex", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if gw.lastPrompt != "override prompt" {
		t.Fatalf("expected 'override prompt', got %q", gw.lastPrompt)
	}
}
```

**Step 3: Run test to verify it fails**

Run: `go test ./internal/webhook/ -run TestWebhook -v`
Expected: Compilation errors

**Step 4: Write implementation**

```go
// internal/webhook/server.go
package webhook

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/user/gopherclaw/internal/state"
)

// TaskHandler processes a task and returns the LLM response.
type TaskHandler func(sessionKey, prompt string) (string, error)

// Server is a lightweight HTTP server for webhook triggers.
type Server struct {
	store   *state.TaskStore
	handler TaskHandler
	mux     *http.ServeMux
}

// New creates a webhook Server.
func New(store *state.TaskStore, handler TaskHandler) *Server {
	s := &Server{
		store:   store,
		handler: handler,
		mux:     http.NewServeMux(),
	}
	s.mux.HandleFunc("GET /health", s.handleHealth)
	s.mux.HandleFunc("POST /webhook", s.handleAdHoc)
	s.mux.HandleFunc("POST /webhook/", s.handleNamed)
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

type adHocRequest struct {
	Prompt     string `json:"prompt"`
	SessionKey string `json:"session_key"`
}

func (s *Server) handleAdHoc(w http.ResponseWriter, r *http.Request) {
	var req adHocRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "invalid JSON"}`, http.StatusBadRequest)
		return
	}
	if req.Prompt == "" || req.SessionKey == "" {
		http.Error(w, `{"error": "prompt and session_key are required"}`, http.StatusBadRequest)
		return
	}

	slog.Info("webhook ad-hoc", "session_key", req.SessionKey)
	response, err := s.handler(req.SessionKey, req.Prompt)
	if err != nil {
		slog.Error("webhook handler error", "error", err)
		http.Error(w, `{"error": "processing failed"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"response": response})
}

func (s *Server) handleNamed(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/webhook/")
	if name == "" {
		http.Error(w, `{"error": "task name required"}`, http.StatusBadRequest)
		return
	}

	task, err := s.store.Get(name)
	if err != nil {
		http.Error(w, `{"error": "task not found"}`, http.StatusNotFound)
		return
	}
	if !task.Enabled {
		http.Error(w, `{"error": "task is disabled"}`, http.StatusForbidden)
		return
	}

	// Allow prompt override via body
	prompt := task.Prompt
	var body adHocRequest
	if r.Body != nil && r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil && body.Prompt != "" {
			prompt = body.Prompt
		}
	}

	slog.Info("webhook named task", "name", name, "session_key", task.SessionKey)
	response, err := s.handler(task.SessionKey, prompt)
	if err != nil {
		slog.Error("webhook handler error", "name", name, "error", err)
		http.Error(w, `{"error": "processing failed"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"response": response})
}
```

**Step 5: Run tests to verify they pass**

Run: `go test ./internal/webhook/ -run TestWebhook -v && go test ./internal/webhook/ -run TestHealth -v`
Expected: All PASS

**Step 6: Commit**

```bash
git add internal/config/config.go internal/webhook/server.go internal/webhook/server_test.go
git commit -m "feat: add webhook HTTP server with ad-hoc and named task endpoints"
```

---

### Task 5: Delivery registry and Telegram integration

**Files:**
- Modify: `internal/telegram/adapter.go:21-46` (add `SendTo` method, export `chatID` extraction)
- Create: `internal/delivery/registry.go`
- Create: `internal/delivery/registry_test.go`

**Step 1: Write the failing test**

```go
// internal/delivery/registry_test.go
package delivery

import (
	"testing"
)

func TestRegistryDeliver(t *testing.T) {
	r := NewRegistry()

	var delivered string
	r.Register("test:", func(sessionKey, message string) error {
		delivered = message
		return nil
	})

	if err := r.Deliver("test:123", "hello"); err != nil {
		t.Fatal(err)
	}
	if delivered != "hello" {
		t.Fatalf("expected 'hello', got %q", delivered)
	}
}

func TestRegistryNoHandler(t *testing.T) {
	r := NewRegistry()
	err := r.Deliver("unknown:123", "hello")
	if err == nil {
		t.Fatal("expected error for unregistered prefix")
	}
}

func TestRegistryMultiplePrefixes(t *testing.T) {
	r := NewRegistry()

	var target string
	r.Register("telegram:", func(sessionKey, message string) error {
		target = "telegram"
		return nil
	})
	r.Register("slack:", func(sessionKey, message string) error {
		target = "slack"
		return nil
	})

	r.Deliver("telegram:1:1", "hi")
	if target != "telegram" {
		t.Fatalf("expected telegram, got %s", target)
	}
	r.Deliver("slack:1", "hi")
	if target != "slack" {
		t.Fatalf("expected slack, got %s", target)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/delivery/ -v`
Expected: Compilation errors

**Step 3: Write implementation**

```go
// internal/delivery/registry.go
package delivery

import (
	"fmt"
	"strings"
)

// Handler delivers a message to the session identified by sessionKey.
type Handler func(sessionKey, message string) error

// Registry maps session key prefixes to delivery handlers.
type Registry struct {
	handlers map[string]Handler
}

// NewRegistry creates an empty delivery registry.
func NewRegistry() *Registry {
	return &Registry{handlers: make(map[string]Handler)}
}

// Register adds a handler for session keys starting with the given prefix.
func (r *Registry) Register(prefix string, handler Handler) {
	r.handlers[prefix] = handler
}

// Deliver routes a message to the handler matching the session key prefix.
func (r *Registry) Deliver(sessionKey, message string) error {
	for prefix, handler := range r.handlers {
		if strings.HasPrefix(sessionKey, prefix) {
			return handler(sessionKey, message)
		}
	}
	return fmt.Errorf("no delivery handler for session key: %s", sessionKey)
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/delivery/ -v`
Expected: All PASS

**Step 5: Add `SendTo` method to Telegram adapter**

In `internal/telegram/adapter.go`, add after the `sendResponse` method (after line 189):

```go
// SendTo delivers a message to a Telegram chat identified by session key.
// Session key format: "telegram:<userID>:<chatID>"
func (a *Adapter) SendTo(sessionKey, message string) error {
	parts := strings.Split(sessionKey, ":")
	if len(parts) != 3 || parts[0] != "telegram" {
		return fmt.Errorf("invalid telegram session key: %s", sessionKey)
	}
	chatID, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return fmt.Errorf("parse chat ID: %w", err)
	}
	if message == "" {
		return nil // bot decided not to respond
	}
	a.sendResponse(chatID, message)
	return nil
}
```

**Step 6: Commit**

```bash
git add internal/delivery/registry.go internal/delivery/registry_test.go internal/telegram/adapter.go
git commit -m "feat: add delivery registry and Telegram SendTo for cron responses"
```

---

### Task 6: Wire everything into `cmd_serve.go`

**Files:**
- Modify: `cmd/gopherclaw/cmd_serve.go:43-163`

**Step 1: Write the wiring code**

Add imports at top of `cmd_serve.go`:

```go
	"net/http"

	"github.com/user/gopherclaw/internal/delivery"
	"github.com/user/gopherclaw/internal/scheduler"
	"github.com/user/gopherclaw/internal/state"
	"github.com/user/gopherclaw/internal/webhook"
```

In `runServe`, after the Telegram adapter block (after line 134), add:

```go
	// Task store
	taskStore := state.NewTaskStore(filepath.Join(cfg.DataDir, "tasks.json"))

	// Delivery registry
	deliveryReg := delivery.NewRegistry()

	// Helper: synchronously process a task through the gateway and return the response.
	processTask := func(sessionKey, prompt string) (string, error) {
		done := make(chan string, 1)
		event := &types.InboundEvent{
			Source:     "task",
			SessionKey: types.SessionKey(sessionKey),
			UserID:     "system",
			Text:       prompt,
		}
		if err := gw.HandleInbound(ctx, event, gateway.WithOnComplete(func(response string) {
			done <- response
		})); err != nil {
			return "", err
		}
		return <-done, nil
	}

	// Scheduler
	sched := scheduler.New(taskStore, func(sessionKey, prompt string) {
		response, err := processTask(sessionKey, prompt)
		if err != nil {
			slog.Error("cron task failed", "session_key", sessionKey, "error", err)
			return
		}
		if response == "" {
			return // bot decided not to respond
		}
		if err := deliveryReg.Deliver(sessionKey, response); err != nil {
			slog.Error("cron delivery failed", "session_key", sessionKey, "error", err)
		}
	})
	if err := sched.Start(); err != nil {
		return fmt.Errorf("start scheduler: %w", err)
	}
	defer sched.Stop()
	slog.Info("scheduler started")

	// Webhook HTTP server
	if cfg.HTTP.Enabled {
		webhookSrv := webhook.New(taskStore, processTask)
		httpServer := &http.Server{
			Addr:    cfg.HTTP.Listen,
			Handler: webhookSrv,
		}
		go func() {
			slog.Info("webhook server started", "listen", cfg.HTTP.Listen)
			if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				slog.Error("webhook server error", "error", err)
			}
		}()
		go func() {
			<-ctx.Done()
			httpServer.Close()
		}()
	}
```

Move the Telegram adapter block to register with the delivery registry. Replace the existing Telegram block (lines 124-134) with:

```go
	// Telegram adapter
	if cfg.Telegram.Token != "" {
		adapter, err := telegram.New(cfg.Telegram.Token, gw, events, sessions, engine, toolNames, memoryPath)
		if err != nil {
			return fmt.Errorf("create telegram adapter: %w", err)
		}
		go adapter.Start(ctx)
		slog.Info("telegram adapter started")

		// Register telegram delivery for cron responses
		deliveryReg.Register("telegram:", func(sessionKey, message string) error {
			return adapter.SendTo(sessionKey, message)
		})
	} else {
		slog.Warn("telegram adapter disabled (no token)")
	}
```

**Step 2: Verify it compiles**

Run: `go build ./cmd/gopherclaw/`
Expected: Clean build

**Step 3: Commit**

```bash
git add cmd/gopherclaw/cmd_serve.go
git commit -m "feat: wire scheduler, webhook server, and delivery registry into serve"
```

---

### Task 7: End-to-end smoke test

**Files:**
- No new files

**Step 1: Add a test task via CLI**

Run: `go run ./cmd/gopherclaw/ task add --name smoke-test --prompt "respond with just the word PONG" --session-key "test:1:1"`

**Step 2: Enable HTTP in config**

Run: `go run ./cmd/gopherclaw/ config set http.enabled true`

**Step 3: Start the daemon**

Run: `go run ./cmd/gopherclaw/ serve &`

**Step 4: Test the health endpoint**

Run: `curl -s http://127.0.0.1:8484/health`
Expected: `{"status":"ok"}`

**Step 5: Test the named webhook**

Run: `curl -s -X POST http://127.0.0.1:8484/webhook/smoke-test`
Expected: JSON response with LLM output

**Step 6: Test ad-hoc webhook**

Run: `curl -s -X POST http://127.0.0.1:8484/webhook -H 'Content-Type: application/json' -d '{"prompt": "say hello", "session_key": "test:1:1"}'`
Expected: JSON response with LLM output

**Step 7: Verify task list**

Run: `go run ./cmd/gopherclaw/ task list`
Expected: Table showing smoke-test task

**Step 8: Clean up**

Kill the daemon. Remove the test task:
Run: `go run ./cmd/gopherclaw/ task remove smoke-test`

**Step 9: Commit (if any fixes were needed)**

---

### Task 8: Run full test suite

**Step 1: Run all tests**

Run: `go test ./... -v -timeout 60s`
Expected: All PASS

**Step 2: Final commit if any fixes**

```bash
git add -A
git commit -m "fix: address test issues from cron/webhook integration"
```
