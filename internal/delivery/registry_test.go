// internal/delivery/registry_test.go
package delivery

import (
	"testing"
)

func TestRegistryDeliver(t *testing.T) {
	reg := NewRegistry()

	var gotKey, gotMsg string
	reg.Register("test:", func(sessionKey, message string) error {
		gotKey = sessionKey
		gotMsg = message
		return nil
	})

	err := reg.Deliver("test:123", "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotKey != "test:123" {
		t.Errorf("expected session key %q, got %q", "test:123", gotKey)
	}
	if gotMsg != "hello" {
		t.Errorf("expected message %q, got %q", "hello", gotMsg)
	}
}

func TestRegistryNoHandler(t *testing.T) {
	reg := NewRegistry()

	err := reg.Deliver("unknown:123", "hello")
	if err == nil {
		t.Fatal("expected error for unregistered prefix, got nil")
	}
}

func TestRegistryMultiplePrefixes(t *testing.T) {
	reg := NewRegistry()

	var telegramCalls, slackCalls int
	reg.Register("telegram:", func(sessionKey, message string) error {
		telegramCalls++
		return nil
	})
	reg.Register("slack:", func(sessionKey, message string) error {
		slackCalls++
		return nil
	})

	if err := reg.Deliver("telegram:42:100", "msg1"); err != nil {
		t.Fatalf("telegram deliver error: %v", err)
	}
	if err := reg.Deliver("slack:general", "msg2"); err != nil {
		t.Fatalf("slack deliver error: %v", err)
	}

	if telegramCalls != 1 {
		t.Errorf("expected 1 telegram call, got %d", telegramCalls)
	}
	if slackCalls != 1 {
		t.Errorf("expected 1 slack call, got %d", slackCalls)
	}
}
