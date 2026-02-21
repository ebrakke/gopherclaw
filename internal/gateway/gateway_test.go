package gateway

import (
	"context"
	"testing"
	"time"

	"github.com/user/gopherclaw/internal/state"
	"github.com/user/gopherclaw/internal/types"
)

func TestGatewayHandleInbound(t *testing.T) {
	dir := t.TempDir()
	sessions := state.NewSessionStore(dir)
	events := state.NewEventStore(dir)
	artifacts := state.NewArtifactStore(dir)

	gw := New(sessions, events, artifacts)
	ctx := context.Background()
	gw.Start(ctx)
	defer gw.Stop()

	inbound := &types.InboundEvent{
		Source:     "test",
		SessionKey: types.NewSessionKey("test", "123"),
		UserID:     "user1",
		Text:       "hello",
	}

	err := gw.HandleInbound(ctx, inbound)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(100 * time.Millisecond)

	sessionList, err := sessions.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessionList) != 1 {
		t.Errorf("expected 1 session, got %d", len(sessionList))
	}
}

func TestGatewayMultipleEvents(t *testing.T) {
	dir := t.TempDir()
	sessions := state.NewSessionStore(dir)
	events := state.NewEventStore(dir)
	artifacts := state.NewArtifactStore(dir)

	gw := New(sessions, events, artifacts)
	ctx := context.Background()
	gw.Start(ctx)
	defer gw.Stop()

	// Send two events with the same session key -- should create only one session
	for i := 0; i < 2; i++ {
		inbound := &types.InboundEvent{
			Source:     "test",
			SessionKey: types.NewSessionKey("test", "same-key"),
			UserID:     "user1",
			Text:       "msg",
		}
		if err := gw.HandleInbound(ctx, inbound); err != nil {
			t.Fatal(err)
		}
	}

	time.Sleep(100 * time.Millisecond)

	sessionList, err := sessions.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessionList) != 1 {
		t.Errorf("expected 1 session (same key), got %d", len(sessionList))
	}
}

func TestGatewayDifferentSessions(t *testing.T) {
	dir := t.TempDir()
	sessions := state.NewSessionStore(dir)
	events := state.NewEventStore(dir)
	artifacts := state.NewArtifactStore(dir)

	gw := New(sessions, events, artifacts)
	ctx := context.Background()
	gw.Start(ctx)
	defer gw.Stop()

	// Send events with different session keys -- should create two sessions
	for _, key := range []string{"session-a", "session-b"} {
		inbound := &types.InboundEvent{
			Source:     "test",
			SessionKey: types.NewSessionKey("test", key),
			UserID:     "user1",
			Text:       "hello",
		}
		if err := gw.HandleInbound(ctx, inbound); err != nil {
			t.Fatal(err)
		}
	}

	time.Sleep(100 * time.Millisecond)

	sessionList, err := sessions.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessionList) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(sessionList))
	}
}
