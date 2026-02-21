// internal/state/artifact.go
package state

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/user/gopherclaw/internal/types"
)

// artifactWrapper is the on-disk format for artifact files.
// Each artifact is stored as {"meta": ..., "data": ...}.
type artifactWrapper struct {
	Meta *types.ArtifactMeta `json:"meta"`
	Data json.RawMessage     `json:"data"`
}

// ArtifactStore stores artifacts as individual JSON files per artifact.
// Files are located at sessions/<sessionID>/artifacts/<artifactID>.json.
type ArtifactStore struct {
	root string
}

// NewArtifactStore creates a new file-backed ArtifactStore rooted at the given directory.
func NewArtifactStore(root string) *ArtifactStore {
	return &ArtifactStore{root: root}
}

func (a *ArtifactStore) artifactsDir(sessionID types.SessionID) string {
	return filepath.Join(a.root, "sessions", string(sessionID), "artifacts")
}

func (a *ArtifactStore) artifactPath(sessionID types.SessionID, artifactID types.ArtifactID) string {
	return filepath.Join(a.artifactsDir(sessionID), string(artifactID)+".json")
}

// findArtifact locates an artifact file by ID using filepath.Glob across all sessions.
func (a *ArtifactStore) findArtifact(id types.ArtifactID) (string, error) {
	pattern := filepath.Join(a.root, "sessions", "*", "artifacts", string(id)+".json")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return "", fmt.Errorf("glob artifact: %w", err)
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("artifact not found: %s", id)
	}
	return matches[0], nil
}

// readWrapper reads and parses an artifact file.
func (a *ArtifactStore) readWrapper(path string) (*artifactWrapper, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read artifact file: %w", err)
	}

	var wrapper artifactWrapper
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, fmt.Errorf("unmarshal artifact: %w", err)
	}
	return &wrapper, nil
}

// Put stores an artifact and returns its ID.
func (a *ArtifactStore) Put(_ context.Context, sessionID types.SessionID, runID types.RunID, tool string, data any) (types.ArtifactID, error) {
	id := types.NewArtifactID()

	meta := &types.ArtifactMeta{
		ID:        id,
		SessionID: sessionID,
		RunID:     runID,
		Tool:      tool,
		CreatedAt: time.Now(),
	}

	// Marshal the data to json.RawMessage
	rawData, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("marshal artifact data: %w", err)
	}

	wrapper := &artifactWrapper{
		Meta: meta,
		Data: json.RawMessage(rawData),
	}

	content, err := json.MarshalIndent(wrapper, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal artifact wrapper: %w", err)
	}

	// Ensure directory exists
	dir := a.artifactsDir(sessionID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create artifacts dir: %w", err)
	}

	// Atomic write via temp file + rename
	target := a.artifactPath(sessionID, id)
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, content, 0o644); err != nil {
		return "", fmt.Errorf("write temp artifact: %w", err)
	}
	if err := os.Rename(tmp, target); err != nil {
		os.Remove(tmp)
		return "", fmt.Errorf("rename temp artifact: %w", err)
	}

	return id, nil
}

// Get returns the raw data for the given artifact.
func (a *ArtifactStore) Get(_ context.Context, id types.ArtifactID) (json.RawMessage, error) {
	path, err := a.findArtifact(id)
	if err != nil {
		return nil, err
	}

	wrapper, err := a.readWrapper(path)
	if err != nil {
		return nil, err
	}

	return wrapper.Data, nil
}

// GetMeta returns the metadata for the given artifact.
func (a *ArtifactStore) GetMeta(_ context.Context, id types.ArtifactID) (*types.ArtifactMeta, error) {
	path, err := a.findArtifact(id)
	if err != nil {
		return nil, err
	}

	wrapper, err := a.readWrapper(path)
	if err != nil {
		return nil, err
	}

	return wrapper.Meta, nil
}

// Excerpt returns a truncated text representation of the artifact data,
// optionally highlighting around a query substring.
func (a *ArtifactStore) Excerpt(_ context.Context, id types.ArtifactID, query string, maxTokens int) (string, error) {
	path, err := a.findArtifact(id)
	if err != nil {
		return "", err
	}

	wrapper, err := a.readWrapper(path)
	if err != nil {
		return "", err
	}

	raw := string(wrapper.Data)

	// Approximate max characters from token count (roughly 4 chars per token)
	maxChars := maxTokens * 4
	if maxChars <= 0 {
		maxChars = len(raw)
	}

	// If there's a query, try to center the excerpt around the query
	if query != "" {
		idx := strings.Index(strings.ToLower(raw), strings.ToLower(query))
		if idx >= 0 {
			start := idx - maxChars/2
			if start < 0 {
				start = 0
			}
			end := start + maxChars
			if end > len(raw) {
				end = len(raw)
			}
			return raw[start:end], nil
		}
	}

	// No query or query not found: return from the beginning
	if len(raw) > maxChars {
		return raw[:maxChars], nil
	}
	return raw, nil
}
