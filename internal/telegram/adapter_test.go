package telegram

import (
	"strings"
	"testing"
)

func TestSplitMessage(t *testing.T) {
	short := "Hello world"
	parts := splitMessage(short)
	if len(parts) != 1 {
		t.Fatalf("expected 1 part, got %d", len(parts))
	}
	if parts[0] != short {
		t.Errorf("expected %q, got %q", short, parts[0])
	}
}

func TestSplitMessageLong(t *testing.T) {
	long := strings.Repeat("a", 5000)
	parts := splitMessage(long)
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(parts))
	}
	if len(parts[0]) != maxTelegramMessage {
		t.Errorf("expected first part length %d, got %d", maxTelegramMessage, len(parts[0]))
	}
}

func TestBuildSessionKey(t *testing.T) {
	key := buildSessionKey(12345, 67890)
	if string(key) != "telegram:12345:67890" {
		t.Errorf("expected 'telegram:12345:67890', got %q", key)
	}
}
