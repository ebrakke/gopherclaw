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

func writePIDFile(dataDir string) (string, error) {
	pidPath := filepath.Join(dataDir, "gopherclaw.pid")
	pid := os.Getpid()
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(pid)+"\n"), 0644); err != nil {
		return "", fmt.Errorf("write PID file: %w", err)
	}
	return pidPath, nil
}

func runServe(cmd *cobra.Command, args []string) error {
	cfg := loadConfig()
	setupLogging(cfg)

	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	// Write PID file
	pidPath, err := writePIDFile(cfg.DataDir)
	if err != nil {
		return err
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

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	for {
		sig := <-sigChan
		if sig == syscall.SIGHUP {
			slog.Info("received SIGHUP, restarting")
			execPath, err := os.Executable()
			if err != nil {
				slog.Error("failed to get executable path", "error", err)
				continue
			}
			// Clean up PID file before re-exec
			os.Remove(pidPath)
			if err := syscall.Exec(execPath, os.Args, os.Environ()); err != nil {
				slog.Error("failed to re-exec", "error", err)
				// Re-write PID file since we failed to re-exec
				if _, writeErr := writePIDFile(cfg.DataDir); writeErr != nil {
					slog.Error("failed to re-write PID file", "error", writeErr)
				}
				continue
			}
		}
		// SIGINT or SIGTERM
		slog.Info("shutting down", "signal", sig)
		return nil
	}
}
