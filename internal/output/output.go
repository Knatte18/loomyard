// output.go holds the JSON envelope helpers for CLI output.
//
// Ok and Err write JSON responses in one envelope: an ok flag, plus the caller's fields or an error message.
// ErrorEnvelopeLine finds an error envelope inside text that should hold none.

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

// ErrorEnvelopeLine returns the first line of text that is a whole error envelope:
// a JSON object whose "ok" is false and whose "error" is a string.
// A line inside a closed fenced code block is never matched, so a text that quotes an envelope in a block passes.
// A fence closes on a line of the opener's character at least as long as the opener and nothing else.
// A fence left open to the end of the text hides nothing, since an unclosed fence is likelier a typo than a quotation.
// It reports false when no line is one.
// A failed verb's standard output, captured into a file or a variable, leaves one where the expected text belongs.
func ErrorEnvelopeLine(text string) (string, bool) {
	var lines []string
	for line := range strings.Lines(text) {
		lines = append(lines, strings.TrimSpace(line))
	}

	fenceChar, fenceLength, openedAt := byte(0), 0, 0
	for index, trimmed := range lines {
		if fenceChar == 0 {
			if char, length := fenceRun(trimmed); length >= 3 {
				fenceChar, fenceLength, openedAt = char, length, index
				continue
			}
			if envelope, ok := envelopeOf(trimmed); ok {
				return envelope, true
			}
			continue
		}
		if char, length := fenceRun(trimmed); char == fenceChar && length >= fenceLength && strings.Trim(trimmed, string(fenceChar)) == "" {
			fenceChar = 0
		}
	}
	if fenceChar != 0 {
		for _, trimmed := range lines[openedAt+1:] {
			if envelope, ok := envelopeOf(trimmed); ok {
				return envelope, true
			}
		}
	}
	return "", false
}

// fenceRun returns the fence character that line opens with and the length of its run.
// The length is zero when the line opens with neither a backtick nor a tilde.
func fenceRun(line string) (char byte, length int) {
	if line == "" || (line[0] != '`' && line[0] != '~') {
		return 0, 0
	}
	char = line[0]
	for length < len(line) && line[length] == char {
		length++
	}
	return char, length
}

// envelopeOf reports whether line is a whole error envelope, and returns it.
func envelopeOf(line string) (string, bool) {
	if !strings.HasPrefix(line, "{") {
		return "", false
	}
	var envelope struct {
		OK    *bool   `json:"ok"`
		Error *string `json:"error"`
	}
	if json.Unmarshal([]byte(line), &envelope) != nil {
		return "", false
	}
	if envelope.OK != nil && !*envelope.OK && envelope.Error != nil {
		return line, true
	}
	return "", false
}
