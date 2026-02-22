package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

// Bash executes shell commands on the host.
type Bash struct{}

// NewBash creates a new Bash tool.
func NewBash() *Bash { return &Bash{} }

func (b *Bash) Name() string        { return "bash" }
func (b *Bash) Description() string { return "Execute a bash command on the host machine" }
func (b *Bash) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"command": {"type": "string", "description": "The command to execute"},
			"timeout_seconds": {"type": "integer", "description": "Timeout in seconds (default: 120)"}
		},
		"required": ["command"]
	}`)
}

func (b *Bash) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Command        string `json:"command"`
		TimeoutSeconds int    `json:"timeout_seconds"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("parse args: %w", err)
	}
	if params.Command == "" {
		return "", fmt.Errorf("command is required")
	}

	timeout := 120 * time.Second
	if params.TimeoutSeconds > 0 {
		timeout = time.Duration(params.TimeoutSeconds) * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", params.Command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("command failed: %w\nOutput: %s", err, string(output))
	}
	return string(output), nil
}
