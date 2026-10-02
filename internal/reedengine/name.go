// name.go mints the durable 128-bit identity a strand is keyed on.
// The strand's display name is formed separately, by strandNameLocked in strand.go.

package reedengine

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// newGUID returns a 128-bit random hex identifier.
func newGUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("rand read: %w", err)
	}
	return hex.EncodeToString(b), nil
}
