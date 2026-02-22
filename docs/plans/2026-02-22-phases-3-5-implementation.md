# Phases 3-5 Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement agentic runtime with tools (bash, brave search, read URL), token-budgeted context engine, and Telegram adapter.

**Architecture:** Runtime plugs into existing `Queue.SetProcessor` as the agentic turn loop. Context engine assembles token-budgeted prompts via tiktoken. Telegram adapter converts updates to `InboundEvent` and delivers responses via completion callbacks on `Run`.

**Tech Stack:** Go, tiktoken-go, html-to-markdown v2, go-telegram-bot-api v5, Brave Search REST API

---

### Task 1: Config Additions

**Files:**
- Modify: `internal/config/config.go`

**Step 1: Add new config fields**

Add `MaxToolRounds`, `Brave`, `Telegram`, and LLM context fields to the Config struct:

```go
type Config struct {
	DataDir       string `json:"data_dir"`
	MaxConcurrent int    `json:"max_concurrent"`
	MaxToolRounds int    `json:"max_tool_rounds"`
	LLM           struct {
		Provider         string  `json:"provider"`
		BaseURL          string  `json:"base_url"`
		APIKey           string  `json:"api_key"`
		Model            string  `json:"model"`
		MaxTokens        int     `json:"max_tokens"`
		Temperature      float32 `json:"temperature"`
		MaxContextTokens int     `json:"max_context_tokens"`
		OutputReserve    int     `json:"output_reserve"`
	} `json:"llm"`
	Brave struct {
		APIKey string `json:"api_key"`
	} `json:"brave"`
	Telegram struct {
		Token string `json:"token"`
	} `json:"telegram"`
}
```

**Step 2: Set defaults in Load**

In the `Load` function, after the existing defaults, add:

```go
cfg.MaxToolRounds = 10
cfg.LLM.MaxContextTokens = 128000
cfg.LLM.OutputReserve = 4096
```

**Step 3: Add env var overrides**

After the existing env overrides, add:

```go
if braveKey := os.Getenv("BRAVE_API_KEY"); braveKey != "" {
    cfg.Brave.APIKey = braveKey
}
if tgToken := os.Getenv("TELEGRAM_BOT_TOKEN"); tgToken != "" {
    cfg.Telegram.Token = tgToken
}
```

**Step 4: Run tests**

Run: `go test ./internal/config/... -v`
Expected: Existing tests still pass (config has no tests yet, but build must succeed)

Run: `go build ./...`
Expected: Clean build

**Step 5: Commit**

```bash
git add internal/config/config.go
git commit -m "feat: add config fields for runtime, brave, telegram, context"
```

---

### Task 2: Run OnComplete Callback + Gateway RunOption

**Files:**
- Modify: `internal/gateway/run.go:20-42`
- Modify: `internal/gateway/gateway.go:56-65`
- Test: `internal/gateway/gateway_test.go`

**Step 1: Write the failing test**

Add to `internal/gateway/gateway_test.go`:

```go
func TestHandleInboundWithOnComplete(t *testing.T) {
	dir := t.TempDir()
	sessions := state.NewSessionStore(dir)
	events := state.NewEventStore(dir)
	artifacts := state.NewArtifactStore(dir)
	gw := New(sessions, events, artifacts)

	ctx := context.Background()
	gw.Start(ctx)
	defer gw.Stop()

	var callbackResult string
	var mu sync.Mutex
	done := make(chan struct{})

	gw.Queue.SetProcessor(func(run *Run) error {
		if run.OnComplete != nil {
			run.OnComplete("hello from processor")
		}
		return nil
	})

	event := &types.InboundEvent{
		Source:     "test",
		SessionKey: types.NewSessionKey("test", "user1"),
		UserID:     "user1",
		Text:       "hi",
	}

	err := gw.HandleInbound(ctx, event, WithOnComplete(func(resp string) {
		mu.Lock()
		callbackResult = resp
		mu.Unlock()
		close(done)
	}))
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for callback")
	}

	mu.Lock()
	defer mu.Unlock()
	if callbackResult != "hello from processor" {
		t.Errorf("expected 'hello from processor', got %q", callbackResult)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/gateway -run TestHandleInboundWithOnComplete -v`
Expected: FAIL — `WithOnComplete` undefined, `OnComplete` field missing

**Step 3: Add OnComplete to Run struct**

In `internal/gateway/run.go`, add the field to the Run struct after line 29:

```go
type Run struct {
	ID         types.RunID
	SessionID  types.SessionID
	Event      *types.InboundEvent
	Status     RunStatus
	Attempts   int
	CreatedAt  time.Time
	StartedAt  *time.Time
	EndedAt    *time.Time
	Error      error
	OnComplete func(response string)
}
```

**Step 4: Add RunOption and WithOnComplete**

In `internal/gateway/gateway.go`, add the RunOption type and modify HandleInbound:

```go
// RunOption configures optional behavior on a Run.
type RunOption func(*Run)

// WithOnComplete sets a callback invoked when the run produces a final response.
func WithOnComplete(fn func(string)) RunOption {
	return func(r *Run) { r.OnComplete = fn }
}

// HandleInbound resolves or creates a session for the event, wraps it in a
// Run, and enqueues it for processing.
func (g *Gateway) HandleInbound(ctx context.Context, event *types.InboundEvent, opts ...RunOption) error {
	sessionID, err := g.sessions.ResolveOrCreate(ctx, event.SessionKey, "default")
	if err != nil {
		return fmt.Errorf("resolve session: %w", err)
	}
	run := NewRun(sessionID, event)
	for _, opt := range opts {
		opt(run)
	}
	return g.Queue.Enqueue(run)
}
```

**Step 5: Run test to verify it passes**

Run: `go test ./internal/gateway -run TestHandleInboundWithOnComplete -v`
Expected: PASS

**Step 6: Run all tests**

Run: `go test ./... -v`
Expected: All pass (existing integration test doesn't use opts, variadic is backwards-compatible)

**Step 7: Commit**

```bash
git add internal/gateway/run.go internal/gateway/gateway.go internal/gateway/gateway_test.go
git commit -m "feat: add OnComplete callback to Run with RunOption pattern"
```

---

### Task 3: Tool Interface + Registry

**Files:**
- Create: `internal/runtime/tool.go`
- Test: `internal/runtime/tool_test.go`

**Step 1: Write the failing test**

Create `internal/runtime/tool_test.go`:

```go
package runtime

import (
	"context"
	"encoding/json"
	"testing"
)

type echoTool struct{}

func (e *echoTool) Name() string                                              { return "echo" }
func (e *echoTool) Description() string                                       { return "Echoes input" }
func (e *echoTool) Parameters() json.RawMessage                               { return json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}},"required":["text"]}`) }
func (e *echoTool) Execute(_ context.Context, args json.RawMessage) (string, error) {
	var p struct{ Text string `json:"text"` }
	json.Unmarshal(args, &p)
	return p.Text, nil
}

func TestRegistryRegisterAndGet(t *testing.T) {
	r := NewRegistry()
	r.Register(&echoTool{})

	tool, ok := r.Get("echo")
	if !ok {
		t.Fatal("expected to find echo tool")
	}
	if tool.Name() != "echo" {
		t.Errorf("expected name 'echo', got %q", tool.Name())
	}
}

func TestRegistryGetMissing(t *testing.T) {
	r := NewRegistry()
	_, ok := r.Get("missing")
	if ok {
		t.Fatal("expected not to find missing tool")
	}
}

func TestRegistryAll(t *testing.T) {
	r := NewRegistry()
	r.Register(&echoTool{})
	tools := r.All()
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
}

func TestRegistryAsLLMTools(t *testing.T) {
	r := NewRegistry()
	r.Register(&echoTool{})
	llmTools := r.AsLLMTools()
	if len(llmTools) != 1 {
		t.Fatalf("expected 1 llm tool, got %d", len(llmTools))
	}
	if llmTools[0].Function.Name != "echo" {
		t.Errorf("expected function name 'echo', got %q", llmTools[0].Function.Name)
	}
	if llmTools[0].Type != "function" {
		t.Errorf("expected type 'function', got %q", llmTools[0].Type)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/runtime -v`
Expected: FAIL — package does not exist

**Step 3: Implement tool.go**

Create `internal/runtime/tool.go`:

```go
package runtime

import (
	"context"
	"encoding/json"

	"github.com/user/gopherclaw/pkg/llm"
)

// Tool defines the interface for an executable tool.
type Tool interface {
	Name() string
	Description() string
	Parameters() json.RawMessage
	Execute(ctx context.Context, args json.RawMessage) (string, error)
}

// Registry holds registered tools and provides lookup.
type Registry struct {
	tools map[string]Tool
}

// NewRegistry creates an empty tool registry.
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register adds a tool to the registry.
func (r *Registry) Register(t Tool) {
	r.tools[t.Name()] = t
}

// Get returns a tool by name.
func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// All returns all registered tools.
func (r *Registry) All() []Tool {
	out := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t)
	}
	return out
}

// AsLLMTools converts registered tools to the LLM provider format.
func (r *Registry) AsLLMTools() []llm.Tool {
	out := make([]llm.Tool, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, llm.Tool{
			Type: "function",
			Function: llm.Function{
				Name:        t.Name(),
				Description: t.Description(),
				Parameters:  t.Parameters(),
			},
		})
	}
	return out
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/runtime -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/runtime/tool.go internal/runtime/tool_test.go
git commit -m "feat: add Tool interface and Registry"
```

---

### Task 4: Bash Tool

**Files:**
- Create: `internal/runtime/tools/bash.go`
- Test: `internal/runtime/tools/bash_test.go`

**Step 1: Write the failing test**

Create `internal/runtime/tools/bash_test.go`:

```go
package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestBashName(t *testing.T) {
	b := NewBash()
	if b.Name() != "bash" {
		t.Errorf("expected 'bash', got %q", b.Name())
	}
}

func TestBashExecuteSimple(t *testing.T) {
	b := NewBash()
	args, _ := json.Marshal(map[string]string{"command": "echo hello"})
	result, err := b.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(result) != "hello" {
		t.Errorf("expected 'hello', got %q", result)
	}
}

func TestBashExecuteStderr(t *testing.T) {
	b := NewBash()
	args, _ := json.Marshal(map[string]string{"command": "echo err >&2"})
	result, err := b.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "err") {
		t.Errorf("expected stderr output, got %q", result)
	}
}

func TestBashExecuteTimeout(t *testing.T) {
	b := NewBash()
	args, _ := json.Marshal(map[string]any{"command": "sleep 10", "timeout_seconds": 1})
	start := time.Now()
	_, err := b.Execute(context.Background(), args)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if elapsed > 3*time.Second {
		t.Errorf("timeout took too long: %v", elapsed)
	}
}

func TestBashExecuteExitCode(t *testing.T) {
	b := NewBash()
	args, _ := json.Marshal(map[string]string{"command": "exit 1"})
	_, err := b.Execute(context.Background(), args)
	if err == nil {
		t.Fatal("expected error for non-zero exit")
	}
}

func TestBashParameters(t *testing.T) {
	b := NewBash()
	var schema map[string]any
	if err := json.Unmarshal(b.Parameters(), &schema); err != nil {
		t.Fatal(err)
	}
	if schema["type"] != "object" {
		t.Errorf("expected object schema, got %v", schema["type"])
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/runtime/tools -v`
Expected: FAIL — package does not exist

**Step 3: Implement bash.go**

Create `internal/runtime/tools/bash.go`:

```go
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

// Bash executes shell commands on the host.
type Bash struct{}

// NewBash creates a new Bash tool.
func NewBash() *Bash { return &Bash{} }

func (b *Bash) Name() string        { return "bash" }
func (b *Bash) Description() string { return "Execute a bash command on the host machine" }
func (b *Bash) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"command": {"type": "string", "description": "The command to execute"},
			"timeout_seconds": {"type": "integer", "description": "Timeout in seconds (default: 120)"}
		},
		"required": ["command"]
	}`)
}

func (b *Bash) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Command        string `json:"command"`
		TimeoutSeconds int    `json:"timeout_seconds"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("parse args: %w", err)
	}
	if params.Command == "" {
		return "", fmt.Errorf("command is required")
	}

	timeout := 120 * time.Second
	if params.TimeoutSeconds > 0 {
		timeout = time.Duration(params.TimeoutSeconds) * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", params.Command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("command failed: %w\nOutput: %s", err, string(output))
	}
	return string(output), nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/runtime/tools -run TestBash -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/runtime/tools/bash.go internal/runtime/tools/bash_test.go
git commit -m "feat: add bash tool"
```

---

### Task 5: Brave Search Tool

**Files:**
- Create: `internal/runtime/tools/brave.go`
- Test: `internal/runtime/tools/brave_test.go`

**Step 1: Write the failing test**

Add to `internal/runtime/tools/brave_test.go`:

```go
package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBraveSearchName(t *testing.T) {
	b := NewBraveSearch("test-key")
	if b.Name() != "brave_search" {
		t.Errorf("expected 'brave_search', got %q", b.Name())
	}
}

func TestBraveSearchExecute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Subscription-Token") != "test-key" {
			t.Error("missing API key header")
		}
		if r.URL.Query().Get("q") != "golang testing" {
			t.Errorf("unexpected query: %s", r.URL.Query().Get("q"))
		}
		json.NewEncoder(w).Encode(braveResponse{
			Web: braveWeb{
				Results: []braveResult{
					{Title: "Go Testing", URL: "https://go.dev/testing", Description: "How to test in Go"},
					{Title: "Go Docs", URL: "https://go.dev/doc", Description: "Go documentation"},
				},
			},
		})
	}))
	defer server.Close()

	b := NewBraveSearch("test-key")
	b.baseURL = server.URL

	args, _ := json.Marshal(map[string]any{"query": "golang testing", "count": 2})
	result, err := b.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "Go Testing") {
		t.Errorf("expected 'Go Testing' in result, got %q", result)
	}
	if !strings.Contains(result, "https://go.dev/testing") {
		t.Errorf("expected URL in result, got %q", result)
	}
}

func TestBraveSearchNoResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(braveResponse{})
	}))
	defer server.Close()

	b := NewBraveSearch("test-key")
	b.baseURL = server.URL

	args, _ := json.Marshal(map[string]string{"query": "xyznonexistent"})
	result, err := b.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "No results") {
		t.Errorf("expected 'No results', got %q", result)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/runtime/tools -run TestBraveSearch -v`
Expected: FAIL — `NewBraveSearch` undefined

**Step 3: Implement brave.go**

Create `internal/runtime/tools/brave.go`:

```go
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// BraveSearch searches the web via Brave Search API.
type BraveSearch struct {
	apiKey string
	baseURL string
	client *http.Client
}

// NewBraveSearch creates a new Brave Search tool.
func NewBraveSearch(apiKey string) *BraveSearch {
	return &BraveSearch{
		apiKey:  apiKey,
		baseURL: "https://api.search.brave.com/res/v1/web/search",
		client:  &http.Client{Timeout: 15 * time.Second},
	}
}

func (b *BraveSearch) Name() string        { return "brave_search" }
func (b *BraveSearch) Description() string { return "Search the web using Brave Search" }
func (b *BraveSearch) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {"type": "string", "description": "Search query"},
			"count": {"type": "integer", "description": "Number of results (default: 5, max: 20)"}
		},
		"required": ["query"]
	}`)
}

type braveResponse struct {
	Web braveWeb `json:"web"`
}

type braveWeb struct {
	Results []braveResult `json:"results"`
}

type braveResult struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
}

func (b *BraveSearch) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Query string `json:"query"`
		Count int    `json:"count"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("parse args: %w", err)
	}
	if params.Query == "" {
		return "", fmt.Errorf("query is required")
	}
	if params.Count <= 0 {
		params.Count = 5
	}
	if params.Count > 20 {
		params.Count = 20
	}

	u, _ := url.Parse(b.baseURL)
	q := u.Query()
	q.Set("q", params.Query)
	q.Set("count", fmt.Sprintf("%d", params.Count))
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", b.apiKey)

	resp, err := b.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("search request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Brave API error (status %d): %s", resp.StatusCode, string(body))
	}

	var result braveResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}

	if len(result.Web.Results) == 0 {
		return "No results found.", nil
	}

	var sb strings.Builder
	for i, r := range result.Web.Results {
		fmt.Fprintf(&sb, "%d. %s\n   %s\n   %s\n\n", i+1, r.Title, r.URL, r.Description)
	}
	return sb.String(), nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/runtime/tools -run TestBraveSearch -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/runtime/tools/brave.go internal/runtime/tools/brave_test.go
git commit -m "feat: add brave search tool"
```

---

### Task 6: Read URL Tool

**Files:**
- Create: `internal/runtime/tools/readurl.go`
- Test: `internal/runtime/tools/readurl_test.go`

**Step 1: Install dependency**

Run: `go get github.com/JohannesKaufmann/html-to-markdown/v2`

**Step 2: Write the failing test**

Create `internal/runtime/tools/readurl_test.go`:

```go
package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReadURLName(t *testing.T) {
	r := NewReadURL()
	if r.Name() != "read_url" {
		t.Errorf("expected 'read_url', got %q", r.Name())
	}
}

func TestReadURLExecute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><body><h1>Hello World</h1><p>This is a test.</p></body></html>`))
	}))
	defer server.Close()

	r := NewReadURL()
	args, _ := json.Marshal(map[string]string{"url": server.URL})
	result, err := r.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "Hello World") {
		t.Errorf("expected 'Hello World' in result, got %q", result)
	}
	if !strings.Contains(result, "This is a test") {
		t.Errorf("expected 'This is a test' in result, got %q", result)
	}
}

func TestReadURLMissingURL(t *testing.T) {
	r := NewReadURL()
	args, _ := json.Marshal(map[string]string{})
	_, err := r.Execute(context.Background(), args)
	if err == nil {
		t.Fatal("expected error for missing URL")
	}
}

func TestReadURLTruncation(t *testing.T) {
	long := strings.Repeat("x", 60000)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html><body><p>" + long + "</p></body></html>"))
	}))
	defer server.Close()

	r := NewReadURL()
	args, _ := json.Marshal(map[string]string{"url": server.URL})
	result, err := r.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) > 51000 {
		t.Errorf("expected truncation, got length %d", len(result))
	}
}
```

**Step 3: Run test to verify it fails**

Run: `go test ./internal/runtime/tools -run TestReadURL -v`
Expected: FAIL — `NewReadURL` undefined

**Step 4: Implement readurl.go**

Create `internal/runtime/tools/readurl.go`:

```go
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
)

const maxReadURLChars = 50000

// ReadURL fetches a URL and converts its HTML content to markdown.
type ReadURL struct {
	client *http.Client
}

// NewReadURL creates a new ReadURL tool.
func NewReadURL() *ReadURL {
	return &ReadURL{
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (r *ReadURL) Name() string        { return "read_url" }
func (r *ReadURL) Description() string { return "Fetch a URL and return its content as markdown" }
func (r *ReadURL) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"url": {"type": "string", "description": "The URL to fetch"}
		},
		"required": ["url"]
	}`)
}

func (r *ReadURL) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("parse args: %w", err)
	}
	if params.URL == "" {
		return "", fmt.Errorf("url is required")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, params.URL, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "Gopherclaw/1.0")

	resp, err := r.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP error: status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}

	md, err := htmltomarkdown.ConvertString(string(body))
	if err != nil {
		return "", fmt.Errorf("convert to markdown: %w", err)
	}

	if len(md) > maxReadURLChars {
		md = md[:maxReadURLChars] + "\n\n[Content truncated]"
	}

	return md, nil
}
```

**Step 5: Run test to verify it passes**

Run: `go test ./internal/runtime/tools -run TestReadURL -v`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/runtime/tools/readurl.go internal/runtime/tools/readurl_test.go
git commit -m "feat: add read_url tool with HTML-to-markdown conversion"
```

---

### Task 7: Context Engine

**Files:**
- Create: `internal/context/engine.go`
- Test: `internal/context/engine_test.go`

**Step 1: Install dependency**

Run: `go get github.com/pkoukk/tiktoken-go`

**Step 2: Write the failing test**

Create `internal/context/engine_test.go`:

```go
package context

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/user/gopherclaw/internal/types"
)

func TestNewEngine(t *testing.T) {
	e, err := New("gpt-4", 128000, 4096)
	if err != nil {
		t.Fatal(err)
	}
	if e == nil {
		t.Fatal("expected non-nil engine")
	}
}

func TestBuildPromptBasic(t *testing.T) {
	e, err := New("gpt-4", 128000, 4096)
	if err != nil {
		t.Fatal(err)
	}

	session := &types.SessionIndex{
		SessionID: "test-session",
		Agent:     "default",
		Status:    "active",
	}

	userPayload, _ := json.Marshal(map[string]string{"text": "hello"})
	assistantPayload, _ := json.Marshal(map[string]string{"text": "hi there"})

	events := []*types.Event{
		{ID: "e1", SessionID: "test-session", Seq: 1, Type: "user_message", Source: "telegram", At: time.Now(), Payload: userPayload},
		{ID: "e2", SessionID: "test-session", Seq: 2, Type: "assistant_message", Source: "runtime", At: time.Now(), Payload: assistantPayload},
	}

	messages, err := e.BuildPrompt(context.Background(), session, events, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Should have: system prompt + 2 event messages
	if len(messages) < 3 {
		t.Fatalf("expected at least 3 messages, got %d", len(messages))
	}
	if messages[0].Role != "system" {
		t.Errorf("expected system message first, got %q", messages[0].Role)
	}
	if messages[1].Role != "user" {
		t.Errorf("expected user message, got %q", messages[1].Role)
	}
	if messages[1].Content != "hello" {
		t.Errorf("expected 'hello', got %q", messages[1].Content)
	}
	if messages[2].Role != "assistant" {
		t.Errorf("expected assistant message, got %q", messages[2].Role)
	}
}

func TestBuildPromptToolCallEvents(t *testing.T) {
	e, err := New("gpt-4", 128000, 4096)
	if err != nil {
		t.Fatal(err)
	}

	session := &types.SessionIndex{SessionID: "test-session", Agent: "default", Status: "active"}

	tcPayload, _ := json.Marshal(map[string]any{
		"tool": "bash", "call_id": "tc1",
		"arguments": map[string]string{"command": "echo hi"},
	})
	trPayload, _ := json.Marshal(map[string]any{
		"tool": "bash", "call_id": "tc1", "result": "hi\n",
	})

	events := []*types.Event{
		{ID: "e1", Seq: 1, Type: "user_message", Source: "telegram", Payload: json.RawMessage(`{"text":"run echo"}`)},
		{ID: "e2", Seq: 2, Type: "tool_call", Source: "runtime", Payload: tcPayload},
		{ID: "e3", Seq: 3, Type: "tool_result", Source: "runtime", Payload: trPayload},
		{ID: "e4", Seq: 4, Type: "assistant_message", Source: "runtime", Payload: json.RawMessage(`{"text":"done"}`)},
	}

	messages, err := e.BuildPrompt(context.Background(), session, events, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// system + user + assistant(tool_call) + tool_result + assistant
	if len(messages) < 5 {
		t.Fatalf("expected at least 5 messages, got %d", len(messages))
	}
}

func TestBuildPromptBudgetTruncation(t *testing.T) {
	// Tiny budget: only 500 tokens total, 100 reserve
	e, err := New("gpt-4", 500, 100)
	if err != nil {
		t.Fatal(err)
	}

	session := &types.SessionIndex{SessionID: "test-session", Agent: "default", Status: "active"}

	// Create many events that exceed the budget
	events := make([]*types.Event, 50)
	for i := range events {
		payload, _ := json.Marshal(map[string]string{"text": "This is a message that takes up tokens in the context window budget."})
		events[i] = &types.Event{
			ID: types.EventID("e" + string(rune('0'+i))), Seq: int64(i + 1),
			Type: "user_message", Source: "test", Payload: payload,
		}
	}

	messages, err := e.BuildPrompt(context.Background(), session, events, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Should have fewer messages than events due to budget
	if len(messages) >= 51 {
		t.Errorf("expected truncation, got %d messages for 50 events", len(messages))
	}
	// Must have at least system prompt
	if len(messages) < 1 {
		t.Fatal("expected at least system prompt")
	}
}
```

**Step 3: Run test to verify it fails**

Run: `go test ./internal/context -v`
Expected: FAIL — package does not exist

**Step 4: Implement engine.go**

Create `internal/context/engine.go`:

```go
package context

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/pkoukk/tiktoken-go"

	"github.com/user/gopherclaw/internal/types"
	"github.com/user/gopherclaw/pkg/llm"
)

// Engine assembles token-budgeted prompts for the LLM.
type Engine struct {
	tokenizer *tiktoken.Tiktoken
	maxTokens int
	reserve   int
}

// New creates a context engine with the specified token budget.
func New(model string, maxTokens, reserve int) (*Engine, error) {
	enc, err := tiktoken.EncodingForModel(model)
	if err != nil {
		// Fallback to cl100k_base for unknown models
		enc, err = tiktoken.GetEncoding("cl100k_base")
		if err != nil {
			return nil, fmt.Errorf("get tokenizer: %w", err)
		}
	}
	return &Engine{
		tokenizer: enc,
		maxTokens: maxTokens,
		reserve:   reserve,
	}, nil
}

// countTokens returns the token count for a string.
func (e *Engine) countTokens(text string) int {
	return len(e.tokenizer.Encode(text, nil, nil))
}

// BuildPrompt assembles a token-budgeted prompt from session history.
// toolNames is an optional list of available tool names for the system prompt.
func (e *Engine) BuildPrompt(
	ctx context.Context,
	session *types.SessionIndex,
	events []*types.Event,
	artifacts types.ArtifactStore,
	toolNames []string,
) ([]llm.Message, error) {
	inputBudget := e.maxTokens - e.reserve

	// 1. System prompt
	sysPrompt := buildSystemPrompt(session, toolNames)
	sysTokens := e.countTokens(sysPrompt)
	remaining := inputBudget - sysTokens

	// 70% for events, 10% safety margin (20% artifact budget unused for now)
	eventBudget := int(float64(remaining) * 0.7)

	// 2. Convert events to messages, respecting budget
	var eventMessages []llm.Message
	usedTokens := 0

	for _, event := range events {
		msg, err := eventToMessage(event)
		if err != nil {
			continue
		}

		msgTokens := e.countTokens(msg.Content)
		for _, tc := range msg.Tools {
			msgTokens += e.countTokens(tc.Function.Name)
			msgTokens += e.countTokens(string(tc.Function.Arguments))
		}

		if usedTokens+msgTokens > eventBudget {
			break
		}

		eventMessages = append(eventMessages, msg)
		usedTokens += msgTokens
	}

	// 3. Assemble: system + events (already in chronological order)
	messages := make([]llm.Message, 0, 1+len(eventMessages))
	messages = append(messages, llm.Message{Role: "system", Content: sysPrompt})
	messages = append(messages, eventMessages...)

	return messages, nil
}

func buildSystemPrompt(session *types.SessionIndex, toolNames []string) string {
	prompt := fmt.Sprintf(
		"You are a helpful assistant. Current time: %s. Session: %s.",
		time.Now().Format(time.RFC3339),
		string(session.SessionID),
	)
	if len(toolNames) > 0 {
		prompt += fmt.Sprintf(" You have access to the following tools: %v.", toolNames)
	}
	return prompt
}

type eventPayload struct {
	Text      string          `json:"text"`
	Tool      string          `json:"tool"`
	CallID    string          `json:"call_id"`
	Arguments json.RawMessage `json:"arguments"`
	Result    string          `json:"result"`
}

func eventToMessage(event *types.Event) (llm.Message, error) {
	var payload eventPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return llm.Message{}, err
	}

	switch event.Type {
	case "user_message":
		return llm.Message{Role: "user", Content: payload.Text}, nil

	case "assistant_message":
		return llm.Message{Role: "assistant", Content: payload.Text}, nil

	case "tool_call":
		return llm.Message{
			Role: "assistant",
			Tools: []llm.ToolCall{{
				ID:   payload.CallID,
				Type: "function",
				Function: llm.FunctionCall{
					Name:      payload.Tool,
					Arguments: payload.Arguments,
				},
			}},
		}, nil

	case "tool_result":
		return llm.Message{
			Role:    "tool",
			Content: payload.Result,
			Tools: []llm.ToolCall{{
				ID: payload.CallID,
			}},
		}, nil

	default:
		return llm.Message{}, fmt.Errorf("unknown event type: %s", event.Type)
	}
}
```

**Step 5: Run test to verify it passes**

Run: `go test ./internal/context -v`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/context/engine.go internal/context/engine_test.go
git commit -m "feat: add token-budgeted context engine with tiktoken"
```

---

### Task 8: Runtime Turn Loop

**Files:**
- Create: `internal/runtime/runtime.go`
- Test: `internal/runtime/runtime_test.go`

**Step 1: Write the failing test**

Create `internal/runtime/runtime_test.go`:

```go
package runtime

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	ctxengine "github.com/user/gopherclaw/internal/context"
	"github.com/user/gopherclaw/internal/gateway"
	"github.com/user/gopherclaw/internal/state"
	"github.com/user/gopherclaw/internal/types"
	"github.com/user/gopherclaw/pkg/llm"
)

// mockProvider returns pre-configured responses.
type mockProvider struct {
	mu        sync.Mutex
	responses []*llm.Response
	callCount int
}

func (m *mockProvider) Complete(_ context.Context, messages []llm.Message, tools []llm.Tool) (*llm.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	idx := m.callCount
	m.callCount++
	if idx < len(m.responses) {
		return m.responses[idx], nil
	}
	return &llm.Response{Content: "fallback"}, nil
}

func (m *mockProvider) Stream(_ context.Context, messages []llm.Message, tools []llm.Tool) (<-chan llm.Delta, error) {
	return nil, nil
}

func TestProcessRunSimpleResponse(t *testing.T) {
	dir := t.TempDir()
	sessions := state.NewSessionStore(dir)
	events := state.NewEventStore(dir)
	artifacts := state.NewArtifactStore(dir)

	ctx := context.Background()
	sid, err := sessions.ResolveOrCreate(ctx, types.NewSessionKey("test", "user1"), "default")
	if err != nil {
		t.Fatal(err)
	}

	provider := &mockProvider{
		responses: []*llm.Response{
			{Content: "Hello! How can I help?"},
		},
	}

	engine, err := ctxengine.New("gpt-4", 128000, 4096)
	if err != nil {
		t.Fatal(err)
	}

	registry := NewRegistry()
	rt := New(provider, engine, sessions, events, artifacts, registry, 10)

	var callbackResult string
	done := make(chan struct{})

	run := &gateway.Run{
		ID:        types.NewRunID(),
		SessionID: sid,
		Event: &types.InboundEvent{
			Source:     "test",
			SessionKey: types.NewSessionKey("test", "user1"),
			UserID:     "user1",
			Text:       "hi",
		},
		Status:    gateway.RunStatusRunning,
		CreatedAt: time.Now(),
		OnComplete: func(resp string) {
			callbackResult = resp
			close(done)
		},
	}

	err = rt.ProcessRun(run)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for callback")
	}

	if callbackResult != "Hello! How can I help?" {
		t.Errorf("expected callback result, got %q", callbackResult)
	}

	// Verify events were recorded: user_message + assistant_message
	count, err := events.Count(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("expected 2 events, got %d", count)
	}
}

func TestProcessRunWithToolCall(t *testing.T) {
	dir := t.TempDir()
	sessions := state.NewSessionStore(dir)
	events := state.NewEventStore(dir)
	artifacts := state.NewArtifactStore(dir)

	ctx := context.Background()
	sid, err := sessions.ResolveOrCreate(ctx, types.NewSessionKey("test", "user1"), "default")
	if err != nil {
		t.Fatal(err)
	}

	provider := &mockProvider{
		responses: []*llm.Response{
			// First call: LLM requests a tool call
			{
				ToolCalls: []llm.ToolCall{{
					ID:   "tc1",
					Type: "function",
					Function: llm.FunctionCall{
						Name:      "echo",
						Arguments: json.RawMessage(`{"text":"world"}`),
					},
				}},
			},
			// Second call: LLM gives final response
			{Content: "The echo returned: world"},
		},
	}

	engine, err := ctxengine.New("gpt-4", 128000, 4096)
	if err != nil {
		t.Fatal(err)
	}

	registry := NewRegistry()
	registry.Register(&echoTool{})

	rt := New(provider, engine, sessions, events, artifacts, registry, 10)

	var callbackResult string
	done := make(chan struct{})

	run := &gateway.Run{
		ID:        types.NewRunID(),
		SessionID: sid,
		Event: &types.InboundEvent{
			Source:     "test",
			SessionKey: types.NewSessionKey("test", "user1"),
			UserID:     "user1",
			Text:       "echo world",
		},
		Status:    gateway.RunStatusRunning,
		CreatedAt: time.Now(),
		OnComplete: func(resp string) {
			callbackResult = resp
			close(done)
		},
	}

	err = rt.ProcessRun(run)
	if err != nil {
		t.Fatal(err)
	}

	<-done

	if callbackResult != "The echo returned: world" {
		t.Errorf("expected 'The echo returned: world', got %q", callbackResult)
	}

	// Events: user_message + tool_call + tool_result + assistant_message = 4
	count, err := events.Count(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	if count != 4 {
		t.Errorf("expected 4 events, got %d", count)
	}
}

func TestProcessRunMaxRounds(t *testing.T) {
	dir := t.TempDir()
	sessions := state.NewSessionStore(dir)
	events := state.NewEventStore(dir)
	artifacts := state.NewArtifactStore(dir)

	ctx := context.Background()
	sid, err := sessions.ResolveOrCreate(ctx, types.NewSessionKey("test", "user1"), "default")
	if err != nil {
		t.Fatal(err)
	}

	// Provider always returns tool calls (infinite loop)
	infProvider := &mockProvider{
		responses: make([]*llm.Response, 20),
	}
	for i := range infProvider.responses {
		infProvider.responses[i] = &llm.Response{
			ToolCalls: []llm.ToolCall{{
				ID: "tc1", Type: "function",
				Function: llm.FunctionCall{Name: "echo", Arguments: json.RawMessage(`{"text":"loop"}`)},
			}},
		}
	}

	engine, _ := ctxengine.New("gpt-4", 128000, 4096)
	registry := NewRegistry()
	registry.Register(&echoTool{})

	rt := New(infProvider, engine, sessions, events, artifacts, registry, 3) // max 3 rounds

	run := &gateway.Run{
		ID:        types.NewRunID(),
		SessionID: sid,
		Event:     &types.InboundEvent{Source: "test", SessionKey: "test:u1", UserID: "u1", Text: "loop"},
		Status:    gateway.RunStatusRunning,
		CreatedAt: time.Now(),
	}

	err = rt.ProcessRun(run)
	if err == nil {
		t.Fatal("expected error for max rounds exceeded")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/runtime -run TestProcessRun -v`
Expected: FAIL — `New`, `ProcessRun` undefined

**Step 3: Implement runtime.go**

Create `internal/runtime/runtime.go`:

```go
package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	ctxengine "github.com/user/gopherclaw/internal/context"
	"github.com/user/gopherclaw/internal/gateway"
	"github.com/user/gopherclaw/internal/types"
	"github.com/user/gopherclaw/pkg/llm"
)

// Runtime implements the agentic turn loop.
type Runtime struct {
	provider  llm.Provider
	engine    *ctxengine.Engine
	sessions  types.SessionStore
	events    types.EventStore
	artifacts types.ArtifactStore
	registry  *Registry
	maxRounds int
}

// New creates a Runtime with the given dependencies.
func New(
	provider llm.Provider,
	engine *ctxengine.Engine,
	sessions types.SessionStore,
	events types.EventStore,
	artifacts types.ArtifactStore,
	registry *Registry,
	maxRounds int,
) *Runtime {
	return &Runtime{
		provider:  provider,
		engine:    engine,
		sessions:  sessions,
		events:    events,
		artifacts: artifacts,
		registry:  registry,
		maxRounds: maxRounds,
	}
}

const artifactThreshold = 2000

// ProcessRun executes the agentic turn loop for a single run.
// This is the function passed to Queue.SetProcessor.
func (rt *Runtime) ProcessRun(run *gateway.Run) error {
	ctx := context.Background()

	// 1. Record user_message event
	userPayload, _ := json.Marshal(map[string]string{"text": run.Event.Text})
	if err := rt.events.Append(ctx, &types.Event{
		ID:        types.NewEventID(),
		SessionID: run.SessionID,
		RunID:     run.ID,
		Type:      "user_message",
		Source:    run.Event.Source,
		At:        time.Now(),
		Payload:   userPayload,
	}); err != nil {
		return fmt.Errorf("record user message: %w", err)
	}

	// Collect tool names for system prompt
	var toolNames []string
	for _, t := range rt.registry.All() {
		toolNames = append(toolNames, t.Name())
	}

	for round := 0; round < rt.maxRounds; round++ {
		// 2. Load session
		session, err := rt.sessions.Get(ctx, run.SessionID)
		if err != nil {
			return fmt.Errorf("load session: %w", err)
		}

		// 3. Load recent events
		events, err := rt.events.Tail(ctx, run.SessionID, 100)
		if err != nil {
			return fmt.Errorf("load events: %w", err)
		}

		// 4. Build prompt
		messages, err := rt.engine.BuildPrompt(ctx, session, events, rt.artifacts, toolNames)
		if err != nil {
			return fmt.Errorf("build prompt: %w", err)
		}

		// 5. Call LLM
		resp, err := rt.provider.Complete(ctx, messages, rt.registry.AsLLMTools())
		if err != nil {
			return fmt.Errorf("LLM call: %w", err)
		}

		// 6. If tool calls, execute them
		if len(resp.ToolCalls) > 0 {
			for _, tc := range resp.ToolCalls {
				// Record tool_call event
				tcPayload, _ := json.Marshal(map[string]any{
					"tool":      tc.Function.Name,
					"call_id":   tc.ID,
					"arguments": tc.Function.Arguments,
				})
				if err := rt.events.Append(ctx, &types.Event{
					ID:        types.NewEventID(),
					SessionID: run.SessionID,
					RunID:     run.ID,
					Type:      "tool_call",
					Source:    "runtime",
					At:        time.Now(),
					Payload:   tcPayload,
				}); err != nil {
					return fmt.Errorf("record tool call: %w", err)
				}

				// Execute tool
				tool, ok := rt.registry.Get(tc.Function.Name)
				var result string
				if !ok {
					result = fmt.Sprintf("error: unknown tool %q", tc.Function.Name)
				} else {
					var execErr error
					result, execErr = tool.Execute(ctx, tc.Function.Arguments)
					if execErr != nil {
						result = fmt.Sprintf("error: %v", execErr)
					}
				}

				// Store as artifact if large
				trPayload := map[string]any{
					"tool":    tc.Function.Name,
					"call_id": tc.ID,
					"result":  result,
				}
				if len(result) > artifactThreshold {
					artID, err := rt.artifacts.Put(ctx, run.SessionID, run.ID, tc.Function.Name, result)
					if err == nil {
						trPayload["artifact_id"] = string(artID)
						// Truncate result in event
						trPayload["result"] = result[:artifactThreshold] + "\n[truncated, see artifact " + string(artID) + "]"
					}
				}

				trPayloadJSON, _ := json.Marshal(trPayload)
				if err := rt.events.Append(ctx, &types.Event{
					ID:        types.NewEventID(),
					SessionID: run.SessionID,
					RunID:     run.ID,
					Type:      "tool_result",
					Source:    "runtime",
					At:        time.Now(),
					Payload:   trPayloadJSON,
				}); err != nil {
					return fmt.Errorf("record tool result: %w", err)
				}
			}
			continue // Loop back for next LLM call
		}

		// 7. Text response — done
		if resp.Content != "" {
			aPayload, _ := json.Marshal(map[string]string{"text": resp.Content})
			if err := rt.events.Append(ctx, &types.Event{
				ID:        types.NewEventID(),
				SessionID: run.SessionID,
				RunID:     run.ID,
				Type:      "assistant_message",
				Source:    "runtime",
				At:        time.Now(),
				Payload:   aPayload,
			}); err != nil {
				return fmt.Errorf("record assistant message: %w", err)
			}
			if run.OnComplete != nil {
				run.OnComplete(resp.Content)
			}
			return nil
		}

		// Empty response (no content, no tool calls) — treat as done
		if run.OnComplete != nil {
			run.OnComplete("")
		}
		return nil
	}

	return fmt.Errorf("max tool rounds (%d) exceeded", rt.maxRounds)
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/runtime -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/runtime/runtime.go internal/runtime/runtime_test.go
git commit -m "feat: add agentic turn loop runtime with tool execution"
```

---

### Task 9: Telegram Adapter

**Files:**
- Create: `internal/telegram/adapter.go`
- Test: `internal/telegram/adapter_test.go`

**Step 1: Install dependency**

Run: `go get github.com/go-telegram-bot-api/telegram-bot-api/v5`

**Step 2: Write the failing test**

Create `internal/telegram/adapter_test.go`:

```go
package telegram

import (
	"strings"
	"testing"
)

func TestSplitMessage(t *testing.T) {
	short := "Hello world"
	parts := splitMessage(short)
	if len(parts) != 1 {
		t.Fatalf("expected 1 part, got %d", len(parts))
	}
	if parts[0] != short {
		t.Errorf("expected %q, got %q", short, parts[0])
	}
}

func TestSplitMessageLong(t *testing.T) {
	long := strings.Repeat("a", 5000)
	parts := splitMessage(long)
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(parts))
	}
	if len(parts[0]) != maxTelegramMessage {
		t.Errorf("expected first part length %d, got %d", maxTelegramMessage, len(parts[0]))
	}
}

func TestBuildSessionKey(t *testing.T) {
	key := buildSessionKey(12345, 67890)
	if string(key) != "telegram:12345:67890" {
		t.Errorf("expected 'telegram:12345:67890', got %q", key)
	}
}
```

**Step 3: Run test to verify it fails**

Run: `go test ./internal/telegram -v`
Expected: FAIL — package does not exist

**Step 4: Implement adapter.go**

Create `internal/telegram/adapter.go`:

```go
package telegram

import (
	"context"
	"fmt"
	"log"
	"strconv"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/user/gopherclaw/internal/gateway"
	"github.com/user/gopherclaw/internal/types"
)

const maxTelegramMessage = 4096

// Adapter bridges Telegram to the gateway.
type Adapter struct {
	bot      *tgbotapi.BotAPI
	gateway  *gateway.Gateway
	events   types.EventStore
	sessions types.SessionStore
}

// New creates a Telegram adapter.
func New(token string, gw *gateway.Gateway, events types.EventStore, sessions types.SessionStore) (*Adapter, error) {
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("create bot: %w", err)
	}
	return &Adapter{
		bot:      bot,
		gateway:  gw,
		events:   events,
		sessions: sessions,
	}, nil
}

// Start begins long-polling for Telegram updates.
func (a *Adapter) Start(ctx context.Context) {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 30

	updates := a.bot.GetUpdatesChan(u)

	for {
		select {
		case update := <-updates:
			if update.Message == nil || update.Message.Text == "" {
				continue
			}
			a.handleMessage(ctx, update.Message)
		case <-ctx.Done():
			a.bot.StopReceivingUpdates()
			return
		}
	}
}

func (a *Adapter) handleMessage(ctx context.Context, msg *tgbotapi.Message) {
	// Handle commands
	if msg.IsCommand() {
		a.handleCommand(ctx, msg)
		return
	}

	chatID := msg.Chat.ID
	event := &types.InboundEvent{
		Source:     "telegram",
		SessionKey: buildSessionKey(msg.From.ID, msg.Chat.ID),
		UserID:     strconv.FormatInt(msg.From.ID, 10),
		Text:       msg.Text,
	}

	err := a.gateway.HandleInbound(ctx, event, gateway.WithOnComplete(func(response string) {
		a.sendResponse(chatID, response)
	}))
	if err != nil {
		log.Printf("handle inbound error: %v", err)
		a.sendResponse(chatID, "Sorry, I encountered an error processing your message.")
	}
}

func (a *Adapter) handleCommand(ctx context.Context, msg *tgbotapi.Message) {
	chatID := msg.Chat.ID

	switch msg.Command() {
	case "start":
		a.sendResponse(chatID, "Hello! I'm Gopherclaw, your AI assistant. Send me a message to get started.")

	case "new":
		// Archive current session by creating a new session key with timestamp
		a.sendResponse(chatID, "Starting a new session. Previous conversation has been archived.")

	case "status":
		key := buildSessionKey(msg.From.ID, msg.Chat.ID)
		sid, err := a.sessions.ResolveOrCreate(ctx, key, "default")
		if err != nil {
			a.sendResponse(chatID, "Error fetching status.")
			return
		}
		count, err := a.events.Count(ctx, sid)
		if err != nil {
			a.sendResponse(chatID, "Error fetching status.")
			return
		}
		a.sendResponse(chatID, fmt.Sprintf("Session: %s\nMessages: %d", sid, count))

	default:
		a.sendResponse(chatID, "Unknown command. Available: /start, /new, /status")
	}
}

func (a *Adapter) sendResponse(chatID int64, text string) {
	parts := splitMessage(text)
	for _, part := range parts {
		msg := tgbotapi.NewMessage(chatID, part)
		msg.ParseMode = "Markdown"
		if _, err := a.bot.Send(msg); err != nil {
			// Retry without markdown if it fails
			msg.ParseMode = ""
			if _, err := a.bot.Send(msg); err != nil {
				log.Printf("send message error: %v", err)
			}
		}
	}
}

func splitMessage(text string) []string {
	if len(text) <= maxTelegramMessage {
		return []string{text}
	}
	var parts []string
	for len(text) > 0 {
		end := maxTelegramMessage
		if end > len(text) {
			end = len(text)
		}
		parts = append(parts, text[:end])
		text = text[end:]
	}
	return parts
}

func buildSessionKey(userID, chatID int64) types.SessionKey {
	return types.NewSessionKey("telegram",
		strconv.FormatInt(userID, 10),
		strconv.FormatInt(chatID, 10),
	)
}
```

**Step 5: Run test to verify it passes**

Run: `go test ./internal/telegram -v`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/telegram/adapter.go internal/telegram/adapter_test.go
git commit -m "feat: add Telegram adapter with long polling and commands"
```

---

### Task 10: Wire Everything in main.go

**Files:**
- Modify: `cmd/gopherclaw/main.go`

**Step 1: Update main.go**

Replace the full contents of `cmd/gopherclaw/main.go`:

```go
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/user/gopherclaw/internal/config"
	ctxengine "github.com/user/gopherclaw/internal/context"
	"github.com/user/gopherclaw/internal/gateway"
	"github.com/user/gopherclaw/internal/runtime"
	"github.com/user/gopherclaw/internal/runtime/tools"
	"github.com/user/gopherclaw/internal/state"
	"github.com/user/gopherclaw/internal/telegram"
	"github.com/user/gopherclaw/pkg/llm/openai"
)

func main() {
	configPath := flag.String("config", filepath.Join(os.Getenv("HOME"), ".gopherclaw", "config.json"), "config file path")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		log.Fatalf("Failed to create data dir: %v", err)
	}

	// Stores
	sessions := state.NewSessionStore(cfg.DataDir)
	events := state.NewEventStore(cfg.DataDir)
	artifacts := state.NewArtifactStore(cfg.DataDir)

	// LLM provider
	provider := openai.New(&openai.Config{
		BaseURL:     cfg.LLM.BaseURL,
		APIKey:      cfg.LLM.APIKey,
		Model:       cfg.LLM.Model,
		MaxTokens:   cfg.LLM.MaxTokens,
		Temperature: cfg.LLM.Temperature,
	})

	// Context engine
	engine, err := ctxengine.New(cfg.LLM.Model, cfg.LLM.MaxContextTokens, cfg.LLM.OutputReserve)
	if err != nil {
		log.Fatalf("Failed to create context engine: %v", err)
	}

	// Tool registry
	registry := runtime.NewRegistry()
	registry.Register(tools.NewBash())
	if cfg.Brave.APIKey != "" {
		registry.Register(tools.NewBraveSearch(cfg.Brave.APIKey))
	}
	registry.Register(tools.NewReadURL())

	// Runtime
	rt := runtime.New(provider, engine, sessions, events, artifacts, registry, cfg.MaxToolRounds)

	// Gateway
	gw := gateway.New(sessions, events, artifacts, int64(cfg.MaxConcurrent))
	gw.Queue.SetProcessor(rt.ProcessRun)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	gw.Start(ctx)
	defer gw.Stop()

	fmt.Printf("Gopherclaw started\n")
	fmt.Printf("Data directory: %s\n", cfg.DataDir)
	fmt.Printf("Max concurrent runs: %d\n", cfg.MaxConcurrent)
	fmt.Printf("Max tool rounds: %d\n", cfg.MaxToolRounds)
	fmt.Printf("LLM provider: %s (%s)\n", cfg.LLM.Provider, cfg.LLM.Model)
	fmt.Printf("Tools: bash, read_url")
	if cfg.Brave.APIKey != "" {
		fmt.Printf(", brave_search")
	}
	fmt.Println()

	// Telegram adapter
	if cfg.Telegram.Token != "" {
		adapter, err := telegram.New(cfg.Telegram.Token, gw, events, sessions)
		if err != nil {
			log.Fatalf("Failed to create Telegram adapter: %v", err)
		}
		go adapter.Start(ctx)
		fmt.Println("Telegram adapter started")
	} else {
		fmt.Println("Telegram adapter disabled (no token)")
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nShutting down...")
}
```

Note: The `openai.New` function currently takes `*llm.Config`. We need to check if it needs to be adapted, or if we pass `*llm.Config` directly. Looking at the existing code, `openai.New` takes `*llm.Config`, so we should use that:

```go
provider := openai.New(&llm.Config{
    BaseURL:     cfg.LLM.BaseURL,
    APIKey:      cfg.LLM.APIKey,
    Model:       cfg.LLM.Model,
    MaxTokens:   cfg.LLM.MaxTokens,
    Temperature: cfg.LLM.Temperature,
})
```

And import `"github.com/user/gopherclaw/pkg/llm"` instead of `openai`.

**Step 2: Verify build**

Run: `go build ./cmd/gopherclaw`
Expected: Clean build

**Step 3: Run all tests**

Run: `go test ./... -v`
Expected: All pass

**Step 4: Commit**

```bash
git add cmd/gopherclaw/main.go
git commit -m "feat: wire runtime, context engine, tools, and telegram in main"
```

---

### Task 11: Update Integration Test

**Files:**
- Modify: `test/integration_test.go`

**Step 1: Add runtime integration test**

Add a new test to `test/integration_test.go` that exercises the full runtime loop with a mock provider:

```go
func TestEndToEndWithRuntime(t *testing.T) {
	dir := t.TempDir()

	sessions := state.NewSessionStore(dir)
	events := state.NewEventStore(dir)
	artifacts := state.NewArtifactStore(dir)

	// Mock provider that returns a simple response
	provider := &mockProvider{
		response: &llm.Response{Content: "Hello from the LLM!"},
	}

	engine, err := ctxengine.New("gpt-4", 128000, 4096)
	if err != nil {
		t.Fatal(err)
	}

	registry := runtime.NewRegistry()
	rt := runtime.New(provider, engine, sessions, events, artifacts, registry, 10)

	gw := gateway.New(sessions, events, artifacts)
	gw.Queue.SetProcessor(rt.ProcessRun)

	ctx := context.Background()
	gw.Start(ctx)
	defer gw.Stop()

	// Send message and capture response via callback
	var response string
	done := make(chan struct{})

	inbound := &types.InboundEvent{
		Source:     "test",
		SessionKey: types.NewSessionKey("test", "user1"),
		UserID:     "user1",
		Text:       "hello",
	}

	err = gw.HandleInbound(ctx, inbound, gateway.WithOnComplete(func(resp string) {
		response = resp
		close(done)
	}))
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for response")
	}

	if response != "Hello from the LLM!" {
		t.Errorf("expected 'Hello from the LLM!', got %q", response)
	}

	// Verify events: user_message + assistant_message
	sessionList, err := sessions.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	eventCount, err := events.Count(ctx, sessionList[0].SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if eventCount != 2 {
		t.Errorf("expected 2 events, got %d", eventCount)
	}
}

type mockProvider struct {
	response *llm.Response
}

func (m *mockProvider) Complete(_ context.Context, _ []llm.Message, _ []llm.Tool) (*llm.Response, error) {
	return m.response, nil
}

func (m *mockProvider) Stream(_ context.Context, _ []llm.Message, _ []llm.Tool) (<-chan llm.Delta, error) {
	return nil, nil
}
```

Add required imports to the test file:

```go
import (
	"context"
	"fmt"
	"testing"
	"time"

	ctxengine "github.com/user/gopherclaw/internal/context"
	"github.com/user/gopherclaw/internal/gateway"
	"github.com/user/gopherclaw/internal/runtime"
	"github.com/user/gopherclaw/internal/state"
	"github.com/user/gopherclaw/internal/types"
	"github.com/user/gopherclaw/pkg/llm"
)
```

**Step 2: Run integration tests**

Run: `go test -tags=integration ./test -v`
Expected: Both `TestEndToEnd` and `TestEndToEndWithRuntime` pass

**Step 3: Run all tests**

Run: `go test ./... -v && go build ./...`
Expected: All pass, clean build

**Step 4: Commit**

```bash
git add test/integration_test.go
git commit -m "feat: add runtime integration test with mock LLM provider"
```

---

### Task 12: Final Verification + go mod tidy

**Step 1: Install all dependencies**

Run: `go mod tidy`

**Step 2: Run full test suite with race detector**

Run: `go test -race ./...`
Expected: All pass, no races

**Step 3: Run integration tests**

Run: `go test -tags=integration -race ./test -v`
Expected: PASS

**Step 4: Build binary**

Run: `go build -o bin/gopherclaw cmd/gopherclaw/main.go`
Expected: Clean build

**Step 5: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: go mod tidy for phases 3-5 dependencies"
```
