// internal/types/ids_test.go
package types

import (
	"testing"
)

func TestNewSessionID(t *testing.T) {
	id := NewSessionID()
	if id == "" {
		t.Error("expected non-empty SessionID")
	}
	if len(string(id)) != 36 {
		t.Errorf("expected UUID format, got %s", id)
	}
}

func TestSessionKeyFormat(t *testing.T) {
	key := NewSessionKey("telegram", "123", "456")
	expected := SessionKey("telegram:123:456")
	if key != expected {
		t.Errorf("expected %s, got %s", expected, key)
	}
}
