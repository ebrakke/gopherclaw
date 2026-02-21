package gateway

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/user/gopherclaw/internal/types"
)

func TestQueueConcurrency(t *testing.T) {
	queue := NewQueue(2)
	ctx := context.Background()
	queue.Start(ctx)
	defer queue.Stop()

	var running int32
	var maxSeen int32

	queue.processor = func(run *Run) error {
		current := atomic.AddInt32(&running, 1)
		for {
			old := atomic.LoadInt32(&maxSeen)
			if current <= old || atomic.CompareAndSwapInt32(&maxSeen, old, current) {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
		atomic.AddInt32(&running, -1)
		return nil
	}

	for i := 0; i < 5; i++ {
		run := &Run{
			ID:        types.NewRunID(),
			SessionID: types.SessionID(fmt.Sprintf("session-%d", i)),
			Status:    RunStatusQueued,
		}
		if err := queue.Enqueue(run); err != nil {
			t.Fatal(err)
		}
	}

	time.Sleep(500 * time.Millisecond)

	if maxSeen > 2 {
		t.Errorf("expected max 2 concurrent, saw %d", maxSeen)
	}
}

func TestQueueProcessorCalled(t *testing.T) {
	queue := NewQueue(1)
	ctx := context.Background()
	queue.Start(ctx)
	defer queue.Stop()

	var processed int32

	queue.SetProcessor(func(run *Run) error {
		atomic.AddInt32(&processed, 1)
		return nil
	})

	run := &Run{
		ID:        types.NewRunID(),
		SessionID: types.SessionID("test-session"),
		Status:    RunStatusQueued,
	}
	if err := queue.Enqueue(run); err != nil {
		t.Fatal(err)
	}

	time.Sleep(100 * time.Millisecond)

	if atomic.LoadInt32(&processed) != 1 {
		t.Errorf("expected 1 processed run, got %d", processed)
	}
}

func TestQueueSameSessionOrdering(t *testing.T) {
	queue := NewQueue(1)
	ctx := context.Background()
	queue.Start(ctx)
	defer queue.Stop()

	var order []int
	done := make(chan struct{})

	queue.SetProcessor(func(run *Run) error {
		order = append(order, run.Attempts) // reuse Attempts as sequence marker
		if len(order) == 3 {
			close(done)
		}
		return nil
	})

	sessionID := types.SessionID("same-session")
	for i := 0; i < 3; i++ {
		run := &Run{
			ID:        types.NewRunID(),
			SessionID: sessionID,
			Status:    RunStatusQueued,
			Attempts:  i,
		}
		if err := queue.Enqueue(run); err != nil {
			t.Fatal(err)
		}
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for runs to process")
	}

	for i, v := range order {
		if v != i {
			t.Errorf("expected order[%d] = %d, got %d", i, i, v)
		}
	}
}

func TestQueueNoProcessor(t *testing.T) {
	queue := NewQueue(1)
	ctx := context.Background()
	queue.Start(ctx)
	defer queue.Stop()

	// Enqueue without setting a processor -- should not panic
	run := &Run{
		ID:        types.NewRunID(),
		SessionID: types.SessionID("no-proc"),
		Status:    RunStatusQueued,
	}
	if err := queue.Enqueue(run); err != nil {
		t.Fatal(err)
	}

	time.Sleep(100 * time.Millisecond)
}
