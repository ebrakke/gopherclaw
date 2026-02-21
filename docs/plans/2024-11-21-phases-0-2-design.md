# Gopherclaw Phases 0-2 Design

## Overview

Implementation design for gopherclaw v1 phases 0-2, covering core contracts, state management, and gateway orchestration. This establishes the foundation for a single-binary Go assistant runtime with filesystem-first state and deterministic execution.

## Architecture Decisions

### Package Structure: Domain-Driven

We're using domain-driven package organization for clear boundaries and testability:

```
gopherclaw/
├── cmd/gopherclaw/          # CLI entry point
├── internal/
│   ├── gateway/             # Request orchestration (Phase 2)
│   ├── state/               # Storage layer (Phase 1)
│   └── types/               # Core contracts (Phase 0)
└── pkg/llm/                 # LLM provider abstraction (Phase 0)
```

**Rationale:** Balances clean structure with pragmatism. Each domain owns its interfaces and implementation, avoiding circular dependencies while keeping code navigable.

### Provider: OpenAI-Compatible API

Initial implementation targets OpenAI-compatible APIs with configurable base URL.

**Rationale:** Maximum flexibility - works with OpenAI, Azure, local models, or any compatible endpoint.

### Scheduling: Standard Cron Syntax

Automation schedules use standard cron expressions (`*/5 * * * *`).

**Rationale:** Industry standard, well-understood, extensive tooling support.

### Retry Policy: Exponential Backoff

Failed operations retry with exponential backoff (1s, 2s, 4s) up to 3 attempts.

**Rationale:** Balances recovery from transient failures with avoiding cascade failures.

## Phase 0: Contracts & Core Types

### Core IDs

All IDs are strongly typed strings, typically UUID v4:

```go
package types

type SessionKey string   // "telegram:123:456"
type SessionID string    // UUID v4
type RunID string        // UUID v4
type EventID string      // UUID v4
type ArtifactID string   // UUID v4
type AutomationID string // UUID v4
```

### Storage Interfaces

```go
type SessionStore interface {
    ResolveOrCreate(ctx, key SessionKey, agent string) (SessionID, error)
    Get(ctx, id SessionID) (SessionIndex, error)
    List(ctx) ([]SessionIndex, error)
}

type EventStore interface {
    Append(ctx, event Event) error
    Tail(ctx, sessionID SessionID, limit int) ([]Event, error)
    Count(ctx, sessionID SessionID) (int64, error)
}

type ArtifactStore interface {
    Put(ctx, sessionID SessionID, runID RunID, tool string, v any) (ArtifactID, error)
    Get(ctx, id ArtifactID) (json.RawMessage, error)
    Excerpt(ctx, id ArtifactID, query string, maxTokens int) (string, error)
}
```

### LLM Provider Abstraction

```go
package llm

type Message struct {
    Role    string      `json:"role"`
    Content string      `json:"content"`
    Tools   []ToolCall  `json:"tool_calls,omitempty"`
}

type Provider interface {
    Complete(ctx context.Context, messages []Message, tools []Tool) (Response, error)
    Stream(ctx context.Context, messages []Message, tools []Tool) (<-chan Delta, error)
}
```

OpenAI implementation supports:
- Function calling protocol
- Streaming responses
- Configurable base URL
- Timeout and retry handling

## Phase 1: State & Storage Layer

### Filesystem Layout

```
~/.gopherclaw/
├── config.json              # Runtime configuration
├── sessions/
│   ├── sessions.json        # Session index
│   └── <sessionID>/
│       ├── events.jsonl     # Append-only event log
│       └── artifacts/
│           └── <artifactID>.json  # Full tool outputs
└── automations/
    └── automations.json     # Automation definitions
```

### Session Store Implementation

- **Index:** JSON file with RWMutex protection
- **Updates:** Atomic via temp file + rename
- **Resolution:** Stable SessionKey → SessionID mapping
- **Creation:** On-demand directory structure

### Event Store Implementation

- **Format:** Append-only JSONL
- **Locking:** flock() for concurrent append safety
- **Sequence:** Auto-incrementing per session
- **Size:** 100-500 bytes per event typical

### Artifact Store Implementation

- **Storage:** Individual JSON files
- **Naming:** `<artifactID>.json` with metadata in index
- **Size:** Can be large (full tool outputs)
- **Excerpt:** Smart extraction for context inclusion

### Safety Guarantees

1. **Atomic Writes:** All updates use temp + rename
2. **Append Safety:** Exclusive locks for JSONL appends
3. **Read Consistency:** Shared locks for reads
4. **Directory Creation:** MkdirAll is atomic
5. **Crash Recovery:** Append-only logs allow replay

## Phase 2: Gateway & Queue

### Gateway Architecture

The gateway is the central orchestrator:

```go
type Gateway struct {
    sessions SessionStore
    events   EventStore
    queue    *Queue
    retry    *RetryPolicy
}

func (g *Gateway) HandleInbound(ctx context.Context, event InboundEvent) error {
    // Resolve session
    sessionID := g.resolveSession(event)

    // Create run
    run := NewRun(sessionID, event)

    // Enqueue for execution
    return g.queue.Enqueue(run)
}
```

### Queue Implementation

**Per-Session Lanes:**
- Each session has a dedicated channel
- FIFO ordering within session
- Parallel execution across sessions

**Concurrency Control:**
- Global semaphore limits concurrent runs (default: 2)
- One goroutine per active session
- Graceful shutdown via context

```go
type Queue struct {
    lanes     map[SessionID]chan *Run
    semaphore *semaphore.Weighted
    mu        sync.RWMutex
}
```

### Run Lifecycle

```
Queued → Running → Complete/Failed
```

1. **Queued:** Waiting in session lane
2. **Running:** Acquired semaphore, executing
3. **Complete:** Success, events persisted
4. **Failed:** Retry exhausted or non-retryable error

### Retry Strategy

```go
type RetryPolicy struct {
    MaxAttempts int           // 3
    InitialDelay time.Duration // 1s
    Multiplier float64        // 2.0
    MaxDelay time.Duration    // 30s
}
```

**Retryable Errors:**
- Network failures
- Timeouts
- 5xx responses

**Non-Retryable Errors:**
- Validation failures
- Authentication errors
- 4xx responses

### Concurrency Guarantees

1. **Ordering:** Strict FIFO within each session
2. **Isolation:** No shared state between sessions
3. **Fairness:** Round-robin across session lanes
4. **Backpressure:** Bounded queues prevent memory exhaustion
5. **Graceful Shutdown:** Drain queues on termination

## Testing Strategy

### Unit Tests
- Pure functions in types package
- Storage operations with temp directories
- Queue operations with mock runs

### Integration Tests
- Full gateway flow with real filesystem
- Concurrent session handling
- Retry behavior verification

### Stress Tests
- High concurrency (100+ sessions)
- Large event logs (10k+ events)
- Queue saturation behavior

## Success Criteria

1. ✓ Single binary compiles and runs
2. ✓ Filesystem state survives restarts
3. ✓ Concurrent sessions execute correctly
4. ✓ Events are never lost or reordered
5. ✓ Retries handle transient failures
6. ✓ Clean shutdown preserves all state

## Next Steps

After Phases 0-2 are complete, the foundation supports:
- Phase 3: Runtime and tools
- Phase 4: Context engine
- Phase 5: Telegram adapter
- Phase 6: Automations

Each phase builds cleanly on this foundation without requiring refactoring.