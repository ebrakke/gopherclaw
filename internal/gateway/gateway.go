package gateway

import (
	"context"
	"fmt"
	"sync"

	"github.com/user/gopherclaw/internal/types"
)

// Gateway orchestrates inbound events into runs. It resolves (or creates)
// sessions, wraps each event in a Run, and enqueues the run for processing.
type Gateway struct {
	sessions  types.SessionStore
	events    types.EventStore
	artifacts types.ArtifactStore
	Queue     *Queue
	retry     *RetryPolicy

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// New creates a Gateway wired to the provided stores.
// The default concurrency limit is 2 and the default retry policy is applied.
func New(sessions types.SessionStore, events types.EventStore, artifacts types.ArtifactStore) *Gateway {
	return &Gateway{
		sessions:  sessions,
		events:    events,
		artifacts: artifacts,
		Queue:     NewQueue(2),
		retry:     DefaultRetryPolicy(),
	}
}

// Start initialises the gateway's context and starts the internal queue.
func (g *Gateway) Start(ctx context.Context) {
	g.ctx, g.cancel = context.WithCancel(ctx)
	g.Queue.Start(g.ctx)
}

// Stop cancels the gateway context, stops the queue, and waits for any
// outstanding work to finish.
func (g *Gateway) Stop() {
	if g.cancel != nil {
		g.cancel()
	}
	g.Queue.Stop()
	g.wg.Wait()
}

// HandleInbound resolves or creates a session for the event, wraps it in a
// Run, and enqueues it for processing.
func (g *Gateway) HandleInbound(ctx context.Context, event *types.InboundEvent) error {
	sessionID, err := g.sessions.ResolveOrCreate(ctx, event.SessionKey, "default")
	if err != nil {
		return fmt.Errorf("resolve session: %w", err)
	}
	run := NewRun(sessionID, event)
	return g.Queue.Enqueue(run)
}
