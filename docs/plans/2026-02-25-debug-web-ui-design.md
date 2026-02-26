# Debug Web UI Design

**Date**: 2026-02-25
**Status**: Approved

## Goal

Add a browser-based debug UI that lets you view all chat sessions and their event history — an alternative to Telegram for inspecting what gopherclaw is doing.

## Architecture

Extend the existing webhook HTTP server (`internal/webhook/server.go`) with:

1. **JSON API endpoints** that read from SessionStore, EventStore, and ArtifactStore
2. **A single embedded HTML page** (`internal/webhook/static/index.html`) with inline CSS/JS, compiled into the binary via Go `embed`

The webhook server's `NewServer` constructor gains three additional store parameters.

### Routes

| Method | Path | Returns |
|--------|------|---------|
| `GET` | `/` | Debug UI HTML page (embedded) |
| `GET` | `/api/sessions` | JSON list of all sessions (sorted by updated_at desc, includes event counts) |
| `GET` | `/api/sessions/{id}/events` | JSON array of events for a session (`?limit=N`, default 200) |
| `GET` | `/api/artifacts/{id}` | Raw artifact JSON data |
| `GET` | `/health` | (existing) `{"status":"ok"}` |
| `POST` | `/webhook` | (existing) ad-hoc prompt |
| `POST` | `/webhook/{task}` | (existing) named task |

### API Response Shapes

**GET /api/sessions**:
```json
[
  {
    "session_id": "7c93e2a8-...",
    "session_key": "telegram:855...:855...",
    "status": "active",
    "created_at": "2026-02-25T15:33:18Z",
    "updated_at": "2026-02-25T17:45:00Z",
    "event_count": 42
  }
]
```

**GET /api/sessions/{id}/events?limit=200**:
```json
[
  {
    "id": "5c04d983-...",
    "seq": 1,
    "type": "user_message",
    "source": "telegram",
    "at": "2026-02-25T14:30:00Z",
    "payload": {"text": "What's hledger?"}
  }
]
```

**GET /api/artifacts/{id}**: Raw artifact data as stored.

## UI Layout

Single page, two-panel layout:

```
┌──────────────────────────────────────────────────────┐
│  gopherclaw debug                        [↻ Refresh] │
├─────────────────┬────────────────────────────────────┤
│ Sessions        │ Session header + metadata           │
│                 │────────────────────────────────────│
│ ● session 1     │ Scrollable conversation view:      │
│ ○ session 2     │ - User messages                    │
│ ○ session 3     │ - Assistant messages                │
│   ...           │ - Tool calls (collapsible)         │
│                 │ - Tool results (collapsible)        │
│                 │ - Artifacts (loaded on demand)      │
└─────────────────┴────────────────────────────────────┘
```

- **Left panel**: Session list showing key (truncated), event count, relative time, active/archived status
- **Right panel**: Session metadata header, then chronological event stream
- **Tool calls**: Rendered as `<details>` elements — summary shows tool name, expand for arguments/results
- **Artifacts**: Tool results referencing artifacts show a "Load artifact" link; data fetched lazily via `/api/artifacts/{id}`
- **Style**: Dark theme, monospace, functional

## Constraints

- Manual refresh only (no WebSocket/SSE/polling)
- No authentication (same as existing webhook server — local access only)
- All HTML/CSS/JS in a single embedded file
- No external dependencies or build tools for the frontend

## Changes Required

1. **`internal/webhook/server.go`**: Expand `NewServer` to accept stores; add API handlers; embed and serve static HTML
2. **`internal/webhook/static/index.html`**: New file — the entire debug UI
3. **`cmd/gopherclaw/cmd_serve.go`**: Pass stores to `NewServer`
4. **`internal/webhook/server_test.go`**: Update tests for new constructor and API endpoints
