// output.go holds the JSON envelope helpers for CLI output.
//
// Ok and Err write JSON responses in one envelope: an ok flag, plus the caller's fields or an error message.

package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Ok writes the supplied fields as a JSON response, and returns exit code 0.
// The fields map is modified in place: its "ok" key is set to true.
func Ok(w io.Writer, fields map[string]any) int {
	fields["ok"] = true
	data, _ := json.Marshal(fields)
	fmt.Fprintln(w, string(data))
	return 0
}

// Err writes a JSON response with ok=false and the given error message, and returns exit code 1.
// The message is trimmed of leading and trailing whitespace, so embedded tool output does not leak its formatting.
func Err(w io.Writer, msg string) int {
	return ErrFields(w, msg, nil)
}

// ErrFields writes a JSON response with ok=false, the caller's fields, and the given error message.
// It returns exit code 1, and it accepts a nil fields map.
// The reserved keys "ok" and "error" always win over a caller key of the same name.
// The message is trimmed of leading and trailing whitespace.
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
