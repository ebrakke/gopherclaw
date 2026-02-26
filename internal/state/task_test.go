// internal/state/task_test.go
package state

import (
	"path/filepath"
	"testing"
)

func TestTaskStore_ListEmpty(t *testing.T) {
	dir := t.TempDir()
	store := NewTaskStore(filepath.Join(dir, "tasks.json"))

	tasks, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 0 {
		t.Errorf("expected empty list, got %d tasks", len(tasks))
	}
}

func TestTaskStore_AddAndList(t *testing.T) {
	dir := t.TempDir()
	store := NewTaskStore(filepath.Join(dir, "tasks.json"))

	task := &Task{
		Name:       "daily-report",
		Prompt:     "Generate a daily report",
		Schedule:   "0 9 * * *",
		SessionKey: "telegram:123",
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
	if tasks[0].Name != "daily-report" {
		t.Errorf("expected name daily-report, got %s", tasks[0].Name)
	}
	if tasks[0].Prompt != "Generate a daily report" {
		t.Errorf("expected prompt mismatch, got %s", tasks[0].Prompt)
	}
	if tasks[0].Schedule != "0 9 * * *" {
		t.Errorf("expected schedule 0 9 * * *, got %s", tasks[0].Schedule)
	}
	if tasks[0].SessionKey != "telegram:123" {
		t.Errorf("expected session_key telegram:123, got %s", tasks[0].SessionKey)
	}
	if !tasks[0].Enabled {
		t.Error("expected task to be enabled")
	}
}

func TestTaskStore_AddDuplicate(t *testing.T) {
	dir := t.TempDir()
	store := NewTaskStore(filepath.Join(dir, "tasks.json"))

	task := &Task{
		Name:       "my-task",
		Prompt:     "do something",
		SessionKey: "telegram:123",
		Enabled:    true,
	}

	if err := store.Add(task); err != nil {
		t.Fatal(err)
	}

	err := store.Add(task)
	if err == nil {
		t.Fatal("expected error for duplicate task name")
	}
}

func TestTaskStore_Get(t *testing.T) {
	dir := t.TempDir()
	store := NewTaskStore(filepath.Join(dir, "tasks.json"))

	task := &Task{
		Name:       "my-task",
		Prompt:     "do something",
		SessionKey: "telegram:123",
		Enabled:    true,
	}

	if err := store.Add(task); err != nil {
		t.Fatal(err)
	}

	got, err := store.Get("my-task")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "my-task" {
		t.Errorf("expected name my-task, got %s", got.Name)
	}
	if got.Prompt != "do something" {
		t.Errorf("expected prompt mismatch, got %s", got.Prompt)
	}
}

func TestTaskStore_GetNotFound(t *testing.T) {
	dir := t.TempDir()
	store := NewTaskStore(filepath.Join(dir, "tasks.json"))

	_, err := store.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent task")
	}
}

func TestTaskStore_Remove(t *testing.T) {
	dir := t.TempDir()
	store := NewTaskStore(filepath.Join(dir, "tasks.json"))

	task := &Task{
		Name:       "my-task",
		Prompt:     "do something",
		SessionKey: "telegram:123",
		Enabled:    true,
	}

	if err := store.Add(task); err != nil {
		t.Fatal(err)
	}

	if err := store.Remove("my-task"); err != nil {
		t.Fatal(err)
	}

	tasks, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 0 {
		t.Errorf("expected empty list after remove, got %d tasks", len(tasks))
	}
}

func TestTaskStore_RemoveNotFound(t *testing.T) {
	dir := t.TempDir()
	store := NewTaskStore(filepath.Join(dir, "tasks.json"))

	err := store.Remove("nonexistent")
	if err == nil {
		t.Fatal("expected error for removing nonexistent task")
	}
}

func TestTaskStore_SetEnabled(t *testing.T) {
	dir := t.TempDir()
	store := NewTaskStore(filepath.Join(dir, "tasks.json"))

	task := &Task{
		Name:       "my-task",
		Prompt:     "do something",
		SessionKey: "telegram:123",
		Enabled:    true,
	}

	if err := store.Add(task); err != nil {
		t.Fatal(err)
	}

	// Disable the task
	if err := store.SetEnabled("my-task", false); err != nil {
		t.Fatal(err)
	}

	got, err := store.Get("my-task")
	if err != nil {
		t.Fatal(err)
	}
	if got.Enabled {
		t.Error("expected task to be disabled")
	}

	// Re-enable the task
	if err := store.SetEnabled("my-task", true); err != nil {
		t.Fatal(err)
	}

	got, err = store.Get("my-task")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Enabled {
		t.Error("expected task to be enabled")
	}
}

func TestTaskStore_SetEnabledNotFound(t *testing.T) {
	dir := t.TempDir()
	store := NewTaskStore(filepath.Join(dir, "tasks.json"))

	err := store.SetEnabled("nonexistent", true)
	if err == nil {
		t.Fatal("expected error for SetEnabled on nonexistent task")
	}
}

func TestTaskStore_Path(t *testing.T) {
	path := "/tmp/test/tasks.json"
	store := NewTaskStore(path)

	if store.Path() != path {
		t.Errorf("expected path %s, got %s", path, store.Path())
	}
}

func TestTaskStore_Persistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tasks.json")

	// Create store and add a task
	store1 := NewTaskStore(path)
	task := &Task{
		Name:       "persist-task",
		Prompt:     "persist me",
		SessionKey: "telegram:456",
		Enabled:    true,
	}
	if err := store1.Add(task); err != nil {
		t.Fatal(err)
	}

	// Create a new store pointing to the same file
	store2 := NewTaskStore(path)
	tasks, err := store2.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task from new store, got %d", len(tasks))
	}
	if tasks[0].Name != "persist-task" {
		t.Errorf("expected name persist-task, got %s", tasks[0].Name)
	}
}
