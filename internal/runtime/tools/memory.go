package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
)

var memoryMu sync.Mutex

func readMemoryFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}

func writeMemoryFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}

// MemorySave appends a fact to the memory file.
type MemorySave struct{ path string }

func NewMemorySave(path string) *MemorySave { return &MemorySave{path: path} }

func (m *MemorySave) Name() string        { return "memory_save" }
func (m *MemorySave) Description() string { return "Save a fact or preference to persistent memory" }
func (m *MemorySave) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"content": {"type": "string", "description": "The fact or preference to remember"}
		},
		"required": ["content"]
	}`)
}

func (m *MemorySave) Execute(_ context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("parse args: %w", err)
	}
	if params.Content == "" {
		return "", fmt.Errorf("content is required")
	}

	memoryMu.Lock()
	defer memoryMu.Unlock()

	existing, err := readMemoryFile(m.path)
	if err != nil {
		return "", err
	}

	line := "- " + params.Content
	for _, l := range strings.Split(existing, "\n") {
		if strings.TrimSpace(l) == strings.TrimSpace(line) {
			return "Memory already exists: " + params.Content, nil
		}
	}

	f, err := os.OpenFile(m.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return "", err
	}
	defer f.Close()

	if _, err := f.WriteString(line + "\n"); err != nil {
		return "", err
	}
	return "Saved: " + params.Content, nil
}

// MemoryDelete removes a fact from the memory file.
type MemoryDelete struct{ path string }

func NewMemoryDelete(path string) *MemoryDelete { return &MemoryDelete{path: path} }

func (m *MemoryDelete) Name() string        { return "memory_delete" }
func (m *MemoryDelete) Description() string { return "Delete a fact or preference from persistent memory" }
func (m *MemoryDelete) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"content": {"type": "string", "description": "The fact or preference to forget (must match existing entry)"}
		},
		"required": ["content"]
	}`)
}

func (m *MemoryDelete) Execute(_ context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("parse args: %w", err)
	}
	if params.Content == "" {
		return "", fmt.Errorf("content is required")
	}

	memoryMu.Lock()
	defer memoryMu.Unlock()

	existing, err := readMemoryFile(m.path)
	if err != nil {
		return "", err
	}

	target := "- " + params.Content
	lines := strings.Split(existing, "\n")
	var kept []string
	found := false
	for _, l := range lines {
		if strings.TrimSpace(l) == strings.TrimSpace(target) {
			found = true
			continue
		}
		if l != "" {
			kept = append(kept, l)
		}
	}

	if !found {
		return "Memory not found: " + params.Content, nil
	}

	content := ""
	if len(kept) > 0 {
		content = strings.Join(kept, "\n") + "\n"
	}
	if err := writeMemoryFile(m.path, content); err != nil {
		return "", err
	}
	return "Deleted: " + params.Content, nil
}

// MemoryList returns all stored memories.
type MemoryList struct{ path string }

func NewMemoryList(path string) *MemoryList { return &MemoryList{path: path} }

func (m *MemoryList) Name() string        { return "memory_list" }
func (m *MemoryList) Description() string { return "List all facts and preferences in persistent memory" }
func (m *MemoryList) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {}
	}`)
}

func (m *MemoryList) Execute(_ context.Context, _ json.RawMessage) (string, error) {
	memoryMu.Lock()
	defer memoryMu.Unlock()

	content, err := readMemoryFile(m.path)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(content) == "" {
		return "No memories stored yet.", nil
	}
	return content, nil
}
