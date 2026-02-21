package llm

import "context"

// Provider defines the interface for interacting with LLM backends.
// Implementations handle protocol-specific details such as request formatting,
// authentication, and response parsing.
type Provider interface {
	// Complete sends a chat completion request and returns the full response.
	Complete(ctx context.Context, messages []Message, tools []Tool) (*Response, error)

	// Stream sends a chat completion request and returns a channel of incremental deltas.
	Stream(ctx context.Context, messages []Message, tools []Tool) (<-chan Delta, error)
}

// Config holds common configuration for LLM providers.
type Config struct {
	BaseURL     string
	APIKey      string
	Model       string
	MaxTokens   int
	Temperature float32
}
