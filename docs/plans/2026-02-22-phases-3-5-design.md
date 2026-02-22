# Gopherclaw Phases 3-5 Design

## Overview

Design for phases 3-5: Runtime (agentic turn loop with tools), Context Engine (token-budgeted prompt assembly), and Telegram Adapter (inbound/outbound messaging). Builds directly on the Phase 0-2 foundation without refactoring.

## Architecture Decisions

### Tools: Bash, Brave Search, Read URL

Three built-in tools ship with the runtime:

- **bash** — execute shell commands on the host, no restrictions (self-hosted, trust the LLM)
- **brave_search** — web search via Brave Search API (requires API key, 2000 free queries/month)
- **read_url** — fetch URL and convert HTML to markdown via `html-to-markdown`

**Rationale:** Makes gopherclaw immediately useful as a system-interacting assistant. Brave Search has a generous free tier with clean JSON API. Markdown output from read_url is the format LLMs handle best.

### Token Counting: tiktoken-go

Accurate token counting using the Go port of OpenAI's tokenizer.

**Rationale:** Context budgeting decisions (what to include/exclude) benefit from accurate counts. Prevents wasted context or unexpected truncation.

### Telegram: go-telegram-bot-api

Long-polling Telegram bot using the most popular Go Telegram library.

**Rationale:** Battle-tested, handles polling cleanly, API maps directly to our needs.

### Response Delivery: Completion Callback

The Run struct carries an `OnComplete` callback invoked when the runtime finishes processing. The Telegram adapter provides a callback that sends the response back to the originating chat.

**Rationale:** Avoids polling EventStore for new messages. Direct, low-latency, simple.

## New Dependencies

```
github.com/go-telegram-bot-api/telegram-bot-api/v5   # Telegram bot
github.com/JohannesKaufmann/html-to-markdown/v2       # HTML→Markdown
github.com/pkoukk/tiktoken-go                         # Token counting
```

Brave Search uses `net/http` directly (simple JSON API, no SDK needed).

## New Package Layout

```
internal/
├── runtime/              # Phase 3: Turn loop + tool execution
│   ├── runtime.go        # Runtime.ProcessRun — the agentic loop
│   ├── tool.go           # Tool interface + registry
│   └── tools/
│       ├── bash.go       # Shell command execution
│       ├── brave.go      # Brave Search API
│       └── readurl.go    # URL fetch + HTML→Markdown
├── context/              # Phase 4: Token-budgeted prompt assembly
│   └── engine.go         # Engine.BuildPrompt
└── telegram/             # Phase 5: Telegram adapter
    └── adapter.go        # Long-poll + response delivery
```

## Config Additions

```go
type Config struct {
    DataDir       string
    MaxConcurrent int
    MaxToolRounds int    // max LLM↔tool iterations per run (default: 10)
    LLM struct {
        Provider         string
        BaseURL          string
        APIKey           string
        Model            string
        MaxTokens        int
        Temperature      float32
        MaxContextTokens int  // model context window (default: 128000)
        OutputReserve    int  // tokens reserved for output (default: 4096)
    }
    Brave struct {
        APIKey string  // also BRAVE_API_KEY env var
    }
    Telegram struct {
        Token string   // also TELEGRAM_BOT_TOKEN env var
    }
}
```

---

## Phase 3: Runtime

### Tool Interface

```go
type Tool interface {
    Name() string
    Description() string
    Parameters() json.RawMessage  // JSON Schema
    Execute(ctx context.Context, args json.RawMessage) (string, error)
}
```

Each tool is self-describing — `Name`, `Description`, and `Parameters` map directly to the OpenAI function calling schema. `Execute` receives the raw JSON arguments from the LLM and returns a string result.

### Tool Registry

```go
type Registry struct {
    tools map[string]Tool
}

func (r *Registry) Register(t Tool)
func (r *Registry) Get(name string) (Tool, bool)
func (r *Registry) All() []Tool
func (r *Registry) AsLLMTools() []llm.Tool  // convert to provider format
```

### Runtime Structure

```go
type Runtime struct {
    provider  llm.Provider
    engine    *context.Engine
    events    types.EventStore
    artifacts types.ArtifactStore
    sessions  types.SessionStore
    registry  *Registry
    maxRounds int
}
```

### Turn Loop: ProcessRun

`ProcessRun` is the function passed to `Queue.SetProcessor`. It implements the agentic loop:

```
ProcessRun(run *Run) error:
  1. Record user_message event (from run.Event.Text)
  2. Load session via sessions.Get(run.SessionID)
  3. Load recent events via events.Tail(sessionID, limit)
  4. Build prompt via engine.BuildPrompt(session, events, artifacts)
  5. Call provider.Complete(messages, registry.AsLLMTools())
  6. If response.ToolCalls:
     a. For each tool call:
        - Record tool_call event (tool name + args in payload)
        - Execute tool via registry.Get(name).Execute(args)
        - If result > 2000 chars: store as artifact, use excerpt
        - Record tool_result event (result or artifact ref in payload)
     b. Loop back to step 3 (reload events to include tool results)
  7. If response.Content:
     - Record assistant_message event
     - Call run.OnComplete(response.Content) if callback set
     - Return nil
  8. If rounds >= maxRounds: record error event, return error
```

### Event Types Emitted

| Type | Source | Payload |
|------|--------|---------|
| `user_message` | adapter name | `{"text": "..."}` |
| `tool_call` | `runtime` | `{"tool": "bash", "call_id": "...", "arguments": {...}}` |
| `tool_result` | `runtime` | `{"tool": "bash", "call_id": "...", "result": "...", "artifact_id": "..."}` |
| `assistant_message` | `runtime` | `{"text": "..."}` |

### Built-in Tools

#### bash

Executes shell commands via `os/exec`. Returns combined stdout+stderr. Timeout: 120 seconds (configurable per-call via args).

```json
{
  "name": "bash",
  "description": "Execute a bash command on the host machine",
  "parameters": {
    "type": "object",
    "properties": {
      "command": { "type": "string", "description": "The command to execute" },
      "timeout_seconds": { "type": "integer", "description": "Timeout in seconds (default: 120)" }
    },
    "required": ["command"]
  }
}
```

#### brave_search

Calls Brave Search API (`https://api.search.brave.com/res/v1/web/search`). Returns top 5 results formatted as title + URL + snippet.

```json
{
  "name": "brave_search",
  "description": "Search the web using Brave Search",
  "parameters": {
    "type": "object",
    "properties": {
      "query": { "type": "string", "description": "Search query" },
      "count": { "type": "integer", "description": "Number of results (default: 5, max: 20)" }
    },
    "required": ["query"]
  }
}
```

#### read_url

Fetches a URL via `net/http`, converts HTML to markdown using `html-to-markdown`. Timeout: 30 seconds. Truncates output to 50000 characters.

```json
{
  "name": "read_url",
  "description": "Fetch a URL and return its content as markdown",
  "parameters": {
    "type": "object",
    "properties": {
      "url": { "type": "string", "description": "The URL to fetch" }
    },
    "required": ["url"]
  }
}
```

### Run Struct Changes

Add completion callback to the existing Run struct:

```go
type Run struct {
    // ... existing fields ...
    OnComplete func(response string)  // called when run finishes with assistant text
}
```

---

## Phase 4: Context Engine

### Engine Structure

```go
type Engine struct {
    tokenizer  *tiktoken.Codec
    maxTokens  int  // model context window
    reserve    int  // output token reserve
}

func New(model string, maxTokens, reserve int) (*Engine, error)
```

### BuildPrompt

```go
func (e *Engine) BuildPrompt(
    ctx context.Context,
    session *types.SessionIndex,
    events []*types.Event,
    artifacts types.ArtifactStore,
) ([]llm.Message, error)
```

### Token Budget Allocation

```
Total input budget = maxTokens - reserve
├── System prompt:     fixed (~500 tokens, measured)
├── Recent events:     70% of remaining budget
├── Artifact excerpts: 20% of remaining budget
└── Safety margin:     10% reserved
```

### Prompt Assembly Algorithm

1. **System prompt** — agent identity, current datetime, available tool names. Measured token count deducted from budget.
2. **Event conversion** — walk events newest-first, convert to `llm.Message`:
   - `user_message` → `{role: "user", content: payload.text}`
   - `assistant_message` → `{role: "assistant", content: payload.text}`
   - `tool_call` → `{role: "assistant", tool_calls: [...]}`
   - `tool_result` → `{role: "tool", content: payload.result}`
3. **Budget enforcement** — count tokens per message with tiktoken. Stop adding when event budget is exhausted.
4. **Chronological ordering** — reverse the collected messages back to chronological order.
5. **Artifact excerpts** — for tool_result events referencing artifacts (via `artifact_id`), fetch excerpts within artifact budget using `ArtifactStore.Excerpt`.

### System Prompt Template

```
You are a helpful assistant. Current time: {time}.
Session: {session_id}.
You have access to the following tools: {tool_list}.
```

Minimal — the LLM's capabilities do the heavy lifting. Agent-specific system prompts are a future enhancement.

---

## Phase 5: Telegram Adapter

### Adapter Structure

```go
type Adapter struct {
    bot      *tgbotapi.BotAPI
    gateway  *gateway.Gateway
    events   types.EventStore
    sessions types.SessionStore
}

func New(token string, gateway *gateway.Gateway, events types.EventStore, sessions types.SessionStore) (*Adapter, error)
```

### Inbound Flow

1. Long-poll loop receives `tgbotapi.Update`
2. Filter: only process `Update.Message` with non-empty text (ignore edits, reactions, etc.)
3. Convert to `InboundEvent`:
   ```go
   event := &types.InboundEvent{
       Source:     "telegram",
       SessionKey: types.NewSessionKey("telegram",
                       strconv.FormatInt(msg.From.ID, 10),
                       strconv.FormatInt(msg.Chat.ID, 10)),
       UserID:     strconv.FormatInt(msg.From.ID, 10),
       Text:       msg.Text,
   }
   ```
4. Set `OnComplete` callback on the Run (via a new `HandleInboundWithCallback` method or by extending `HandleInbound`) that sends the response to `msg.Chat.ID`
5. Call `gateway.HandleInbound(ctx, event)`

### Outbound Flow

When the runtime calls `run.OnComplete(responseText)`:

1. Adapter's callback receives the response text and the originating chat ID
2. Split long messages at 4096 chars (Telegram limit)
3. Send via `bot.Send(tgbotapi.NewMessage(chatID, text))`
4. Use Markdown parse mode for formatted responses

### Gateway Changes for Callbacks

Extend `HandleInbound` to accept an optional callback:

```go
func (g *Gateway) HandleInbound(ctx context.Context, event *types.InboundEvent, opts ...RunOption) error

type RunOption func(*Run)

func WithOnComplete(fn func(string)) RunOption {
    return func(r *Run) { r.OnComplete = fn }
}
```

### Bot Commands

| Command | Action |
|---------|--------|
| `/start` | Welcome message, create session |
| `/new` | Archive current session, start fresh |
| `/status` | Show session info (message count, last activity) |

### Startup Integration

In `cmd/gopherclaw/main.go`:

```go
// After gateway.Start()
if cfg.Telegram.Token != "" {
    adapter, err := telegram.New(cfg.Telegram.Token, gw, eventStore, sessionStore)
    // ...
    go adapter.Start(ctx)
}
```

The adapter starts in a goroutine alongside the gateway. Both respect the shared context for graceful shutdown.

---

## Testing Strategy

### Phase 3 Tests
- **Tool registry**: register, lookup, list, convert to LLM format
- **Each tool**: bash (command execution, timeout), brave_search (mock HTTP), read_url (mock HTTP, HTML→MD)
- **Turn loop**: mock LLM provider returning tool calls then text, verify event sequence
- **Max rounds**: verify loop terminates and records error

### Phase 4 Tests
- **Token counting**: verify accurate counts against known strings
- **Budget allocation**: verify system prompt + events fit within budget
- **Event conversion**: each event type maps to correct message role
- **Truncation**: verify oldest events dropped when budget exceeded
- **Artifact excerpts**: verify excerpts included within artifact budget

### Phase 5 Tests
- **InboundEvent creation**: verify SessionKey format from Telegram update
- **OnComplete callback**: verify response sent to correct chat ID
- **Message splitting**: verify long messages split at 4096 chars
- **Bot commands**: verify /start, /new, /status handling

### Integration Test Updates
- Extend existing integration test to exercise full flow: InboundEvent → Runtime → LLM (mocked) → Tool execution → Response callback

---

## Wiring Summary

```
main.go:
  config := config.Load(path)
  stores := state.New(config.DataDir)
  provider := openai.New(config.LLM)

  engine := context.New(config.LLM.Model, config.LLM.MaxContextTokens, config.LLM.OutputReserve)

  registry := runtime.NewRegistry()
  registry.Register(tools.NewBash())
  registry.Register(tools.NewBraveSearch(config.Brave.APIKey))
  registry.Register(tools.NewReadURL())

  rt := runtime.New(provider, engine, stores, registry, config.MaxToolRounds)

  gw := gateway.New(stores, int64(config.MaxConcurrent))
  gw.Queue.SetProcessor(rt.ProcessRun)
  gw.Start(ctx)

  if config.Telegram.Token != "" {
      adapter := telegram.New(config.Telegram.Token, gw, stores)
      go adapter.Start(ctx)
  }
```

## Success Criteria

1. Send a message via Telegram → receive an LLM response back
2. LLM can execute bash commands and return results
3. LLM can search the web and read URLs
4. Conversation history persists across restarts
5. Token budget prevents context overflow
6. Concurrent sessions from different Telegram chats work correctly
7. Graceful shutdown drains in-flight runs before exiting
