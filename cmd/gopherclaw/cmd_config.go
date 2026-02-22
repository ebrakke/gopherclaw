package main

import (
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"
	"github.com/user/gopherclaw/internal/config"
)

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configListCmd, configGetCmd, configSetCmd)
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage configuration",
}

var configListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all configuration values",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := loadConfig()
		values, err := config.ListValues(cfg, true)
		if err != nil {
			return fmt.Errorf("list config: %w", err)
		}

		// Sort keys for stable output
		keys := make([]string, 0, len(values))
		for k := range values {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			fmt.Fprintf(os.Stdout, "%s = %v\n", k, values[k])
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
		fmt.Fprintln(os.Stdout, val)
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
		display := args[1]
		if config.IsSecretKey(args[0]) {
			display = "***"
		}
		fmt.Fprintf(os.Stdout, "Set %s = %s\n", args[0], display)
		return nil
	},
}
