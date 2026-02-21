package config

import (
	"encoding/json"
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

	// Load from file if exists
	if _, err := os.Stat(path); err == nil {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(data, cfg); err != nil {
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
