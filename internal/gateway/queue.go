package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/semaphore"

	"github.com/user/gopherclaw/internal/types"
)

// Queue manages per-session lanes with a global concurrency semaphore.
// Each session gets its own FIFO channel (lane) so that runs within a
// session are processed sequentially, while the semaphore limits the
// total number of concurrent run processors across all sessions.
type Queue struct {
	lanes     map[types.SessionID]chan *Run
	semaphore *semaphore.Weighted
	processor func(*Run) error
	active    atomic.Int64

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.RWMutex
}

// NewQueue creates a Queue that allows up to maxConcurrent runs to execute
// simultaneously across all session lanes.
func NewQueue(maxConcurrent int64) *Queue {
	return &Queue{
		lanes:     make(map[types.SessionID]chan *Run),
		semaphore: semaphore.NewWeighted(maxConcurrent),
	}
}

// Start initialises the queue's context. Must be called before Enqueue.
func (q *Queue) Start(ctx context.Context) {
	q.ctx, q.cancel = context.WithCancel(ctx)
}

// Stop cancels the queue context, closes all lanes, and waits for in-flight
// processors to finish.
func (q *Queue) Stop() {
	if q.cancel != nil {
		q.cancel()
	}
	q.mu.Lock()
	for _, lane := range q.lanes {
		close(lane)
	}
	q.mu.Unlock()
	q.wg.Wait()
}

// Enqueue adds a Run to the session's lane, creating the lane (and its
// goroutine) on first use. Returns an error if the lane's buffer is full.
func (q *Queue) Enqueue(run *Run) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	lane, exists := q.lanes[run.SessionID]
	if !exists {
		lane = make(chan *Run, 100)
		q.lanes[run.SessionID] = lane
		q.wg.Add(1)
		go q.processLane(run.SessionID, lane)
	}

	select {
	case lane <- run:
		return nil
	default:
		return fmt.Errorf("queue full for session %s", run.SessionID)
	}
}

// processLane drains a single session lane, acquiring a semaphore slot
// before running the processor synchronously. This ensures strict FIFO
// ordering within a session while the semaphore limits cross-session
// parallelism.
func (q *Queue) processLane(sessionID types.SessionID, lane chan *Run) {
	defer q.wg.Done()
	for {
		select {
		case run, ok := <-lane:
			if !ok {
				return
			}
			if err := q.semaphore.Acquire(q.ctx, 1); err != nil {
				return
			}
			if q.processor != nil {
				q.active.Add(1)
				run.Ctx = q.ctx
				if err := q.processor(run); err != nil {
					slog.Error("run failed", "run_id", string(run.ID), "session_id", string(run.SessionID), "error", err)
					if run.OnComplete != nil {
						run.OnComplete("Sorry, something went wrong processing your message.")
					}
				}
				q.active.Add(-1)
			}
			q.semaphore.Release(1)
		case <-q.ctx.Done():
			return
		}
	}
}

// WaitIdle blocks until no runs are actively being processed, or the timeout
// expires. Returns true if idle, false if timed out.
func (q *Queue) WaitIdle(timeout time.Duration) bool {
	deadline := time.After(timeout)
	for {
		if q.active.Load() == 0 {
			return true
		}
		select {
		case <-deadline:
			return false
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// SetProcessor sets the function invoked for each dequeued Run.
func (q *Queue) SetProcessor(fn func(*Run) error) {
	q.processor = fn
}
