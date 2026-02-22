package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/user/gopherclaw/internal/state"
)

func init() {
	rootCmd.AddCommand(sessionCmd)
	sessionCmd.AddCommand(sessionListCmd, sessionClearCmd)
}

var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Manage sessions",
}

var sessionListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all sessions",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := loadConfig()
		sessions := state.NewSessionStore(cfg.DataDir)
		events := state.NewEventStore(cfg.DataDir)

		ctx := context.Background()
		list, err := sessions.List(ctx)
		if err != nil {
			return fmt.Errorf("list sessions: %w", err)
		}

		if len(list) == 0 {
			fmt.Println("No sessions found.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tSTATUS\tMESSAGES\tCREATED")
		for _, s := range list {
			count, err := events.Count(ctx, s.SessionID)
			if err != nil {
				count = 0
			}
			fmt.Fprintf(w, "%s\t%s\t%d\t%s\n",
				s.SessionID,
				s.Status,
				count,
				s.CreatedAt.Format("2006-01-02 15:04:05"),
			)
		}
		return w.Flush()
	},
}

var sessionClearCmd = &cobra.Command{
	Use:   "clear <id|all>",
	Short: "Clear a session or all sessions",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := loadConfig()
		sessionsDir := filepath.Join(cfg.DataDir, "sessions")

		if args[0] == "all" {
			if err := os.RemoveAll(sessionsDir); err != nil {
				return fmt.Errorf("remove sessions directory: %w", err)
			}
			fmt.Println("All sessions cleared.")
			return nil
		}

		// Remove specific session directory (validate path to prevent traversal)
		sessionDir := filepath.Join(sessionsDir, args[0])
		resolved, err := filepath.Abs(sessionDir)
		if err != nil {
			return fmt.Errorf("resolve path: %w", err)
		}
		absSessionsDir, _ := filepath.Abs(sessionsDir)
		if !strings.HasPrefix(resolved, absSessionsDir+string(filepath.Separator)) {
			return fmt.Errorf("invalid session ID: %s", args[0])
		}
		if _, err := os.Stat(sessionDir); os.IsNotExist(err) {
			return fmt.Errorf("session not found: %s", args[0])
		}
		if err := os.RemoveAll(sessionDir); err != nil {
			return fmt.Errorf("remove session directory: %w", err)
		}
		fmt.Fprintf(os.Stdout, "Session %s cleared.\n", args[0])
		return nil
	},
}
