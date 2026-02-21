// Package state provides filesystem-backed storage implementations.
package state

import "github.com/user/gopherclaw/internal/types"

// Compile-time interface compliance checks.
var _ types.SessionStore = (*SessionStore)(nil)
var _ types.EventStore = (*EventStore)(nil)
var _ types.ArtifactStore = (*ArtifactStore)(nil)
