package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	DataDir       string `json:"data_dir"`
	LogLevel      string `json:"log_level"`
	MaxConcurrent int    `json:"max_concurrent"`
	MaxToolRounds int    `json:"max_tool_rounds"`
	LLM           struct {
		Provider         string  `json:"provider"`
		BaseURL          string  `json:"base_url"`
		APIKey           string  `json:"api_key"`
		Model            string  `json:"model"`
		MaxTokens        int     `json:"max_tokens"`
		Temperature      float32 `json:"temperature"`
		MaxContextTokens int     `json:"max_context_tokens"`
		OutputReserve    int     `json:"output_reserve"`
	} `json:"llm"`
	Brave struct {
		APIKey string `json:"api_key"`
	} `json:"brave"`
	Telegram struct {
		Token string `json:"token"`
	} `json:"telegram"`
}

func Load(path string) (*Config, error) {
	cfg := &Config{
		DataDir:       filepath.Join(os.Getenv("HOME"), ".gopherclaw"),
		MaxConcurrent: 2,
	}
	cfg.LogLevel = "info"
	cfg.MaxToolRounds = 10
	cfg.LLM.Provider = "openai"
	cfg.LLM.BaseURL = "https://api.openai.com/v1"
	cfg.LLM.Model = "gpt-3.5-turbo"
	cfg.LLM.MaxTokens = 2000
	cfg.LLM.Temperature = 0.7
	cfg.LLM.MaxContextTokens = 128000
	cfg.LLM.OutputReserve = 4096

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
	if braveKey := os.Getenv("BRAVE_API_KEY"); braveKey != "" {
		cfg.Brave.APIKey = braveKey
	}
	if tgToken := os.Getenv("TELEGRAM_BOT_TOKEN"); tgToken != "" {
		cfg.Telegram.Token = tgToken
	}

	return cfg, nil
}

// Save writes the config to the given path using an atomic write
// (temp file + rename).
func Save(path string, cfg *Config) error {
	return writeDefaults(path, cfg)
}

// ToMap converts a Config struct into a generic map[string]any via JSON
// round-trip.
func ToMap(cfg *Config) (map[string]any, error) {
	data, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("unmarshal config to map: %w", err)
	}
	return m, nil
}

// ListValues returns all config values as a flat map with dot-separated keys.
// If mask is true, secret values are masked.
func ListValues(cfg *Config, mask bool) (map[string]any, error) {
	m, err := ToMap(cfg)
	if err != nil {
		return nil, err
	}
	flat := Flatten(m)
	if mask {
		flat = MaskSecrets(flat)
	}
	return flat, nil
}

// GetValue reads the raw JSON config from path, flattens it, and returns
// the value for the given dot-separated key. If the file does not exist,
// it is created with defaults first (via Load). Returns an error if the
// key is not found.
func GetValue(path, key string) (any, error) {
	// Ensure the file exists (creates with defaults if needed)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if _, err := Load(path); err != nil {
			return nil, fmt.Errorf("load config: %w", err)
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	flat := Flatten(raw)
	v, ok := flat[key]
	if !ok {
		return nil, fmt.Errorf("unknown config key: %s", key)
	}
	return v, nil
}

// SetValue reads the raw JSON config from path, flattens it, sets the given
// key to value, unflattens, and writes back atomically. The value string is
// first parsed as JSON (to handle numbers, booleans, null); if that fails,
// it is stored as a plain string.
func SetValue(path, key, value string) error {
	// Read existing raw JSON
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	flat := Flatten(raw)

	// Try to parse value as JSON (number, bool, null, etc.)
	var parsed any
	if err := json.Unmarshal([]byte(value), &parsed); err != nil {
		// Not valid JSON, treat as string
		parsed = value
	}
	flat[key] = parsed

	nested := Unflatten(flat)

	// Write back via atomic write: marshal, temp file, rename
	out, err := json.MarshalIndent(nested, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	out = append(out, '\n')

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, out, 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename config: %w", err)
	}
	return nil
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
