# Cron & Webhook System Design

## Overview

Add the ability to schedule recurring prompts and trigger tasks via HTTP webhooks. Both cron and webhook tasks share the same model and flow through the existing gateway.

## Task Model

Tasks live in `~/.gopherclaw/tasks.json` as a JSON array. Each task:

```json
{
  "name": "morning-summary",
  "prompt": "Summarize any interesting Hacker News posts from the last 12 hours",
  "schedule": "0 8 * * *",
  "session_key": "telegram:12345:12345",
  "enabled": true
}
```

- **name**: unique identifier, doubles as the webhook path (`/webhook/morning-summary`)
- **prompt**: message sent to the LLM as if the user typed it
- **schedule**: standard 5-field cron expression; omit for webhook-only tasks
- **session_key**: which session to run in (ties to a Telegram chat for delivery)
- **enabled**: toggle without deleting

## Scheduler

A new `scheduler` package runs as a goroutine inside `serve`:

1. Loads `tasks.json` on startup
2. Parses cron expressions for tasks with a `schedule` field
3. On each tick, checks which tasks are due and submits an `InboundEvent` to the gateway with `Source: "cron"`
4. The session key on the task determines where the response is delivered

Uses `robfig/cron` for cron expression parsing.

Reloads `tasks.json` on changes (file watch or SIGHUP) so the daemon doesn't need restarting after adding tasks.

## Webhook HTTP Server

A minimal HTTP server starts alongside the Telegram adapter in `serve`:

- **`POST /webhook`** — ad-hoc prompt. Body: `{"prompt": "...", "session_key": "..."}`. Submits through gateway, waits for `OnComplete`, returns the response as JSON.
- **`POST /webhook/:name`** — runs a named task from `tasks.json`. Optional body to override prompt. Returns the response as JSON.
- **`GET /health`** — simple liveness check.

### Config

New `http` block in `config.json`:

```json
{
  "http": {
    "enabled": true,
    "listen": "127.0.0.1:8484"
  }
}
```

Defaults to localhost only for security. No auth initially since it's bound to localhost.

## CLI Subcommands

```
gopherclaw task add --name <name> --prompt <prompt> [--schedule <cron>] [--session-key <key>]
gopherclaw task list
gopherclaw task remove <name>
gopherclaw task enable <name>
gopherclaw task disable <name>
```

These directly read/write `tasks.json`. No daemon restart needed.

## Response Delivery

When a cron or webhook task runs:

1. Gateway routes it through the normal `ProcessRun` path
2. `OnComplete` callback delivers the response based on context:
   - **Cron tasks**: session key determines delivery. For `telegram:*` keys, the Telegram adapter sends the response to that chat. The LLM can decide not to respond (empty content = no message sent).
   - **Webhook requests**: HTTP handler holds the request open and returns the response in the body.
3. A delivery registry allows adapters to register as handlers for session key prefixes (e.g. Telegram registers for `telegram:*`).

## Components

| Component | Location | New/Modified |
|---|---|---|
| Task model + store | `internal/state/task.go` | New |
| Scheduler | `internal/scheduler/scheduler.go` | New |
| HTTP server | `internal/http/server.go` | New |
| CLI commands | `cmd/gopherclaw/cmd_task.go` | New |
| Config additions | `internal/config/config.go` | Modified |
| Delivery registry | `internal/gateway/gateway.go` | Modified |
| Serve wiring | `cmd/gopherclaw/cmd_serve.go` | Modified |
