package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMemoryToolNames(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "memory.md")

	save := NewMemorySave(path)
	del := NewMemoryDelete(path)
	list := NewMemoryList(path)

	if save.Name() != "memory_save" {
		t.Errorf("expected 'memory_save', got %q", save.Name())
	}
	if del.Name() != "memory_delete" {
		t.Errorf("expected 'memory_delete', got %q", del.Name())
	}
	if list.Name() != "memory_list" {
		t.Errorf("expected 'memory_list', got %q", list.Name())
	}
}

func TestMemorySaveAndList(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "memory.md")

	save := NewMemorySave(path)
	list := NewMemoryList(path)

	args, _ := json.Marshal(map[string]string{"content": "User's name is Alex"})
	result, err := save.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "Saved") {
		t.Errorf("expected confirmation, got %q", result)
	}

	listResult, err := list.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(listResult, "User's name is Alex") {
		t.Errorf("expected memory in list, got %q", listResult)
	}
}

func TestMemorySaveDeduplicates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "memory.md")

	save := NewMemorySave(path)
	args, _ := json.Marshal(map[string]string{"content": "Timezone: PST"})

	save.Execute(context.Background(), args)
	result, _ := save.Execute(context.Background(), args)
	if !strings.Contains(result, "already") {
		t.Errorf("expected dedup message, got %q", result)
	}

	data, _ := os.ReadFile(path)
	if strings.Count(string(data), "Timezone: PST") != 1 {
		t.Errorf("expected one entry, got:\n%s", data)
	}
}

func TestMemoryDelete(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "memory.md")

	os.WriteFile(path, []byte("- Fact one\n- Fact two\n- Fact three\n"), 0644)

	del := NewMemoryDelete(path)
	args, _ := json.Marshal(map[string]string{"content": "Fact two"})
	result, err := del.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "Deleted") {
		t.Errorf("expected delete confirmation, got %q", result)
	}

	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "Fact two") {
		t.Errorf("expected 'Fact two' removed, got:\n%s", data)
	}
	if !strings.Contains(string(data), "Fact one") || !strings.Contains(string(data), "Fact three") {
		t.Errorf("expected other facts preserved, got:\n%s", data)
	}
}

func TestMemoryDeleteNotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "memory.md")

	os.WriteFile(path, []byte("- Existing fact\n"), 0644)

	del := NewMemoryDelete(path)
	args, _ := json.Marshal(map[string]string{"content": "Nonexistent"})
	result, _ := del.Execute(context.Background(), args)
	if !strings.Contains(result, "not found") {
		t.Errorf("expected 'not found', got %q", result)
	}
}

func TestMemoryListEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "memory.md")

	list := NewMemoryList(path)
	result, err := list.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "No memories") {
		t.Errorf("expected empty message, got %q", result)
	}
}

func TestMemoryParameters(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "memory.md")

	save := NewMemorySave(path)
	var schema map[string]any
	if err := json.Unmarshal(save.Parameters(), &schema); err != nil {
		t.Fatal(err)
	}
	if schema["type"] != "object" {
		t.Errorf("expected object schema, got %v", schema["type"])
	}
}
