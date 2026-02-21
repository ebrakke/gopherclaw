// internal/state/artifact_test.go
package state

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/user/gopherclaw/internal/types"
)

func TestArtifactStore(t *testing.T) {
	dir := t.TempDir()
	store := NewArtifactStore(dir)
	ctx := context.Background()

	sessionID := types.NewSessionID()
	runID := types.NewRunID()

	// Test put
	data := map[string]any{
		"output": "test result",
		"lines":  []string{"line1", "line2"},
	}

	artifactID, err := store.Put(ctx, sessionID, runID, "test-tool", data)
	if err != nil {
		t.Fatal(err)
	}
	if artifactID == "" {
		t.Error("expected non-empty artifact ID")
	}

	// Test get
	raw, err := store.Get(ctx, artifactID)
	if err != nil {
		t.Fatal(err)
	}

	var retrieved map[string]any
	if err := json.Unmarshal(raw, &retrieved); err != nil {
		t.Fatal(err)
	}
	if retrieved["output"] != "test result" {
		t.Error("data mismatch")
	}

	// Test get meta
	meta, err := store.GetMeta(ctx, artifactID)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Tool != "test-tool" {
		t.Errorf("expected tool test-tool, got %s", meta.Tool)
	}
}
