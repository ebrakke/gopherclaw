// internal/state/session_test.go
package state

import (
	"context"
	"testing"

	"github.com/user/gopherclaw/internal/types"
)

func TestSessionStore(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStore(dir)
	ctx := context.Background()

	// Test resolve or create
	key := types.NewSessionKey("test", "123")
	id, err := store.ResolveOrCreate(ctx, key, "default")
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Error("expected non-empty session ID")
	}

	// Test get
	session, err := store.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if session.SessionKey != key {
		t.Errorf("expected key %s, got %s", key, session.SessionKey)
	}

	// Test idempotency
	id2, err := store.ResolveOrCreate(ctx, key, "default")
	if err != nil {
		t.Fatal(err)
	}
	if id != id2 {
		t.Error("expected same session ID for same key")
	}
}
