package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/user/gopherclaw/internal/config"
)

func init() {
	rootCmd.AddCommand(setupCmd)
}

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Interactive setup wizard",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := loadConfig()
		scanner := bufio.NewScanner(os.Stdin)

		fmt.Println("GopherClaw Setup Wizard")
		fmt.Println("Press Enter to accept the default value shown in brackets.")
		fmt.Println()

		// 1. LLM base URL
		cfg.LLM.BaseURL = prompt(scanner, "LLM base URL", cfg.LLM.BaseURL)

		// 2. LLM API key
		cfg.LLM.APIKey = prompt(scanner, "LLM API key", cfg.LLM.APIKey)

		// 3. LLM model name
		cfg.LLM.Model = prompt(scanner, "LLM model name", cfg.LLM.Model)

		// 4. Max output tokens
		maxTokensStr := prompt(scanner, "Max output tokens", strconv.Itoa(cfg.LLM.MaxTokens))
		if n, err := strconv.Atoi(maxTokensStr); err == nil {
			cfg.LLM.MaxTokens = n
		}

		// 5. Telegram bot token (optional)
		cfg.Telegram.Token = prompt(scanner, "Telegram bot token (optional)", cfg.Telegram.Token)

		// 6. Brave API key (optional)
		cfg.Brave.APIKey = prompt(scanner, "Brave API key (optional)", cfg.Brave.APIKey)

		if err := config.Save(cfgPath, cfg); err != nil {
			return fmt.Errorf("save config: %w", err)
		}

		fmt.Println()
		fmt.Println("Configuration saved to", cfgPath)
		return nil
	},
}

// prompt displays a labeled prompt with a default value and reads user input.
// If the user enters nothing, the default is returned.
func prompt(scanner *bufio.Scanner, label, defaultVal string) string {
	if defaultVal != "" {
		fmt.Printf("%s [%s]: ", label, defaultVal)
	} else {
		fmt.Printf("%s: ", label)
	}
	if scanner.Scan() {
		input := strings.TrimSpace(scanner.Text())
		if input != "" {
			return input
		}
	}
	return defaultVal
}
