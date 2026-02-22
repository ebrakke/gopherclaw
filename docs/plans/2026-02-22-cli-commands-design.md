# CLI Commands Design

## Goal

Add a cobra-based CLI to gopherclaw so that both humans and the bot itself (via the bash tool) can manage configuration, sessions, and the daemon lifecycle.

## Command Tree

```
gopherclaw serve                          # start daemon (current main.go behavior)
gopherclaw config list                    # print all config key=value pairs
gopherclaw config get <key>               # get single value (dot notation: llm.model)
gopherclaw config set <key> <value>       # set a value and write config file
gopherclaw session list                   # list sessions with status and message count
gopherclaw session clear <id|all>         # delete session data
gopherclaw restart                        # send SIGHUP to running daemon
gopherclaw stop                           # send SIGTERM to running daemon
gopherclaw setup                          # interactive first-time config wizard
```

All commands accept `--config <path>` (default `~/.gopherclaw/config.json`).

## Design Decisions

### Cobra framework

Industry standard for Go CLIs. Gives us subcommand routing, auto-generated help, shell completion, and flag parsing. Single dependency, widely used (kubectl, docker, gh).

### Config get/set with dot notation

Flatten the nested JSON config into dot-separated keys for the CLI:

```
gopherclaw config get llm.model          → gpt-3.5-turbo
gopherclaw config set llm.model gpt-4o
gopherclaw config set llm.api_key sk-...
gopherclaw config set telegram.token 123:ABC
gopherclaw config list                   → prints all keys and values
```

Implementation: use `encoding/json` to marshal config to `map[string]any`, then flatten recursively. For `set`, unflatten back to nested map, marshal to JSON, and write the file. This avoids reflect and works with the existing JSON config format.

API keys are masked in `config list` output (show last 4 chars only).

### PID file for lifecycle commands

`serve` writes a PID file to `<data_dir>/gopherclaw.pid` on startup and removes it on shutdown. `restart` and `stop` read this file to find the running process.

- `stop` sends SIGTERM (already handled by serve)
- `restart` sends SIGHUP — serve catches it, reloads config, and re-initializes components

### SIGHUP reload in serve

When serve catches SIGHUP:
1. Reload config from file
2. Log the reload
3. Note: some components (LLM provider, context engine) are cheap to recreate. The gateway and telegram adapter keep running — only the runtime's config-dependent state updates.

For v1, SIGHUP triggers a full process restart via `syscall.Exec` (re-exec the same binary). This is simpler than hot-reloading individual components and guarantees clean state. The PID stays the same from the parent's perspective.

### Session list/clear

`session list` reads session files from `<data_dir>/sessions/` and prints a table:

```
ID                                    STATUS    MESSAGES  CREATED
7d5b71da-e03b-4451-8d0b-8d21f3ebff3c  active    42        2026-02-22
a1b2c3d4-...                          archived  15        2026-02-21
```

Message count comes from counting events for each session.

`session clear <id>` deletes the session file and its events. `session clear all` clears everything.

### Setup wizard

Interactive prompts for first-time configuration:

1. LLM provider base URL (default: https://api.openai.com/v1)
2. LLM API key
3. LLM model name (default: gpt-4o)
4. Telegram bot token (optional, press enter to skip)
5. Brave API key (optional, press enter to skip)

Uses `bufio.Scanner` to read stdin. Writes the config file at the end. No external prompt library needed.

## Package Layout

```
cmd/gopherclaw/
  main.go              → cobra root command + global --config flag
  cmd_serve.go         → serve subcommand (current main.go logic)
  cmd_config.go        → config list/get/set subcommands
  cmd_session.go       → session list/clear subcommands
  cmd_lifecycle.go     → restart/stop subcommands
  cmd_setup.go         → setup wizard
internal/config/
  config.go            → existing, add Save() method
  flatten.go           → dot-notation flatten/unflatten helpers
```

## Bot Self-Management

The bot can run any of these commands via its bash tool:

```
bash: gopherclaw config set llm.model gpt-4o
bash: gopherclaw config set log_level debug
bash: gopherclaw restart
bash: gopherclaw session clear all
```

No special integration needed — the CLI operates on the same config file the daemon reads.
