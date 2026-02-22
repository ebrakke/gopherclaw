package config

import (
	"strings"
)

// secretKeys lists the dot-separated keys whose values should be masked.
var secretKeys = map[string]bool{
	"llm.api_key":    true,
	"brave.api_key":  true,
	"telegram.token": true,
}

// IsSecretKey returns true if the given dot-separated key is a secret.
func IsSecretKey(key string) bool {
	return secretKeys[key]
}

// Flatten converts a nested map into a flat map with dot-separated keys.
// For example, {"llm": {"provider": "openai"}} becomes {"llm.provider": "openai"}.
func Flatten(m map[string]any) map[string]any {
	out := make(map[string]any)
	flatten("", m, out)
	return out
}

func flatten(prefix string, m map[string]any, out map[string]any) {
	for k, v := range m {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		switch child := v.(type) {
		case map[string]any:
			flatten(key, child, out)
		default:
			out[key] = v
		}
	}
}

// Unflatten converts a flat map with dot-separated keys back into a nested map.
// For example, {"llm.provider": "openai"} becomes {"llm": {"provider": "openai"}}.
func Unflatten(flat map[string]any) map[string]any {
	out := make(map[string]any)
	for k, v := range flat {
		parts := strings.Split(k, ".")
		current := out
		for i, part := range parts {
			if i == len(parts)-1 {
				current[part] = v
			} else {
				next, ok := current[part]
				if !ok {
					next = make(map[string]any)
					current[part] = next
				}
				m, ok := next.(map[string]any)
				if !ok {
					m = make(map[string]any)
					current[part] = m
				}
				current = m
			}
		}
	}
	return out
}

// MaskSecrets returns a copy of the flat map with secret values masked.
// Secret keys (llm.api_key, brave.api_key, telegram.token) are shown as
// "***xxxx" where xxxx is the last 4 characters of the value. Empty
// values are left empty.
func MaskSecrets(flat map[string]any) map[string]any {
	out := make(map[string]any, len(flat))
	for k, v := range flat {
		if secretKeys[k] {
			s, ok := v.(string)
			if ok && s != "" {
				if len(s) <= 4 {
					out[k] = "***" + s
				} else {
					out[k] = "***" + s[len(s)-4:]
				}
			} else {
				out[k] = v
			}
		} else {
			out[k] = v
		}
	}
	return out
}
