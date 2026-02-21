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
	"github.com/user/gopherclaw/internal/gateway"
	"github.com/user/gopherclaw/internal/state"
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

	sessions := state.NewSessionStore(cfg.DataDir)
	events := state.NewEventStore(cfg.DataDir)
	artifacts := state.NewArtifactStore(cfg.DataDir)

	gw := gateway.New(sessions, events, artifacts, int64(cfg.MaxConcurrent))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	gw.Start(ctx)
	defer gw.Stop()

	fmt.Printf("Gopherclaw started\n")
	fmt.Printf("Data directory: %s\n", cfg.DataDir)
	fmt.Printf("Max concurrent runs: %d\n", cfg.MaxConcurrent)
	fmt.Printf("LLM provider: %s\n", cfg.LLM.Provider)
	fmt.Printf("Model: %s\n", cfg.LLM.Model)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nShutting down...")
}
