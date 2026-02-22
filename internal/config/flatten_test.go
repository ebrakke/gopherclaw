package config

import (
	"testing"
)

func TestFlatten_Simple(t *testing.T) {
	m := map[string]any{
		"a": "hello",
		"b": 42.0,
	}
	got := Flatten(m)
	if got["a"] != "hello" {
		t.Errorf("expected a=hello, got %v", got["a"])
	}
	if got["b"] != 42.0 {
		t.Errorf("expected b=42, got %v", got["b"])
	}
	if len(got) != 2 {
		t.Errorf("expected 2 keys, got %d", len(got))
	}
}

func TestFlatten_Nested(t *testing.T) {
	m := map[string]any{
		"llm": map[string]any{
			"provider": "openai",
			"api_key":  "sk-test123",
		},
		"log_level": "info",
	}
	got := Flatten(m)
	if got["llm.provider"] != "openai" {
		t.Errorf("expected llm.provider=openai, got %v", got["llm.provider"])
	}
	if got["llm.api_key"] != "sk-test123" {
		t.Errorf("expected llm.api_key=sk-test123, got %v", got["llm.api_key"])
	}
	if got["log_level"] != "info" {
		t.Errorf("expected log_level=info, got %v", got["log_level"])
	}
	if len(got) != 3 {
		t.Errorf("expected 3 keys, got %d", len(got))
	}
}

func TestFlatten_DeeplyNested(t *testing.T) {
	m := map[string]any{
		"a": map[string]any{
			"b": map[string]any{
				"c": "deep",
			},
		},
	}
	got := Flatten(m)
	if got["a.b.c"] != "deep" {
		t.Errorf("expected a.b.c=deep, got %v", got["a.b.c"])
	}
	if len(got) != 1 {
		t.Errorf("expected 1 key, got %d", len(got))
	}
}

func TestFlatten_EmptyMap(t *testing.T) {
	got := Flatten(map[string]any{})
	if len(got) != 0 {
		t.Errorf("expected 0 keys, got %d", len(got))
	}
}

func TestFlatten_EmptyNestedMap(t *testing.T) {
	m := map[string]any{
		"a": map[string]any{},
	}
	got := Flatten(m)
	if len(got) != 0 {
		t.Errorf("expected 0 keys (empty nested map produces nothing), got %d", len(got))
	}
}

func TestUnflatten_Simple(t *testing.T) {
	flat := map[string]any{
		"a": "hello",
		"b": 42.0,
	}
	got := Unflatten(flat)
	if got["a"] != "hello" {
		t.Errorf("expected a=hello, got %v", got["a"])
	}
	if got["b"] != 42.0 {
		t.Errorf("expected b=42, got %v", got["b"])
	}
}

func TestUnflatten_Nested(t *testing.T) {
	flat := map[string]any{
		"llm.provider": "openai",
		"llm.api_key":  "sk-test123",
		"log_level":    "info",
	}
	got := Unflatten(flat)
	llm, ok := got["llm"].(map[string]any)
	if !ok {
		t.Fatalf("expected llm to be map, got %T", got["llm"])
	}
	if llm["provider"] != "openai" {
		t.Errorf("expected llm.provider=openai, got %v", llm["provider"])
	}
	if llm["api_key"] != "sk-test123" {
		t.Errorf("expected llm.api_key=sk-test123, got %v", llm["api_key"])
	}
	if got["log_level"] != "info" {
		t.Errorf("expected log_level=info, got %v", got["log_level"])
	}
}

func TestUnflatten_DeeplyNested(t *testing.T) {
	flat := map[string]any{
		"a.b.c": "deep",
	}
	got := Unflatten(flat)
	a, ok := got["a"].(map[string]any)
	if !ok {
		t.Fatalf("expected a to be map, got %T", got["a"])
	}
	b, ok := a["b"].(map[string]any)
	if !ok {
		t.Fatalf("expected a.b to be map, got %T", a["b"])
	}
	if b["c"] != "deep" {
		t.Errorf("expected a.b.c=deep, got %v", b["c"])
	}
}

func TestUnflatten_EmptyMap(t *testing.T) {
	got := Unflatten(map[string]any{})
	if len(got) != 0 {
		t.Errorf("expected 0 keys, got %d", len(got))
	}
}

func TestRoundTrip_FlattenUnflatten(t *testing.T) {
	original := map[string]any{
		"data_dir":  "/home/test/.gopherclaw",
		"log_level": "debug",
		"llm": map[string]any{
			"provider": "openai",
			"api_key":  "sk-test123456",
			"model":    "gpt-4",
		},
		"brave": map[string]any{
			"api_key": "brave-key-xyz",
		},
		"telegram": map[string]any{
			"token": "bot-token-abc",
		},
	}

	flat := Flatten(original)
	restored := Unflatten(flat)

	// Check top-level values
	if restored["data_dir"] != original["data_dir"] {
		t.Errorf("data_dir mismatch: %v != %v", restored["data_dir"], original["data_dir"])
	}
	if restored["log_level"] != original["log_level"] {
		t.Errorf("log_level mismatch: %v != %v", restored["log_level"], original["log_level"])
	}

	// Check nested values
	llm := restored["llm"].(map[string]any)
	origLLM := original["llm"].(map[string]any)
	if llm["provider"] != origLLM["provider"] {
		t.Errorf("llm.provider mismatch: %v != %v", llm["provider"], origLLM["provider"])
	}
	if llm["api_key"] != origLLM["api_key"] {
		t.Errorf("llm.api_key mismatch: %v != %v", llm["api_key"], origLLM["api_key"])
	}
	if llm["model"] != origLLM["model"] {
		t.Errorf("llm.model mismatch: %v != %v", llm["model"], origLLM["model"])
	}

	brave := restored["brave"].(map[string]any)
	origBrave := original["brave"].(map[string]any)
	if brave["api_key"] != origBrave["api_key"] {
		t.Errorf("brave.api_key mismatch: %v != %v", brave["api_key"], origBrave["api_key"])
	}

	tg := restored["telegram"].(map[string]any)
	origTg := original["telegram"].(map[string]any)
	if tg["token"] != origTg["token"] {
		t.Errorf("telegram.token mismatch: %v != %v", tg["token"], origTg["token"])
	}
}

func TestMaskSecrets_AllSecrets(t *testing.T) {
	flat := map[string]any{
		"llm.provider":   "openai",
		"llm.api_key":    "sk-test123456",
		"brave.api_key":  "BSA-abcdef1234",
		"telegram.token":  "123456:ABCdefGHIjkl",
		"log_level":      "info",
	}
	got := MaskSecrets(flat)

	// Non-secret should be unchanged
	if got["llm.provider"] != "openai" {
		t.Errorf("expected llm.provider=openai, got %v", got["llm.provider"])
	}
	if got["log_level"] != "info" {
		t.Errorf("expected log_level=info, got %v", got["log_level"])
	}

	// Secrets should be masked with last 4 chars
	if got["llm.api_key"] != "***3456" {
		t.Errorf("expected llm.api_key=***3456, got %v", got["llm.api_key"])
	}
	if got["brave.api_key"] != "***1234" {
		t.Errorf("expected brave.api_key=***1234, got %v", got["brave.api_key"])
	}
	if got["telegram.token"] != "***Ijkl" {
		t.Errorf("expected telegram.token=***Ijkl, got %v", got["telegram.token"])
	}
}

func TestMaskSecrets_EmptySecret(t *testing.T) {
	flat := map[string]any{
		"llm.api_key": "",
	}
	got := MaskSecrets(flat)
	if got["llm.api_key"] != "" {
		t.Errorf("expected empty string to remain empty, got %v", got["llm.api_key"])
	}
}

func TestMaskSecrets_ShortSecret(t *testing.T) {
	flat := map[string]any{
		"llm.api_key": "ab",
	}
	got := MaskSecrets(flat)
	if got["llm.api_key"] != "***ab" {
		t.Errorf("expected ***ab for short secret, got %v", got["llm.api_key"])
	}
}

func TestMaskSecrets_ExactlyFourChars(t *testing.T) {
	flat := map[string]any{
		"llm.api_key": "abcd",
	}
	got := MaskSecrets(flat)
	if got["llm.api_key"] != "***abcd" {
		t.Errorf("expected ***abcd for 4-char secret, got %v", got["llm.api_key"])
	}
}

func TestMaskSecrets_NoSecretKeys(t *testing.T) {
	flat := map[string]any{
		"log_level":    "debug",
		"data_dir":     "/tmp",
		"llm.provider": "openai",
	}
	got := MaskSecrets(flat)
	if got["log_level"] != "debug" {
		t.Errorf("expected log_level=debug, got %v", got["log_level"])
	}
	if got["data_dir"] != "/tmp" {
		t.Errorf("expected data_dir=/tmp, got %v", got["data_dir"])
	}
	if got["llm.provider"] != "openai" {
		t.Errorf("expected llm.provider=openai, got %v", got["llm.provider"])
	}
}

func TestFlatten_MixedTypes(t *testing.T) {
	m := map[string]any{
		"str":   "hello",
		"num":   42.0,
		"bool":  true,
		"float": 3.14,
		"nested": map[string]any{
			"val": "inside",
		},
	}
	got := Flatten(m)
	if got["str"] != "hello" {
		t.Errorf("expected str=hello, got %v", got["str"])
	}
	if got["num"] != 42.0 {
		t.Errorf("expected num=42, got %v", got["num"])
	}
	if got["bool"] != true {
		t.Errorf("expected bool=true, got %v", got["bool"])
	}
	if got["float"] != 3.14 {
		t.Errorf("expected float=3.14, got %v", got["float"])
	}
	if got["nested.val"] != "inside" {
		t.Errorf("expected nested.val=inside, got %v", got["nested.val"])
	}
}
