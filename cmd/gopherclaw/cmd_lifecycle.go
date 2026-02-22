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
	rootCmd.AddCommand(stopCmd, restartCmd)
}

// readPID reads the PID from the gopherclaw.pid file and validates the
// process exists by sending signal 0.
func readPID() (int, error) {
	cfg := loadConfig()
	pidPath := filepath.Join(cfg.DataDir, "gopherclaw.pid")

	data, err := os.ReadFile(pidPath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, fmt.Errorf("no running daemon (PID file not found)")
		}
		return 0, fmt.Errorf("read PID file: %w", err)
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("invalid PID file content: %w", err)
	}

	// Check if process exists
	proc, err := os.FindProcess(pid)
	if err != nil {
		return 0, fmt.Errorf("find process %d: %w", pid, err)
	}
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		return 0, fmt.Errorf("no running daemon (process %d not found)", pid)
	}

	return pid, nil
}

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the running daemon",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		pid, err := readPID()
		if err != nil {
			return err
		}

		proc, err := os.FindProcess(pid)
		if err != nil {
			return fmt.Errorf("find process: %w", err)
		}
		if err := proc.Signal(syscall.SIGTERM); err != nil {
			return fmt.Errorf("send SIGTERM: %w", err)
		}

		fmt.Fprintf(os.Stdout, "Sent SIGTERM to daemon (PID %d).\n", pid)
		return nil
	},
}

var restartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart the running daemon",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		pid, err := readPID()
		if err != nil {
			return err
		}

		proc, err := os.FindProcess(pid)
		if err != nil {
			return fmt.Errorf("find process: %w", err)
		}
		if err := proc.Signal(syscall.SIGHUP); err != nil {
			return fmt.Errorf("send SIGHUP: %w", err)
		}

		fmt.Fprintf(os.Stdout, "Sent SIGHUP to daemon (PID %d) for restart.\n", pid)
		return nil
	},
}
