// output.go — JSON envelope helpers for CLI output.
//
// Provides Ok and Err functions for emitting structured JSON responses with consistent envelope
// shape (ok flag and optional fields/error message).

package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Ok writes a JSON response with ok=true plus the supplied fields,
// and returns exit code 0. It mutates the supplied fields map in place by injecting "ok": true.
func Ok(w io.Writer, fields map[string]any) int {
	fields["ok"] = true
	data, _ := json.Marshal(fields)
	fmt.Fprintln(w, string(data))
	return 0
}

// Err writes a JSON response with ok=false and the given error message,
// and returns exit code 1. The message is trimmed of leading and trailing whitespace to prevent
// embedded tool output from leaking formatting.
func Err(w io.Writer, msg string) int {
	return ErrFields(w, msg, nil)
}

// ErrFields writes a JSON response with ok=false, the given error message, and the caller's fields,
// and returns exit code 1. It mirrors Ok's shape: the caller's fields are laid down first, then
// "ok": false and "error": strings.TrimSpace(msg) are injected after, so those two reserved keys
// always win over a caller-supplied key of the same name — matching Ok's existing in-place
// fields["ok"] = true mutation rather than inventing a second collision policy.
// A nil fields map is accepted and behaves exactly like Err(w, msg).
func ErrFields(w io.Writer, msg string, fields map[string]any) int {
	if fields == nil {
		fields = map[string]any{}
	}
	fields["ok"] = false
	fields["error"] = strings.TrimSpace(msg)
	data, _ := json.Marshal(fields)
	fmt.Fprintln(w, string(data))
	return 1
}
