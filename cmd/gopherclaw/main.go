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
