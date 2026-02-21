# gopherclaw

A single-binary Go assistant runtime with filesystem-first state, gateway-centric orchestration, and context-engineered LLM interactions.

## Status

**Phases 0-2 complete** — core contracts, storage layer, and gateway orchestration are implemented. Phases 3-7 (runtime, context engine, Telegram adapter, automations, hardening) are planned.

## Architecture

```
cmd/gopherclaw/          CLI entry point (daemon)
internal/
  types/                 Core ID types, data models, storage interfaces
  state/                 Filesystem-backed SessionStore, EventStore, ArtifactStore
  gateway/               Gateway orchestrator, per-session FIFO queue, retry policy
  config/                Config loader (defaults → file → env vars)
pkg/
  llm/                   Provider interface and types
  llm/openai/            OpenAI-compatible client implementation
```

### Key design decisions

- **Filesystem-first state**: Sessions, events, and artifacts live in `~/.gopherclaw/` as JSON/JSONL files. No database required. Everything is inspectable with standard tools.
- **Append-only events**: Session history is an append-only JSONL log with auto-incrementing sequence numbers. Tool outputs are stored as separate artifact files, referenced by ID from event digests.
- **Per-session FIFO with global concurrency**: Each session gets strict in-order processing. A global semaphore caps total parallel runs across sessions.
- **Atomic writes**: All index/config updates use temp-file-plus-rename for crash safety.
- **OpenAI-compatible provider**: The LLM client targets any OpenAI-compatible API via configurable base URL.

## Build

```bash
make build    # → bin/gopherclaw
make test     # unit tests
make clean    # remove bin/

# integration tests
go test -tags=integration ./test -v
```

## Configuration

Config is loaded with precedence: **defaults → config file → environment variables**.

```bash
# Environment variables (highest precedence)
export OPENAI_API_KEY=sk-...
export OPENAI_BASE_URL=https://api.openai.com

# Or use a config file
cat ~/.gopherclaw/config.json
```

```json
{
  "data_dir": "/home/user/.gopherclaw",
  "max_concurrent": 2,
  "llm": {
    "provider": "openai",
    "base_url": "https://api.openai.com",
    "model": "gpt-4",
    "max_tokens": 2000,
    "temperature": 0.7
  }
}
```

## Run

```bash
./bin/gopherclaw                              # default config
./bin/gopherclaw --config /path/to/config.json  # custom config
```

The daemon starts the gateway, prints status, and waits for SIGINT/SIGTERM to shut down gracefully.

## Data layout

```
~/.gopherclaw/
├── config.json
├── sessions/
│   ├── sessions.json                 # session index
│   └── <sessionID>/
│       ├── events.jsonl              # append-only event log
│       └── artifacts/
│           └── <artifactID>.json     # full tool outputs
├── agents/<agent>/MEMORY.md          # (future) agent long memory
└── automations/automations.json      # (future) scheduled automations
```

## Roadmap

| Phase | Scope | Status |
|-------|-------|--------|
| 0 | Core contracts, types, LLM abstraction | Done |
| 1 | Filesystem-backed storage layer | Done |
| 2 | Gateway orchestration, queue, retry | Done |
| 3 | Runtime and tool execution loop | Planned |
| 4 | Context engine with token budgeting | Planned |
| 5 | Telegram adapter | Planned |
| 6 | Automations (internal scheduler) | Planned |
| 7 | Hardening, logging, packaging | Planned |

## Dependencies

- `github.com/google/uuid` — UUID generation for IDs
- `golang.org/x/sync` — Weighted semaphore for concurrency control
