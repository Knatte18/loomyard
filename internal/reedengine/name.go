// name.go implements the strand-identity/display helpers: newGUID mints the durable 128-bit
// identity a strand is keyed on,
// and FormatStrandName does the pure token substitution that turns reed.yaml's strand_name template
// into a caller-facing display name at add-time.
// Neither function persists or reads anything — the substitution result is a plain string the
// caller writes into Strand.Name once, at AddStrand.

package reedengine

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
)

// newGUID returns a 128-bit random hex identifier.
func newGUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("rand read: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// FormatStrandName substitutes <WORKTREE>, <ROLE>, <ROUND>, <SHORT_GUID> tokens in template with
// values from parts.
// Pure;
// does no I/O.
func FormatStrandName(template string, parts map[string]string) string {
	result := template
	for _, token := range []string{"<WORKTREE>", "<ROLE>", "<ROUND>", "<SHORT_GUID>"} {
		result = strings.ReplaceAll(result, token, parts[token])
	}
	return result
}
