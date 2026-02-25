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

	task := &state.Task{
		Name:       "every-second",
		Prompt:     "do something every second",
		Schedule:   "* * * * * *",
		SessionKey: "telegram:123",
		Enabled:    true,
	}
	if err := store.Add(task); err != nil {
		t.Fatal(err)
	}

	var fires atomic.Int32
	handler := func(sessionKey, prompt string) {
		fires.Add(1)
	}

	sched := New(store, handler)
	if err := sched.Start(); err != nil {
		t.Fatal(err)
	}
	defer sched.Stop()

	// Wait up to 2.5 seconds for at least one fire
	deadline := time.After(2500 * time.Millisecond)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			t.Fatalf("handler did not fire within 2.5s, fires=%d", fires.Load())
		case <-ticker.C:
			if fires.Load() > 0 {
				return
			}
		}
	}
}

func TestSchedulerSkipsDisabled(t *testing.T) {
	dir := t.TempDir()
	store := state.NewTaskStore(filepath.Join(dir, "tasks.json"))

	task := &state.Task{
		Name:       "disabled-task",
		Prompt:     "should not fire",
		Schedule:   "* * * * * *",
		SessionKey: "telegram:123",
		Enabled:    false,
	}
	if err := store.Add(task); err != nil {
		t.Fatal(err)
	}

	var fires atomic.Int32
	handler := func(sessionKey, prompt string) {
		fires.Add(1)
	}

	sched := New(store, handler)
	if err := sched.Start(); err != nil {
		t.Fatal(err)
	}
	defer sched.Stop()

	time.Sleep(2 * time.Second)

	if n := fires.Load(); n != 0 {
		t.Errorf("expected 0 fires for disabled task, got %d", n)
	}
}

func TestSchedulerNoScheduleTasks(t *testing.T) {
	dir := t.TempDir()
	store := state.NewTaskStore(filepath.Join(dir, "tasks.json"))

	task := &state.Task{
		Name:       "no-schedule",
		Prompt:     "webhook only",
		Schedule:   "",
		SessionKey: "telegram:123",
		Enabled:    true,
	}
	if err := store.Add(task); err != nil {
		t.Fatal(err)
	}

	var fires atomic.Int32
	handler := func(sessionKey, prompt string) {
		fires.Add(1)
	}

	sched := New(store, handler)
	if err := sched.Start(); err != nil {
		t.Fatal(err)
	}
	defer sched.Stop()

	time.Sleep(2 * time.Second)

	if n := fires.Load(); n != 0 {
		t.Errorf("expected 0 fires for task with no schedule, got %d", n)
	}
}
