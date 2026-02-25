package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/spf13/cobra"
	ctxengine "github.com/user/gopherclaw/internal/context"
	"github.com/user/gopherclaw/internal/delivery"
	"github.com/user/gopherclaw/internal/gateway"
	"github.com/user/gopherclaw/internal/runtime"
	"github.com/user/gopherclaw/internal/runtime/tools"
	"github.com/user/gopherclaw/internal/scheduler"
	"github.com/user/gopherclaw/internal/state"
	"github.com/user/gopherclaw/internal/telegram"
	"github.com/user/gopherclaw/internal/types"
	"github.com/user/gopherclaw/internal/webhook"
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
	engine, err := ctxengine.New(cfg.LLM.Model, cfg.LLM.MaxContextTokens, cfg.LLM.OutputReserve, cfg.SystemPromptPath)
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

	// Memory tools
	memoryPath := filepath.Join(cfg.DataDir, "memory.md")
	registry.Register(tools.NewMemorySave(memoryPath))
	registry.Register(tools.NewMemoryDelete(memoryPath))
	registry.Register(tools.NewMemoryList(memoryPath))

	// Wire memory path into context engine
	engine.SetMemoryPath(memoryPath)

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

	// Collect tool names for context summary
	var toolNames []string
	for _, t := range registry.All() {
		toolNames = append(toolNames, t.Name())
	}

	// Task store
	taskStore := state.NewTaskStore(filepath.Join(cfg.DataDir, "tasks.json"))

	// Delivery registry
	deliveryReg := delivery.NewRegistry()

	// Telegram adapter
	if cfg.Telegram.Token != "" {
		adapter, err := telegram.New(cfg.Telegram.Token, gw, events, sessions, engine, toolNames, memoryPath)
		if err != nil {
			return fmt.Errorf("create telegram adapter: %w", err)
		}
		go adapter.Start(ctx)
		slog.Info("telegram adapter started")

		// Register telegram delivery for cron responses
		deliveryReg.Register("telegram:", func(sessionKey, message string) error {
			return adapter.SendTo(sessionKey, message)
		})
	} else {
		slog.Warn("telegram adapter disabled (no token)")
	}

	// Helper: synchronously process a task through the gateway and return the response.
	processTask := func(sessionKey, prompt string) (string, error) {
		done := make(chan string, 1)
		event := &types.InboundEvent{
			Source:     "task",
			SessionKey: types.SessionKey(sessionKey),
			UserID:     "system",
			Text:       prompt,
		}
		if err := gw.HandleInbound(ctx, event, gateway.WithOnComplete(func(response string) {
			done <- response
		})); err != nil {
			return "", err
		}
		return <-done, nil
	}

	// Scheduler
	sched := scheduler.New(taskStore, func(sessionKey, prompt string) {
		response, err := processTask(sessionKey, prompt)
		if err != nil {
			slog.Error("cron task failed", "session_key", sessionKey, "error", err)
			return
		}
		if response == "" {
			return // bot decided not to respond
		}
		if err := deliveryReg.Deliver(sessionKey, response); err != nil {
			slog.Error("cron delivery failed", "session_key", sessionKey, "error", err)
		}
	})
	if err := sched.Start(); err != nil {
		return fmt.Errorf("start scheduler: %w", err)
	}
	defer sched.Stop()
	slog.Info("scheduler started")

	// Webhook HTTP server
	if cfg.HTTP.Enabled {
		webhookSrv := webhook.NewServer(taskStore, processTask)
		httpServer := &http.Server{
			Addr:    cfg.HTTP.Listen,
			Handler: webhookSrv,
		}
		go func() {
			slog.Info("webhook server started", "listen", cfg.HTTP.Listen)
			if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				slog.Error("webhook server error", "error", err)
			}
		}()
		go func() {
			<-ctx.Done()
			httpServer.Close()
		}()
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
