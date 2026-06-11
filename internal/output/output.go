// output.go — JSON envelope helpers for CLI output.
//
// Provides Ok and Err functions for emitting structured JSON responses
// with consistent envelope shape (ok flag and optional fields/error message).

package output

import (
	"encoding/json"
	"fmt"
	"io"
)

// Ok writes a JSON response with ok=true plus the supplied fields, and returns exit code 0.
//
// It mutates the supplied fields map in place by injecting "ok": true.
// Callers should therefore pass freshly-built map literals.
// Marshal errors are ignored (carry-over from board's writeJSON).
func Ok(w io.Writer, fields map[string]any) int {
	fields["ok"] = true
	data, _ := json.Marshal(fields)
	fmt.Fprintln(w, string(data))
	return 0
}

// Err writes a JSON response with ok=false and the given error message, and returns exit code 1.
//
// Marshal errors are ignored (carry-over from board's writeJSON).
func Err(w io.Writer, msg string) int {
	data, _ := json.Marshal(map[string]any{"ok": false, "error": msg})
	fmt.Fprintln(w, string(data))
	return 1
}
