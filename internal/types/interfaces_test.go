// internal/types/interfaces_test.go
package types

import "testing"

func TestInterfaceCompilation(t *testing.T) {
	var _ SessionStore
	var _ EventStore
	var _ ArtifactStore
}
