package main

import (
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/user/gopherclaw/internal/state"
)

func init() {
	rootCmd.AddCommand(taskCmd)
	taskCmd.AddCommand(taskAddCmd, taskListCmd, taskRemoveCmd, taskEnableCmd, taskDisableCmd)

	taskAddCmd.Flags().String("name", "", "task name (required)")
	taskAddCmd.Flags().String("prompt", "", "prompt text (required)")
	taskAddCmd.Flags().String("schedule", "", "cron schedule expression")
	taskAddCmd.Flags().String("session-key", "", "session key (required)")
	_ = taskAddCmd.MarkFlagRequired("name")
	_ = taskAddCmd.MarkFlagRequired("prompt")
	_ = taskAddCmd.MarkFlagRequired("session-key")
}

func taskStore() *state.TaskStore {
	cfg := loadConfig()
	return state.NewTaskStore(filepath.Join(cfg.DataDir, "tasks.json"))
}

var taskCmd = &cobra.Command{
	Use:   "task",
	Short: "Manage tasks",
}

var taskAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a new task",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		name, _ := cmd.Flags().GetString("name")
		prompt, _ := cmd.Flags().GetString("prompt")
		schedule, _ := cmd.Flags().GetString("schedule")
		sessionKey, _ := cmd.Flags().GetString("session-key")

		store := taskStore()
		task := &state.Task{
			Name:       name,
			Prompt:     prompt,
			Schedule:   schedule,
			SessionKey: sessionKey,
			Enabled:    true,
		}
		if err := store.Add(task); err != nil {
			return fmt.Errorf("add task: %w", err)
		}
		fmt.Fprintf(os.Stdout, "Task %q added.\n", name)
		return nil
	},
}

var taskListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all tasks",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		store := taskStore()
		tasks, err := store.List()
		if err != nil {
			return fmt.Errorf("list tasks: %w", err)
		}

		if len(tasks) == 0 {
			fmt.Println("No tasks configured.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tSCHEDULE\tENABLED\tSESSION KEY")
		for _, t := range tasks {
			fmt.Fprintf(w, "%s\t%s\t%v\t%s\n",
				t.Name,
				t.Schedule,
				t.Enabled,
				t.SessionKey,
			)
		}
		return w.Flush()
	},
}

var taskRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove a task",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store := taskStore()
		if err := store.Remove(args[0]); err != nil {
			return fmt.Errorf("remove task: %w", err)
		}
		fmt.Fprintf(os.Stdout, "Task %q removed.\n", args[0])
		return nil
	},
}

var taskEnableCmd = &cobra.Command{
	Use:   "enable <name>",
	Short: "Enable a task",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store := taskStore()
		if err := store.SetEnabled(args[0], true); err != nil {
			return fmt.Errorf("enable task: %w", err)
		}
		fmt.Fprintf(os.Stdout, "Task %q enabled.\n", args[0])
		return nil
	},
}

var taskDisableCmd = &cobra.Command{
	Use:   "disable <name>",
	Short: "Disable a task",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store := taskStore()
		if err := store.SetEnabled(args[0], false); err != nil {
			return fmt.Errorf("disable task: %w", err)
		}
		fmt.Fprintf(os.Stdout, "Task %q disabled.\n", args[0])
		return nil
	},
}
