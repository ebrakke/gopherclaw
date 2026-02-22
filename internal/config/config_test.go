package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func tempConfigPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return filepath.Join(dir, "config.json")
}

func writeTestConfig(t *testing.T, path string, cfg *Config) {
	t.Helper()
	if err := Save(path, cfg); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}
}

func TestSave_ReloadRoundTrip(t *testing.T) {
	path := tempConfigPath(t)

	original := &Config{
		DataDir:       "/tmp/test-data",
		LogLevel:      "debug",
		MaxConcurrent: 4,
		MaxToolRounds: 20,
	}
	original.LLM.Provider = "openai"
	original.LLM.BaseURL = "https://api.openai.com/v1"
	original.LLM.APIKey = "sk-test-round-trip"
	original.LLM.Model = "gpt-4"
	original.LLM.MaxTokens = 4000
	original.LLM.Temperature = 0.5
	original.LLM.MaxContextTokens = 128000
	original.LLM.OutputReserve = 4096
	original.Brave.APIKey = "brave-key-123"
	original.Telegram.Token = "bot-token-456"

	// Save
	if err := Save(path, original); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("config file does not exist after Save: %v", err)
	}

	// Reload
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Compare key fields
	if loaded.DataDir != original.DataDir {
		t.Errorf("DataDir mismatch: %v != %v", loaded.DataDir, original.DataDir)
	}
	if loaded.LogLevel != original.LogLevel {
		t.Errorf("LogLevel mismatch: %v != %v", loaded.LogLevel, original.LogLevel)
	}
	if loaded.MaxConcurrent != original.MaxConcurrent {
		t.Errorf("MaxConcurrent mismatch: %v != %v", loaded.MaxConcurrent, original.MaxConcurrent)
	}
	if loaded.LLM.Provider != original.LLM.Provider {
		t.Errorf("LLM.Provider mismatch: %v != %v", loaded.LLM.Provider, original.LLM.Provider)
	}
	if loaded.LLM.APIKey != original.LLM.APIKey {
		t.Errorf("LLM.APIKey mismatch: %v != %v", loaded.LLM.APIKey, original.LLM.APIKey)
	}
	if loaded.LLM.Model != original.LLM.Model {
		t.Errorf("LLM.Model mismatch: %v != %v", loaded.LLM.Model, original.LLM.Model)
	}
	if loaded.LLM.Temperature != original.LLM.Temperature {
		t.Errorf("LLM.Temperature mismatch: %v != %v", loaded.LLM.Temperature, original.LLM.Temperature)
	}
	if loaded.Brave.APIKey != original.Brave.APIKey {
		t.Errorf("Brave.APIKey mismatch: %v != %v", loaded.Brave.APIKey, original.Brave.APIKey)
	}
	if loaded.Telegram.Token != original.Telegram.Token {
		t.Errorf("Telegram.Token mismatch: %v != %v", loaded.Telegram.Token, original.Telegram.Token)
	}
}

func TestSave_AtomicWrite(t *testing.T) {
	path := tempConfigPath(t)

	cfg := &Config{LogLevel: "info"}
	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify no temp file left behind
	tmpPath := path + ".tmp"
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Errorf("temp file should not exist after successful save")
	}

	// Verify the file is valid JSON
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read saved config: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Errorf("saved file is not valid JSON: %v", err)
	}
}

func TestToMap(t *testing.T) {
	cfg := &Config{
		DataDir:  "/tmp/test",
		LogLevel: "debug",
	}
	cfg.LLM.Provider = "openai"
	cfg.LLM.Model = "gpt-4"
	cfg.LLM.MaxTokens = 2000

	m, err := ToMap(cfg)
	if err != nil {
		t.Fatalf("ToMap failed: %v", err)
	}

	if m["data_dir"] != "/tmp/test" {
		t.Errorf("expected data_dir=/tmp/test, got %v", m["data_dir"])
	}
	if m["log_level"] != "debug" {
		t.Errorf("expected log_level=debug, got %v", m["log_level"])
	}

	llm, ok := m["llm"].(map[string]any)
	if !ok {
		t.Fatalf("expected llm to be map, got %T", m["llm"])
	}
	if llm["provider"] != "openai" {
		t.Errorf("expected llm.provider=openai, got %v", llm["provider"])
	}
	if llm["model"] != "gpt-4" {
		t.Errorf("expected llm.model=gpt-4, got %v", llm["model"])
	}
	// JSON numbers are float64
	if llm["max_tokens"] != float64(2000) {
		t.Errorf("expected llm.max_tokens=2000, got %v", llm["max_tokens"])
	}
}

func TestListValues_NoMask(t *testing.T) {
	cfg := &Config{
		LogLevel: "info",
	}
	cfg.LLM.APIKey = "sk-secret-key-1234"
	cfg.Brave.APIKey = "brave-key-5678"
	cfg.Telegram.Token = "bot-token-abcd"

	flat, err := ListValues(cfg, false)
	if err != nil {
		t.Fatalf("ListValues failed: %v", err)
	}

	// Secrets should be unmasked
	if flat["llm.api_key"] != "sk-secret-key-1234" {
		t.Errorf("expected unmasked llm.api_key, got %v", flat["llm.api_key"])
	}
	if flat["brave.api_key"] != "brave-key-5678" {
		t.Errorf("expected unmasked brave.api_key, got %v", flat["brave.api_key"])
	}
	if flat["telegram.token"] != "bot-token-abcd" {
		t.Errorf("expected unmasked telegram.token, got %v", flat["telegram.token"])
	}
	if flat["log_level"] != "info" {
		t.Errorf("expected log_level=info, got %v", flat["log_level"])
	}
}

func TestListValues_WithMask(t *testing.T) {
	cfg := &Config{
		LogLevel: "info",
	}
	cfg.LLM.APIKey = "sk-secret-key-1234"
	cfg.Brave.APIKey = "brave-key-5678"
	cfg.Telegram.Token = "bot-token-abcd"

	flat, err := ListValues(cfg, true)
	if err != nil {
		t.Fatalf("ListValues failed: %v", err)
	}

	// Secrets should be masked
	if flat["llm.api_key"] != "***1234" {
		t.Errorf("expected masked llm.api_key=***1234, got %v", flat["llm.api_key"])
	}
	if flat["brave.api_key"] != "***5678" {
		t.Errorf("expected masked brave.api_key=***5678, got %v", flat["brave.api_key"])
	}
	if flat["telegram.token"] != "***abcd" {
		t.Errorf("expected masked telegram.token=***abcd, got %v", flat["telegram.token"])
	}

	// Non-secrets should be unchanged
	if flat["log_level"] != "info" {
		t.Errorf("expected log_level=info, got %v", flat["log_level"])
	}
}

func TestGetValue_ExistingKey(t *testing.T) {
	path := tempConfigPath(t)

	cfg := &Config{
		LogLevel:      "debug",
		MaxConcurrent: 8,
	}
	cfg.LLM.Provider = "openai"
	cfg.LLM.Model = "gpt-4"
	writeTestConfig(t, path, cfg)

	v, err := GetValue(path, "log_level")
	if err != nil {
		t.Fatalf("GetValue failed: %v", err)
	}
	if v != "debug" {
		t.Errorf("expected log_level=debug, got %v", v)
	}

	v, err = GetValue(path, "llm.model")
	if err != nil {
		t.Fatalf("GetValue failed: %v", err)
	}
	if v != "gpt-4" {
		t.Errorf("expected llm.model=gpt-4, got %v", v)
	}

	v, err = GetValue(path, "max_concurrent")
	if err != nil {
		t.Fatalf("GetValue failed: %v", err)
	}
	// JSON numbers are float64
	if v != float64(8) {
		t.Errorf("expected max_concurrent=8, got %v (%T)", v, v)
	}
}

func TestGetValue_UnknownKey(t *testing.T) {
	path := tempConfigPath(t)

	cfg := &Config{LogLevel: "info"}
	writeTestConfig(t, path, cfg)

	_, err := GetValue(path, "nonexistent.key")
	if err == nil {
		t.Fatal("expected error for unknown key, got nil")
	}
	expected := "unknown config key: nonexistent.key"
	if err.Error() != expected {
		t.Errorf("expected error %q, got %q", expected, err.Error())
	}
}

func TestSetValue_String(t *testing.T) {
	path := tempConfigPath(t)

	cfg := &Config{LogLevel: "info"}
	cfg.LLM.Provider = "openai"
	writeTestConfig(t, path, cfg)

	// Set a string value
	if err := SetValue(path, "log_level", "debug"); err != nil {
		t.Fatalf("SetValue failed: %v", err)
	}

	// Verify it was set
	v, err := GetValue(path, "log_level")
	if err != nil {
		t.Fatalf("GetValue failed: %v", err)
	}
	if v != "debug" {
		t.Errorf("expected log_level=debug after set, got %v", v)
	}

	// Verify other values are preserved
	v, err = GetValue(path, "llm.provider")
	if err != nil {
		t.Fatalf("GetValue failed: %v", err)
	}
	if v != "openai" {
		t.Errorf("expected llm.provider=openai (preserved), got %v", v)
	}
}

func TestSetValue_Numeric(t *testing.T) {
	path := tempConfigPath(t)

	cfg := &Config{MaxConcurrent: 2}
	writeTestConfig(t, path, cfg)

	// Set a numeric value (JSON parseable)
	if err := SetValue(path, "max_concurrent", "16"); err != nil {
		t.Fatalf("SetValue failed: %v", err)
	}

	v, err := GetValue(path, "max_concurrent")
	if err != nil {
		t.Fatalf("GetValue failed: %v", err)
	}
	if v != float64(16) {
		t.Errorf("expected max_concurrent=16, got %v (%T)", v, v)
	}
}

func TestSetValue_Boolean(t *testing.T) {
	path := tempConfigPath(t)

	cfg := &Config{LogLevel: "info"}
	writeTestConfig(t, path, cfg)

	// Set a boolean value (JSON parseable)
	if err := SetValue(path, "some_flag", "true"); err != nil {
		t.Fatalf("SetValue failed: %v", err)
	}

	v, err := GetValue(path, "some_flag")
	if err != nil {
		t.Fatalf("GetValue failed: %v", err)
	}
	if v != true {
		t.Errorf("expected some_flag=true, got %v (%T)", v, v)
	}
}

func TestSetValue_Float(t *testing.T) {
	path := tempConfigPath(t)

	cfg := &Config{}
	cfg.LLM.Temperature = 0.7
	writeTestConfig(t, path, cfg)

	if err := SetValue(path, "llm.temperature", "0.3"); err != nil {
		t.Fatalf("SetValue failed: %v", err)
	}

	v, err := GetValue(path, "llm.temperature")
	if err != nil {
		t.Fatalf("GetValue failed: %v", err)
	}
	if v != 0.3 {
		t.Errorf("expected llm.temperature=0.3, got %v (%T)", v, v)
	}
}

func TestSetValue_NestedKey(t *testing.T) {
	path := tempConfigPath(t)

	cfg := &Config{}
	cfg.LLM.Model = "gpt-3.5-turbo"
	writeTestConfig(t, path, cfg)

	if err := SetValue(path, "llm.model", "gpt-4"); err != nil {
		t.Fatalf("SetValue failed: %v", err)
	}

	v, err := GetValue(path, "llm.model")
	if err != nil {
		t.Fatalf("GetValue failed: %v", err)
	}
	if v != "gpt-4" {
		t.Errorf("expected llm.model=gpt-4, got %v", v)
	}
}

func TestSetValue_NewNestedKey(t *testing.T) {
	path := tempConfigPath(t)

	cfg := &Config{LogLevel: "info"}
	writeTestConfig(t, path, cfg)

	// Set a new nested key that doesn't exist in Config struct
	if err := SetValue(path, "custom.setting", "value"); err != nil {
		t.Fatalf("SetValue failed: %v", err)
	}

	v, err := GetValue(path, "custom.setting")
	if err != nil {
		t.Fatalf("GetValue failed: %v", err)
	}
	if v != "value" {
		t.Errorf("expected custom.setting=value, got %v", v)
	}
}

func TestSetValue_NonexistentFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist", "config.json")
	err := SetValue(path, "log_level", "debug")
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}
}

func TestGetValue_NonexistentFile(t *testing.T) {
	// GetValue calls Load, which creates the file if it doesn't exist.
	// But if the directory doesn't exist, it should still work because
	// Load creates it. Let's test with a valid temp dir.
	path := tempConfigPath(t)

	// File doesn't exist yet; Load will create it with defaults
	v, err := GetValue(path, "log_level")
	if err != nil {
		t.Fatalf("GetValue on new config failed: %v", err)
	}
	// Default log_level is "info"
	if v != "info" {
		t.Errorf("expected default log_level=info, got %v", v)
	}
}

func TestSave_CreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "config.json")

	cfg := &Config{LogLevel: "warn"}
	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save should create parent directory, got: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Errorf("config file should exist: %v", err)
	}
}
