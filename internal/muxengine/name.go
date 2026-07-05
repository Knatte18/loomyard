// name.go implements the strand-identity/display helpers: newGUID mints the
// durable 128-bit identity a strand is keyed on, and FormatStrandName does
// the pure token substitution that turns mux.yaml's strand_name template
// into a caller-facing display name at add-time. Neither function persists
// or reads anything — the substitution result is a plain string the caller
// writes into Strand.Name once, at AddStrand.

package muxengine

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
)

// newGUID returns a 128-bit random identifier, hex-encoded, generated from
// crypto/rand. This is the durable key mux generates once at AddStrand:
// parent links, and every selector (--parent, remove), key on this value —
// never on the caller-supplied, non-unique display name.
func newGUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("rand read: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// FormatStrandName substitutes the tokens <WORKTREE>, <ROLE>, <ROUND>, and
// <SHORT_GUID> in template with the corresponding values from parts. A
// token with no entry in parts (or an entry that is the empty string)
// substitutes to "". FormatStrandName is pure — it performs no I/O and
// generates nothing itself; callers that want <SHORT_GUID> filled pass it
// in via parts (typically the first 8 hex characters of a newGUID() value).
// The caller convention, not enforced here, is that when neither a name nor
// a role is supplied the caller falls back to using <SHORT_GUID> alone as
// the strand name.
func FormatStrandName(template string, parts map[string]string) string {
	result := template
	for _, token := range []string{"<WORKTREE>", "<ROLE>", "<ROUND>", "<SHORT_GUID>"} {
		result = strings.ReplaceAll(result, token, parts[token])
	}
	return result
}
