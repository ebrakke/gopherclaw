# CLI Commands Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add cobra-based CLI with config, session, lifecycle, and setup commands so both humans and the bot can manage gopherclaw.

**Architecture:** Cobra root command in `cmd/gopherclaw/main.go` with subcommands in separate files. Config get/set uses a flatten/unflatten layer over the existing JSON config. PID file enables lifecycle commands. Setup wizard uses stdin prompts.

**Tech Stack:** cobra v1.8+, existing config/state packages, syscall for signals

---

### Task 1: Add cobra dependency

**Files:**
- Modify: `go.mod`

**Step 1: Install cobra**

Run: `go get github.com/spf13/cobra@latest`

**Step 2: Tidy**

Run: `go mod tidy`

**Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: add cobra CLI framework dependency"
```

---

### Task 2: Config flatten/unflatten helpers

**Files:**
- Create: `internal/config/flatten.go`
- Create: `internal/config/flatten_test.go`

**Step 1: Write the failing tests**

```go
package config

import (
	"testing"
)

func TestFlatten(t *testing.T) {
	m := map[string]any{
		"data_dir":  "/tmp/gc",
		"log_level": "info",
		"llm": map[string]any{
			"provider": "openai",
			"model":    "gpt-4o",
		},
		"brave": map[string]any{
			"api_key": "abc123",
		},
	}

	flat := Flatten(m)

	tests := map[string]any{
		"data_dir":     "/tmp/gc",
		"log_level":    "info",
		"llm.provider": "openai",
		"llm.model":    "gpt-4o",
		"brave.api_key": "abc123",
	}
	for k, want := range tests {
		got, ok := flat[k]
		if !ok {
			t.Errorf("missing key %q", k)
			continue
		}
		if got != want {
			t.Errorf("key %q: got %v, want %v", k, got, want)
		}
	}
}

func TestUnflatten(t *testing.T) {
	flat := map[string]any{
		"data_dir":     "/tmp/gc",
		"llm.provider": "openai",
		"llm.model":    "gpt-4o",
		"brave.api_key": "abc123",
	}

	nested := Unflatten(flat)

	llm, ok := nested["llm"].(map[string]any)
	if !ok {
		t.Fatal("llm is not a map")
	}
	if llm["provider"] != "openai" {
		t.Errorf("llm.provider: got %v, want openai", llm["provider"])
	}
	if llm["model"] != "gpt-4o" {
		t.Errorf("llm.model: got %v, want gpt-4o", llm["model"])
	}

	brave, ok := nested["brave"].(map[string]any)
	if !ok {
		t.Fatal("brave is not a map")
	}
	if brave["api_key"] != "abc123" {
		t.Errorf("brave.api_key: got %v, want abc123", brave["api_key"])
	}
}

func TestMaskSecrets(t *testing.T) {
	flat := map[string]any{
		"llm.api_key":   "sk-1234567890abcdef",
		"llm.model":     "gpt-4o",
		"brave.api_key":  "brave-key-value",
		"telegram.token": "123456:ABCdef",
	}

	masked := MaskSecrets(flat)

	if masked["llm.api_key"] != "***cdef" {
		t.Errorf("llm.api_key: got %v, want ***cdef", masked["llm.api_key"])
	}
	if masked["llm.model"] != "gpt-4o" {
		t.Errorf("llm.model should not be masked: got %v", masked["llm.model"])
	}
	if masked["brave.api_key"] != "***alue" {
		t.Errorf("brave.api_key: got %v, want ***alue", masked["brave.api_key"])
	}
	if masked["telegram.token"] != "***Cdef" {
		t.Errorf("telegram.token: got %v, want ***Cdef", masked["telegram.token"])
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/config/ -run TestFlatten -v`
Expected: FAIL (Flatten not defined)

**Step 3: Write implementation**

```go
package config

import (
	"fmt"
	"strings"
)

// secretKeys are config keys whose values should be masked in output.
var secretKeys = map[string]bool{
	"llm.api_key":    true,
	"brave.api_key":  true,
	"telegram.token": true,
}

// Flatten converts a nested map to dot-separated keys.
func Flatten(m map[string]any) map[string]any {
	out := make(map[string]any)
	flatten("", m, out)
	return out
}

func flatten(prefix string, m map[string]any, out map[string]any) {
	for k, v := range m {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		if sub, ok := v.(map[string]any); ok {
			flatten(key, sub, out)
		} else {
			out[key] = v
		}
	}
}

// Unflatten converts dot-separated keys back to a nested map.
func Unflatten(flat map[string]any) map[string]any {
	out := make(map[string]any)
	for k, v := range flat {
		parts := strings.Split(k, ".")
		m := out
		for i, p := range parts {
			if i == len(parts)-1 {
				m[p] = v
			} else {
				if _, ok := m[p]; !ok {
					m[p] = make(map[string]any)
				}
				m = m[p].(map[string]any)
			}
		}
	}
	return out
}

// MaskSecrets returns a copy of the flat map with secret values masked.
func MaskSecrets(flat map[string]any) map[string]any {
	out := make(map[string]any, len(flat))
	for k, v := range flat {
		if secretKeys[k] {
			s := fmt.Sprintf("%v", v)
			if len(s) > 4 {
				out[k] = "***" + s[len(s)-4:]
			} else if len(s) > 0 {
				out[k] = "***"
			} else {
				out[k] = ""
			}
		} else {
			out[k] = v
		}
	}
	return out
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/config/ -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/config/flatten.go internal/config/flatten_test.go
git commit -m "feat: add config flatten/unflatten helpers with secret masking"
```

---

### Task 3: Add Save() and ToMap() to config package

**Files:**
- Modify: `internal/config/config.go`
- Create: `internal/config/config_test.go`

**Step 1: Write the failing tests**

```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAndReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	cfg.LLM.Model = "gpt-4o"
	cfg.Brave.APIKey = "test-brave-key"

	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}

	cfg2, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if cfg2.LLM.Model != "gpt-4o" {
		t.Errorf("model: got %s, want gpt-4o", cfg2.LLM.Model)
	}
	if cfg2.Brave.APIKey != "test-brave-key" {
		t.Errorf("brave key: got %s, want test-brave-key", cfg2.Brave.APIKey)
	}
}

func TestToMap(t *testing.T) {
	cfg := &Config{}
	cfg.DataDir = "/tmp/test"
	cfg.LLM.Model = "gpt-4o"

	m, err := ToMap(cfg)
	if err != nil {
		t.Fatal(err)
	}

	if m["data_dir"] != "/tmp/test" {
		t.Errorf("data_dir: got %v", m["data_dir"])
	}

	llm, ok := m["llm"].(map[string]any)
	if !ok {
		t.Fatal("llm is not a map")
	}
	if llm["model"] != "gpt-4o" {
		t.Errorf("llm.model: got %v", llm["model"])
	}
}

func TestSetValue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	// Create initial config
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}

	// Set a nested value
	if err := SetValue(path, "llm.model", "gpt-4o"); err != nil {
		t.Fatal(err)
	}

	cfg2, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg2.LLM.Model != "gpt-4o" {
		t.Errorf("model: got %s, want gpt-4o", cfg2.LLM.Model)
	}
}

func TestGetValue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg.LLM.Model = "gpt-4o"
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}

	val, err := GetValue(path, "llm.model")
	if err != nil {
		t.Fatal(err)
	}
	if val != "gpt-4o" {
		t.Errorf("got %v, want gpt-4o", val)
	}

	_, err = GetValue(path, "nonexistent.key")
	if err == nil {
		t.Error("expected error for nonexistent key")
	}
}

func TestSetValueNumeric(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg, _ := Load(path)
	Save(path, cfg)

	// Set numeric value
	if err := SetValue(path, "max_tool_rounds", "20"); err != nil {
		t.Fatal(err)
	}

	cfg2, _ := Load(path)
	if cfg2.MaxToolRounds != 20 {
		t.Errorf("max_tool_rounds: got %d, want 20", cfg2.MaxToolRounds)
	}
}

func TestListValues(t *testing.T) {
	cfg := &Config{}
	cfg.DataDir = "/tmp/test"
	cfg.LLM.Model = "gpt-4o"
	cfg.LLM.APIKey = "sk-secret1234"

	flat, err := ListValues(cfg, true)
	if err != nil {
		t.Fatal(err)
	}

	if flat["data_dir"] != "/tmp/test" {
		t.Errorf("data_dir: got %v", flat["data_dir"])
	}
	// API key should be masked
	if flat["llm.api_key"] != "***1234" {
		t.Errorf("llm.api_key should be masked: got %v", flat["llm.api_key"])
	}

	// Without masking
	flat2, err := ListValues(cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	if flat2["llm.api_key"] != "sk-secret1234" {
		t.Errorf("llm.api_key should not be masked: got %v", flat2["llm.api_key"])
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/config/ -run TestSave -v`
Expected: FAIL

**Step 3: Write implementation — add to config.go**

Add `Save`, `ToMap`, `SetValue`, `GetValue`, `ListValues` functions to `config.go`:

```go
// Save writes the config to the given path atomically.
func Save(path string, cfg *Config) error {
	return writeDefaults(path, cfg)
}

// ToMap converts a Config to a generic map via JSON round-trip.
func ToMap(cfg *Config) (map[string]any, error) {
	data, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// ListValues returns all config values as a flat dot-notation map.
// If mask is true, secret values are masked.
func ListValues(cfg *Config, mask bool) (map[string]any, error) {
	m, err := ToMap(cfg)
	if err != nil {
		return nil, err
	}
	flat := Flatten(m)
	if mask {
		flat = MaskSecrets(flat)
	}
	return flat, nil
}

// GetValue reads a single config value by dot-notation key.
func GetValue(path, key string) (any, error) {
	cfg, err := Load(path)
	if err != nil {
		return nil, err
	}
	m, err := ToMap(cfg)
	if err != nil {
		return nil, err
	}
	flat := Flatten(m)
	val, ok := flat[key]
	if !ok {
		return nil, fmt.Errorf("unknown config key: %s", key)
	}
	return val, nil
}

// SetValue updates a single config value by dot-notation key.
// The value string is parsed as JSON first; if that fails, it's stored as a string.
func SetValue(path, key, value string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}

	flat := Flatten(m)

	// Try to parse value as JSON (handles numbers, booleans)
	var parsed any
	if err := json.Unmarshal([]byte(value), &parsed); err != nil {
		parsed = value // Store as string
	}
	flat[key] = parsed

	nested := Unflatten(flat)

	outData, err := json.MarshalIndent(nested, "", "  ")
	if err != nil {
		return err
	}
	outData = append(outData, '\n')

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, outData, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return nil
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/config/ -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: add Save, GetValue, SetValue, ListValues to config package"
```

---

### Task 4: Cobra root command and serve subcommand

**Files:**
- Rewrite: `cmd/gopherclaw/main.go` (cobra root + slog setup)
- Create: `cmd/gopherclaw/cmd_serve.go` (current daemon logic)

**Step 1: Rewrite main.go as cobra root**

```go
package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/user/gopherclaw/internal/config"
)

var (
	cfgPath string
	rootCmd = &cobra.Command{
		Use:   "gopherclaw",
		Short: "Single-binary AI assistant runtime",
	}
)

func init() {
	defaultPath := filepath.Join(os.Getenv("HOME"), ".gopherclaw", "config.json")
	rootCmd.PersistentFlags().StringVar(&cfgPath, "config", defaultPath, "config file path")
}

func loadConfig() *config.Config {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}
	return cfg
}

func setupLogging(cfg *config.Config) {
	var level slog.Level
	switch strings.ToLower(cfg.LogLevel) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
```

**Step 2: Create cmd_serve.go with current daemon logic**

Move all the daemon logic from the old main.go into a `serve` cobra command:

```go
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/spf13/cobra"

	ctxengine "github.com/user/gopherclaw/internal/context"
	"github.com/user/gopherclaw/internal/gateway"
	"github.com/user/gopherclaw/internal/runtime"
	"github.com/user/gopherclaw/internal/runtime/tools"
	"github.com/user/gopherclaw/internal/state"
	"github.com/user/gopherclaw/internal/telegram"
	"github.com/user/gopherclaw/pkg/llm"
	"github.com/user/gopherclaw/pkg/llm/openai"
)

func init() {
	rootCmd.AddCommand(serveCmd)
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the gopherclaw daemon",
	RunE:  runServe,
}

func runServe(cmd *cobra.Command, args []string) error {
	cfg := loadConfig()
	setupLogging(cfg)

	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	// Write PID file
	pidPath := filepath.Join(cfg.DataDir, "gopherclaw.pid")
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())), 0644); err != nil {
		return fmt.Errorf("write pid file: %w", err)
	}
	defer os.Remove(pidPath)

	// Stores
	sessions := state.NewSessionStore(cfg.DataDir)
	events := state.NewEventStore(cfg.DataDir)
	artifacts := state.NewArtifactStore(cfg.DataDir)

	// LLM provider
	provider := openai.New(&llm.Config{
		BaseURL:     cfg.LLM.BaseURL,
		APIKey:      cfg.LLM.APIKey,
		Model:       cfg.LLM.Model,
		MaxTokens:   cfg.LLM.MaxTokens,
		Temperature: cfg.LLM.Temperature,
	})

	// Context engine
	engine, err := ctxengine.New(cfg.LLM.Model, cfg.LLM.MaxContextTokens, cfg.LLM.OutputReserve)
	if err != nil {
		return fmt.Errorf("create context engine: %w", err)
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

	slog.Info("gopherclaw started",
		"data_dir", cfg.DataDir,
		"log_level", cfg.LogLevel,
		"max_concurrent", cfg.MaxConcurrent,
		"max_tool_rounds", cfg.MaxToolRounds,
		"llm_provider", cfg.LLM.Provider,
		"llm_model", cfg.LLM.Model,
		"pid_file", pidPath,
	)

	// Telegram adapter
	if cfg.Telegram.Token != "" {
		adapter, err := telegram.New(cfg.Telegram.Token, gw, events, sessions)
		if err != nil {
			return fmt.Errorf("create telegram adapter: %w", err)
		}
		go adapter.Start(ctx)
		slog.Info("telegram adapter started")
	} else {
		slog.Warn("telegram adapter disabled (no token)")
	}

	// Handle signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	for {
		sig := <-sigChan
		if sig == syscall.SIGHUP {
			slog.Info("received SIGHUP, restarting...")
			cancel()
			gw.Stop()
			// Re-exec ourselves
			exe, err := os.Executable()
			if err != nil {
				return fmt.Errorf("find executable: %w", err)
			}
			return syscall.Exec(exe, os.Args, os.Environ())
		}
		slog.Info("shutting down", "signal", sig.String())
		return nil
	}
}
```

**Step 3: Verify build**

Run: `go build ./cmd/gopherclaw/`
Expected: clean build

**Step 4: Verify `gopherclaw --help` and `gopherclaw serve --help`**

Run: `go run ./cmd/gopherclaw/ --help`
Expected: shows "serve" subcommand in list

**Step 5: Commit**

```bash
git add cmd/gopherclaw/main.go cmd/gopherclaw/cmd_serve.go
git commit -m "feat: cobra root command with serve subcommand and PID file"
```

---

### Task 5: Config list/get/set commands

**Files:**
- Create: `cmd/gopherclaw/cmd_config.go`

**Step 1: Write the config commands**

```go
package main

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"github.com/user/gopherclaw/internal/config"
)

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configListCmd)
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configSetCmd)
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage configuration",
}

var configListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all configuration values",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := loadConfig()
		flat, err := config.ListValues(cfg, true)
		if err != nil {
			return err
		}

		keys := make([]string, 0, len(flat))
		for k := range flat {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			fmt.Printf("%s = %v\n", k, flat[k])
		}
		return nil
	},
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a configuration value",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		val, err := config.GetValue(cfgPath, args[0])
		if err != nil {
			return err
		}
		fmt.Println(val)
		return nil
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.SetValue(cfgPath, args[0], args[1]); err != nil {
			return err
		}
		fmt.Printf("%s = %s\n", args[0], args[1])
		return nil
	},
}
```

**Step 2: Verify build and manual test**

Run: `go build -o /tmp/gc ./cmd/gopherclaw/ && /tmp/gc config list`
Expected: prints all config values with masked secrets

Run: `/tmp/gc config get llm.model`
Expected: prints current model name

Run: `/tmp/gc config set llm.model test-model && /tmp/gc config get llm.model`
Expected: prints "test-model"

**Step 3: Commit**

```bash
git add cmd/gopherclaw/cmd_config.go
git commit -m "feat: add config list/get/set CLI commands"
```

---

### Task 6: Session list/clear commands

**Files:**
- Create: `cmd/gopherclaw/cmd_session.go`

**Step 1: Write the session commands**

```go
package main

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/user/gopherclaw/internal/state"
)

func init() {
	rootCmd.AddCommand(sessionCmd)
	sessionCmd.AddCommand(sessionListCmd)
	sessionCmd.AddCommand(sessionClearCmd)
}

var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Manage sessions",
}

var sessionListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all sessions",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := loadConfig()
		sessions := state.NewSessionStore(cfg.DataDir)
		events := state.NewEventStore(cfg.DataDir)
		ctx := context.Background()

		list, err := sessions.List(ctx)
		if err != nil {
			return err
		}

		if len(list) == 0 {
			fmt.Println("No sessions found.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tSTATUS\tMESSAGES\tCREATED")
		for _, s := range list {
			count, _ := events.Count(ctx, s.SessionID)
			fmt.Fprintf(w, "%s\t%s\t%d\t%s\n",
				s.SessionID,
				s.Status,
				count,
				s.CreatedAt.Format("2006-01-02 15:04"),
			)
		}
		w.Flush()
		return nil
	},
}

var sessionClearCmd = &cobra.Command{
	Use:   "clear <session-id|all>",
	Short: "Clear session data",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := loadConfig()
		sessionsDir := cfg.DataDir + "/sessions"

		if args[0] == "all" {
			if err := os.RemoveAll(sessionsDir); err != nil {
				return fmt.Errorf("clear sessions: %w", err)
			}
			fmt.Println("All sessions cleared.")
			return nil
		}

		sessionDir := sessionsDir + "/" + args[0]
		if _, err := os.Stat(sessionDir); os.IsNotExist(err) {
			return fmt.Errorf("session %s not found", args[0])
		}
		if err := os.RemoveAll(sessionDir); err != nil {
			return fmt.Errorf("clear session: %w", err)
		}
		fmt.Printf("Session %s cleared.\n", args[0])
		return nil
	},
}
```

**Step 2: Verify build**

Run: `go build -o /tmp/gc ./cmd/gopherclaw/ && /tmp/gc session list`
Expected: lists sessions or "No sessions found."

**Step 3: Commit**

```bash
git add cmd/gopherclaw/cmd_session.go
git commit -m "feat: add session list/clear CLI commands"
```

---

### Task 7: Lifecycle commands (stop/restart)

**Files:**
- Create: `cmd/gopherclaw/cmd_lifecycle.go`

**Step 1: Write lifecycle commands**

```go
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(restartCmd)
}

func readPID() (int, error) {
	cfg := loadConfig()
	pidPath := filepath.Join(cfg.DataDir, "gopherclaw.pid")
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return 0, fmt.Errorf("no running daemon found (pid file missing: %s)", pidPath)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("invalid pid file: %w", err)
	}
	// Check if process exists
	process, err := os.FindProcess(pid)
	if err != nil {
		return 0, fmt.Errorf("process %d not found: %w", pid, err)
	}
	// On Unix, FindProcess always succeeds. Send signal 0 to check.
	if err := process.Signal(syscall.Signal(0)); err != nil {
		return 0, fmt.Errorf("daemon not running (pid %d): %w", pid, err)
	}
	return pid, nil
}

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the running daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		pid, err := readPID()
		if err != nil {
			return err
		}
		process, _ := os.FindProcess(pid)
		if err := process.Signal(syscall.SIGTERM); err != nil {
			return fmt.Errorf("send SIGTERM: %w", err)
		}
		fmt.Printf("Sent SIGTERM to gopherclaw (pid %d)\n", pid)
		return nil
	},
}

var restartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart the running daemon (reload config)",
	RunE: func(cmd *cobra.Command, args []string) error {
		pid, err := readPID()
		if err != nil {
			return err
		}
		process, _ := os.FindProcess(pid)
		if err := process.Signal(syscall.SIGHUP); err != nil {
			return fmt.Errorf("send SIGHUP: %w", err)
		}
		fmt.Printf("Sent SIGHUP to gopherclaw (pid %d) — restarting\n", pid)
		return nil
	},
}
```

**Step 2: Verify build**

Run: `go build -o /tmp/gc ./cmd/gopherclaw/ && /tmp/gc stop`
Expected: "no running daemon found (pid file missing)" error

**Step 3: Commit**

```bash
git add cmd/gopherclaw/cmd_lifecycle.go
git commit -m "feat: add stop/restart lifecycle CLI commands with PID file"
```

---

### Task 8: Setup wizard

**Files:**
- Create: `cmd/gopherclaw/cmd_setup.go`

**Step 1: Write setup command**

```go
package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/user/gopherclaw/internal/config"
)

func init() {
	rootCmd.AddCommand(setupCmd)
}

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Interactive first-time configuration wizard",
	RunE:  runSetup,
}

func runSetup(cmd *cobra.Command, args []string) error {
	scanner := bufio.NewScanner(os.Stdin)

	fmt.Println("Gopherclaw Setup")
	fmt.Println("=================")
	fmt.Println()

	cfg, _ := config.Load(cfgPath)

	cfg.LLM.BaseURL = prompt(scanner, "LLM base URL", cfg.LLM.BaseURL)
	cfg.LLM.APIKey = prompt(scanner, "LLM API key", cfg.LLM.APIKey)
	cfg.LLM.Model = prompt(scanner, "LLM model", cfg.LLM.Model)

	maxTokensStr := prompt(scanner, "Max output tokens", fmt.Sprintf("%d", cfg.LLM.MaxTokens))
	if v := parseInt(maxTokensStr); v > 0 {
		cfg.LLM.MaxTokens = v
	}

	token := prompt(scanner, "Telegram bot token (empty to skip)", cfg.Telegram.Token)
	cfg.Telegram.Token = token

	braveKey := prompt(scanner, "Brave Search API key (empty to skip)", cfg.Brave.APIKey)
	cfg.Brave.APIKey = braveKey

	if err := config.Save(cfgPath, cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	fmt.Printf("\nConfig saved to %s\n", cfgPath)
	fmt.Println("Run 'gopherclaw serve' to start the daemon.")
	return nil
}

func prompt(scanner *bufio.Scanner, label, defaultVal string) string {
	if defaultVal != "" {
		fmt.Printf("%s [%s]: ", label, defaultVal)
	} else {
		fmt.Printf("%s: ", label)
	}
	scanner.Scan()
	val := strings.TrimSpace(scanner.Text())
	if val == "" {
		return defaultVal
	}
	return val
}

func parseInt(s string) int {
	var v int
	fmt.Sscanf(s, "%d", &v)
	return v
}
```

**Step 2: Verify build**

Run: `go build -o /tmp/gc ./cmd/gopherclaw/ && /tmp/gc setup --help`
Expected: shows setup command help

**Step 3: Commit**

```bash
git add cmd/gopherclaw/cmd_setup.go
git commit -m "feat: add interactive setup wizard CLI command"
```

---

### Task 9: Build verification and final tests

**Step 1: Build clean**

Run: `go build ./...`
Expected: clean

**Step 2: Run all tests**

Run: `go test ./... -race -count=1`
Expected: all pass

**Step 3: Verify CLI end-to-end**

Run: `go build -o /tmp/gc ./cmd/gopherclaw/`

Verify help:
Run: `/tmp/gc --help`
Expected: shows serve, config, session, stop, restart, setup

Verify config flow:
Run: `/tmp/gc config list && /tmp/gc config get llm.model && /tmp/gc config set llm.model test && /tmp/gc config get llm.model`

Verify session list:
Run: `/tmp/gc session list`

**Step 4: Final commit if any fixes needed**

---

### Task 10: Update Makefile if exists

Check if `make run` needs updating from bare binary to `gopherclaw serve`.

**Step 1: Check Makefile**

If Makefile exists and has a `run` target, update it to use `gopherclaw serve`.

**Step 2: Commit if changed**
