# gopherclaw

A single-binary Go assistant runtime with filesystem-first state, gateway-centric orchestration, and context-engineered LLM interactions.

## Status

**Phases 0-6 complete** — core infrastructure, runtime, context engine, Telegram adapter, CLI, scheduled tasks, and debug web UI are all implemented and running.

## Architecture

```
cmd/gopherclaw/          CLI entry point (serve, config, session, task, setup, stop/restart)
internal/
  types/                 Core ID types, data models, storage interfaces
  state/                 Filesystem-backed SessionStore, EventStore, ArtifactStore, TaskStore
  gateway/               Gateway orchestrator, per-session FIFO queue, retry policy
  runtime/               Agentic turn loop, tool registry, tool execution
  runtime/tools/         Built-in tools (bash, brave_search, read_url, memory_*)
  context/               Token-budgeted prompt assembly with memory injection
  config/                Config loader with flatten/unflatten and CLI get/set
  telegram/              Telegram bot adapter with long polling
  webhook/               HTTP server (debug UI, JSON API, webhooks)
  webhook/static/        Embedded HTML debug UI
  scheduler/             Cron-based task scheduler
  delivery/              Response delivery routing (Telegram, etc.)
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
go build -o gopherclaw ./cmd/gopherclaw/
go test ./...
```

## Configuration

Config is loaded with precedence: **defaults → config file → environment variables**.

```bash
# Environment variables (highest precedence)
export OPENAI_API_KEY=sk-...
export OPENAI_BASE_URL=https://api.openai.com/v1
export TELEGRAM_BOT_TOKEN=...
export BRAVE_API_KEY=...

# Or manage config via CLI
gopherclaw config list
gopherclaw config set llm.model gpt-4
gopherclaw config get llm.model
```

A default config file is created at `~/.gopherclaw/config.json` on first run. Key settings:

```json
{
  "data_dir": "/home/user/.gopherclaw",
  "log_level": "info",
  "max_concurrent": 2,
  "max_tool_rounds": 10,
  "llm": {
    "provider": "openai",
    "base_url": "https://api.openai.com/v1",
    "model": "gpt-4",
    "max_tokens": 2000,
    "temperature": 0.7,
    "max_context_tokens": 128000,
    "output_reserve": 4096
  },
  "telegram": { "token": "" },
  "brave": { "api_key": "" },
  "http": { "enabled": true, "listen": "127.0.0.1:8484" }
}
```

## Run

```bash
gopherclaw serve                                # start daemon
gopherclaw stop                                 # stop daemon
gopherclaw restart                              # graceful restart (SIGHUP)
gopherclaw setup                                # interactive setup wizard
```

The daemon starts the gateway, Telegram adapter, task scheduler, and HTTP server, then waits for SIGINT/SIGTERM to shut down gracefully. SIGHUP triggers a graceful restart (drains in-flight requests, then re-execs).

## Debug Web UI

When `http.enabled` is true, a debug web UI is served at the HTTP listen address (default `http://localhost:8484/`). It provides:

- Session list with event counts and status
- Conversation viewer with full event history
- Collapsible tool call/result blocks
- Lazy artifact loading
- JSON API at `/api/sessions`, `/api/sessions/{id}/events`, `/api/artifacts/{id}`

## Scheduled Tasks

```bash
gopherclaw task list
gopherclaw task add --name daily-summary --prompt "Summarize today" --schedule "0 18 * * *" --session-key "telegram:USER:CHAT"
gopherclaw task remove daily-summary
gopherclaw task enable daily-summary
gopherclaw task disable daily-summary
```

Tasks use standard cron syntax. Scheduled task responses are delivered to the session key's channel (e.g. Telegram chat). Webhook-only tasks (no schedule) can be triggered via `POST /webhook/{name}`. After adding/changing scheduled tasks, restart the daemon.

## Data layout

```
~/.gopherclaw/
├── config.json                       # configuration
├── gopherclaw.pid                    # daemon PID file
├── memory.md                         # persistent agent memory
├── tasks.json                        # scheduled/webhook task definitions
├── sessions/
│   ├── sessions.json                 # session index
│   └── <sessionID>/
│       ├── events.jsonl              # append-only event log
│       └── artifacts/
│           └── <artifactID>.json     # full tool outputs
```

## Roadmap

| Phase | Scope | Status |
|-------|-------|--------|
| 0 | Core contracts, types, LLM abstraction | Done |
| 1 | Filesystem-backed storage layer | Done |
| 2 | Gateway orchestration, queue, retry | Done |
| 3 | Runtime, tool execution loop, tool registry | Done |
| 4 | Context engine with token budgeting | Done |
| 5 | Telegram adapter with commands | Done |
| 6 | CLI commands, scheduled tasks, webhooks, debug web UI | Done |
| 7 | Hardening, logging, packaging | Planned |

## Dependencies

- `github.com/google/uuid` — UUID generation
- `github.com/spf13/cobra` — CLI framework
- `github.com/go-telegram-bot-api/telegram-bot-api/v5` — Telegram bot API
- `github.com/pkoukk/tiktoken-go` — Token counting for context budgeting
- `github.com/JohannesKaufmann/html-to-markdown/v2` — HTML→Markdown for read_url tool
- `github.com/robfig/cron/v3` — Cron expression parsing for task scheduler
- `golang.org/x/sync` — Weighted semaphore for concurrency control
