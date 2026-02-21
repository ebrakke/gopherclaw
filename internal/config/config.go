package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	DataDir       string `json:"data_dir"`
	MaxConcurrent int    `json:"max_concurrent"`
	LLM           struct {
		Provider    string  `json:"provider"`
		BaseURL     string  `json:"base_url"`
		APIKey      string  `json:"api_key"`
		Model       string  `json:"model"`
		MaxTokens   int     `json:"max_tokens"`
		Temperature float32 `json:"temperature"`
	} `json:"llm"`
}

func Load(path string) (*Config, error) {
	cfg := &Config{
		DataDir:       filepath.Join(os.Getenv("HOME"), ".gopherclaw"),
		MaxConcurrent: 2,
	}
	cfg.LLM.Provider = "openai"
	cfg.LLM.BaseURL = "https://api.openai.com"
	cfg.LLM.Model = "gpt-3.5-turbo"
	cfg.LLM.MaxTokens = 2000
	cfg.LLM.Temperature = 0.7

	// Load from file if exists, otherwise write defaults
	if _, err := os.Stat(path); err == nil {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(data, cfg); err != nil {
			return nil, err
		}
	} else if os.IsNotExist(err) {
		if err := writeDefaults(path, cfg); err != nil {
			return nil, err
		}
	}

	// Override from env (highest precedence)
	if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
		cfg.LLM.APIKey = apiKey
	}
	if baseURL := os.Getenv("OPENAI_BASE_URL"); baseURL != "" {
		cfg.LLM.BaseURL = baseURL
	}

	return cfg, nil
}

func writeDefaults(path string, cfg *Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal default config: %w", err)
	}
	data = append(data, '\n')
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("write default config: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename default config: %w", err)
	}
	return nil
}
