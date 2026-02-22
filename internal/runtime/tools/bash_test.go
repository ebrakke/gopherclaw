package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestBashName(t *testing.T) {
	b := NewBash()
	if b.Name() != "bash" {
		t.Errorf("expected 'bash', got %q", b.Name())
	}
}

func TestBashExecuteSimple(t *testing.T) {
	b := NewBash()
	args, _ := json.Marshal(map[string]string{"command": "echo hello"})
	result, err := b.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(result) != "hello" {
		t.Errorf("expected 'hello', got %q", result)
	}
}

func TestBashExecuteStderr(t *testing.T) {
	b := NewBash()
	args, _ := json.Marshal(map[string]string{"command": "echo err >&2"})
	result, err := b.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "err") {
		t.Errorf("expected stderr output, got %q", result)
	}
}

func TestBashExecuteTimeout(t *testing.T) {
	b := NewBash()
	args, _ := json.Marshal(map[string]any{"command": "sleep 10", "timeout_seconds": 1})
	start := time.Now()
	_, err := b.Execute(context.Background(), args)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if elapsed > 3*time.Second {
		t.Errorf("timeout took too long: %v", elapsed)
	}
}

func TestBashExecuteExitCode(t *testing.T) {
	b := NewBash()
	args, _ := json.Marshal(map[string]string{"command": "exit 1"})
	_, err := b.Execute(context.Background(), args)
	if err == nil {
		t.Fatal("expected error for non-zero exit")
	}
}

func TestBashParameters(t *testing.T) {
	b := NewBash()
	var schema map[string]any
	if err := json.Unmarshal(b.Parameters(), &schema); err != nil {
		t.Fatal(err)
	}
	if schema["type"] != "object" {
		t.Errorf("expected object schema, got %v", schema["type"])
	}
}
