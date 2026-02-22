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
	"github.com/user/gopherclaw/pkg/llm"
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
