// internal/delivery/registry.go
package delivery

import (
	"fmt"
	"strings"
	"sync"
)

// Handler delivers a message to a session identified by sessionKey.
type Handler func(sessionKey, message string) error

// Registry routes messages to the appropriate delivery handler based on
// session key prefix (e.g. "telegram:", "slack:").
type Registry struct {
	mu       sync.RWMutex
	handlers map[string]Handler
}

// NewRegistry creates an empty delivery registry.
func NewRegistry() *Registry {
	return &Registry{
		handlers: make(map[string]Handler),
	}
}

// Register adds a handler for session keys starting with prefix.
func (r *Registry) Register(prefix string, handler Handler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[prefix] = handler
}

// Deliver finds the handler matching the session key prefix and calls it.
// Returns an error if no handler is registered for the prefix.
func (r *Registry) Deliver(sessionKey, message string) error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for prefix, handler := range r.handlers {
		if strings.HasPrefix(sessionKey, prefix) {
			return handler(sessionKey, message)
		}
	}
	return fmt.Errorf("no delivery handler for session key: %s", sessionKey)
}
