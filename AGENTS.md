# AGENTS.md — LLM Developer Guide

This file helps AI coding assistants understand the gopherclaw codebase quickly. Read this before making changes.

## What is gopherclaw?

A Go assistant runtime that receives messages (from Telegram or automations), routes them through a gateway, executes LLM tool loops, and persists everything to the filesystem. Think of it as a self-hosted agent framework with context engineering as the primary differentiator.

## Package dependency graph

```
cmd/gopherclaw/main.go
  ├── internal/config      (config loading)
  ├── internal/gateway      (orchestration)
  │     └── internal/types  (interfaces)
  ├── internal/state        (storage implementations)
  │     └── internal/types  (interfaces, models)
  └── pkg/llm              (LLM provider - not yet wired to gateway)
        └── pkg/llm/openai (OpenAI client)
```

No circular dependencies. `internal/types` is the shared contract layer. `internal/state` implements storage. `internal/gateway` consumes storage via interfaces.

## How to navigate

**"Where are the data types?"** → `internal/types/models.go` (Event, SessionIndex, ArtifactMeta, InboundEvent)

**"Where are the ID types?"** → `internal/types/ids.go` (SessionKey, SessionID, RunID, EventID, ArtifactID, AutomationID)

**"Where are the storage interfaces?"** → `internal/types/interfaces.go` (SessionStore, EventStore, ArtifactStore)

**"Where are the storage implementations?"** → `internal/state/` (session.go, event.go, artifact.go)

**"Where is the gateway?"** → `internal/gateway/gateway.go` (Gateway struct, HandleInbound)

**"Where is the queue?"** → `internal/gateway/queue.go` (per-session lanes, global semaphore)

**"Where is the retry logic?"** → `internal/gateway/retry.go` (exponential backoff)

**"Where is the LLM client?"** → `pkg/llm/openai/client.go` (OpenAI-compatible)

**"Where is config?"** → `internal/config/config.go` (Load with defaults → file → env)

**"Where is main?"** → `cmd/gopherclaw/main.go`

## Key patterns to follow

### 1. Interfaces live in `internal/types`, implementations elsewhere

Storage interfaces are in `internal/types/interfaces.go`. Implementations are in `internal/state/`. The gateway consumes interfaces, not concrete types. This enables testing with mocks and future backend swaps.

### 2. Compile-time interface checks

Every implementation has a compile-time check in `internal/state/doc.go`:

```go
var _ types.SessionStore = (*SessionStore)(nil)
var _ types.EventStore = (*EventStore)(nil)
var _ types.ArtifactStore = (*ArtifactStore)(nil)
```

Add these when creating new interface implementations.

### 3. Atomic file writes

All mutable files (session index, artifacts) use the temp-file-plus-rename pattern:

```go
tmpPath := path + ".tmp"
os.WriteFile(tmpPath, data, 0644)
os.Rename(tmpPath, path)
```

Never write directly to the target file. Event logs are the exception — they use `O_APPEND`.

### 4. Per-session locking

EventStore uses per-session mutexes (`map[SessionID]*sync.Mutex`). SessionStore uses a single RWMutex for the index. Don't use a global lock where a per-session lock suffices.

### 5. FIFO within sessions

The queue processes runs synchronously within each session lane (not in goroutines). This guarantees strict ordering. The global semaphore limits cross-session parallelism. Do not change `processLane` to dispatch goroutines — this was intentionally fixed to prevent FIFO violations.

### 6. Config precedence

Defaults → JSON file → Environment variables. Env vars always win. This follows 12-factor convention.

### 7. Error wrapping

Use `fmt.Errorf("context: %w", err)` for all error returns. This enables `errors.Is`/`errors.As` by callers.

## Testing conventions

- Unit tests are in `*_test.go` alongside source files
- Use `t.TempDir()` for filesystem tests — never write to real paths
- Integration tests use build tag `//go:build integration` and live in `test/`
- Run all: `go test ./...`
- Run with race detector: `go test -race ./...`
- Run integration: `go test -tags=integration ./test -v`

## What's implemented vs planned

### Implemented (Phases 0-2)

- All core types and IDs with UUID generation
- Storage interfaces and filesystem implementations
- Gateway with per-session FIFO queue and global concurrency semaphore
- Retry policy with exponential backoff (1s/2x/3 attempts/30s cap)
- LLM provider interface with OpenAI-compatible client
- Config loader with env override support
- CLI entry point with graceful shutdown
- 31 tests (30 unit + 1 integration)

### Not yet implemented (Phases 3-7)

- **Runtime** (Phase 3): LLM turn loop, tool execution, tool registry, digest + artifact persistence during runs. The gateway's `Queue.processor` is currently set externally — Phase 3 will wire in the real runtime.
- **Context engine** (Phase 4): Token-budgeted prompt assembly from agent memory, recent events, tool digests, artifact excerpts. The `ContextManager` interface from the design doc is not yet coded.
- **Telegram adapter** (Phase 5): Convert Telegram updates to `InboundEvent`, send responses back, handle `/status`, `/new`, `/reset`, `/compact` commands.
- **Automations** (Phase 6): Internal scheduler emitting trigger events into the gateway queue. `AutomationID` type exists but no store or scheduler yet.
- **Hardening** (Phase 7): Structured logging, metrics, startup recovery, graceful queue draining, integration tests.

## Known technical debt

1. **EventStore.Append is O(n)** — counts all lines on every append to assign sequence numbers. Should cache counts in memory.
2. **Queue lane goroutines never cleaned up** — dormant sessions retain goroutines. Needs idle reaping.
3. **SetProcessor is not thread-safe** — must be called before Start. Should accept processor in NewQueue constructor.
4. **RetryPolicy.Execute ignores context** — uses `time.Sleep` instead of context-aware timers.
5. **Retry error classification uses string matching** — should use sentinel types or `errors.As`.
6. **SessionStore.Get is O(n)** — linear scan over all sessions. Should add a reverse index by SessionID.
7. **No config validation** — missing API key or zero MaxConcurrent not caught at startup.

## Design documents

- `docs/plans/2024-11-21-phases-0-2-design.md` — Architecture decisions, data models, context engineering strategy
- `docs/plans/2024-11-21-phases-0-2-implementation.md` — Task-by-task implementation plan with code

Read the design doc first for the full vision including context engineering (the primary differentiator), run lifecycle, and the automation model.
