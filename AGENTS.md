# AGENTS.md — LLM Developer Guide

This file helps AI coding assistants understand the gopherclaw codebase quickly. Read this before making changes.

## What is gopherclaw?

A Go assistant runtime that receives messages (from Telegram or automations), routes them through a gateway, executes LLM tool loops, and persists everything to the filesystem. Think of it as a self-hosted agent framework with context engineering as the primary differentiator.

## Package dependency graph

```
cmd/gopherclaw/
  ├── internal/config         (config loading, get/set, flatten)
  ├── internal/gateway        (orchestration, queue)
  │     └── internal/types    (interfaces)
  ├── internal/state          (storage: session, event, artifact, task)
  │     └── internal/types    (interfaces, models)
  ├── internal/runtime        (agentic turn loop, tool registry)
  │     ├── internal/runtime/tools  (bash, brave_search, read_url, memory_*)
  │     ├── internal/context  (token-budgeted prompt builder)
  │     └── pkg/llm           (LLM provider)
  ├── internal/telegram       (Telegram bot adapter)
  ├── internal/webhook        (HTTP server: debug UI, API, webhooks)
  ├── internal/scheduler      (cron-based task scheduler)
  └── internal/delivery       (response routing by session key prefix)
```

No circular dependencies. `internal/types` is the shared contract layer. `internal/state` implements storage. `internal/gateway` consumes storage via interfaces. `internal/runtime` wires the LLM turn loop into the gateway's queue processor.

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

**"Where is the runtime?"** → `internal/runtime/runtime.go` (ProcessRun agentic turn loop)

**"Where are the tools?"** → `internal/runtime/tools/` (bash.go, brave.go, readurl.go, memory.go)

**"Where is the context engine?"** → `internal/context/engine.go` (token-budgeted prompt builder)

**"Where is the system prompt?"** → `internal/context/prompt.go` (DefaultPrompt template)

**"Where is the Telegram adapter?"** → `internal/telegram/adapter.go` (long polling, commands)

**"Where is the HTTP server?"** → `internal/webhook/server.go` (debug UI, API, webhooks)

**"Where is the debug UI?"** → `internal/webhook/static/index.html` (embedded via `//go:embed`)

**"Where is the scheduler?"** → `internal/scheduler/scheduler.go` (cron-based task firing)

**"Where is delivery routing?"** → `internal/delivery/registry.go` (prefix-based response routing)

**"Where are CLI commands?"** → `cmd/gopherclaw/cmd_*.go` (serve, config, session, task, setup, lifecycle)

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

### Implemented (Phases 0-6)

- All core types and IDs with UUID generation
- Storage interfaces and filesystem implementations (session, event, artifact, task)
- Gateway with per-session FIFO queue and global concurrency semaphore
- Retry policy with exponential backoff (1s/2x/3 attempts/30s cap)
- LLM provider interface with OpenAI-compatible client
- Config loader with env override, CLI get/set, flatten/unflatten
- Agentic turn loop runtime with tool execution and max-rounds handling
- Tool registry with built-in tools: bash, brave_search, read_url, memory_save/delete/list
- Token-budgeted context engine with tiktoken, history walkback, memory injection
- Customizable system prompt template
- Telegram adapter with long polling, typing indicators, message splitting
- Telegram commands: /start, /new, /status, /context, /memories
- CLI commands: serve, stop, restart, config (list/get/set), session (list/clear), task (add/list/remove/enable/disable), setup wizard
- Graceful shutdown (SIGINT/SIGTERM) and restart (SIGHUP with in-flight request draining)
- PID file management
- Cron-based task scheduler with delivery routing
- HTTP webhook server (ad-hoc and named task endpoints)
- Debug web UI with session list, event viewer, artifact loading (embedded HTML via `//go:embed`)
- JSON API: /api/sessions, /api/sessions/{id}/events, /api/artifacts/{id}

### Not yet implemented (Phase 7)

- **Hardening**: Structured logging improvements, metrics/telemetry, startup recovery, packaging

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
- `docs/plans/2026-02-22-phases-3-5-design.md` — Runtime, context engine, Telegram adapter design
- `docs/plans/2026-02-22-phases-3-5-implementation.md` — Phases 3-5 implementation plan
- `docs/plans/2026-02-22-cli-commands-design.md` — CLI commands design
- `docs/plans/2026-02-22-cli-commands-implementation.md` — CLI commands implementation plan
- `docs/plans/2026-02-25-debug-web-ui-design.md` — Debug web UI design
- `docs/plans/2026-02-25-debug-web-ui-implementation.md` — Debug web UI implementation plan

Read the original design doc first for the full vision including context engineering (the primary differentiator), run lifecycle, and the automation model.
