// Package envelope decodes the internal/output JSON envelope a CLI prints.
//
// It imports only the standard library.
// Beyond the RequireOK/RequireErr shape check on ok and error, it asserts nothing about module behaviour.
package envelope

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

// Envelope is one decoded CLI envelope.
// Raw holds every key other than ok, error and partial.
type Envelope struct {
	OK      bool
	Error   string
	Partial *bool
	Raw     map[string]any
}

// Decode parses out as exactly one JSON document, failing the test on malformed JSON or trailing documents.
func Decode(t testing.TB, out string) Envelope {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader([]byte(out)))
	var all map[string]any
	if err := dec.Decode(&all); err != nil {
		t.Fatalf("envelope: malformed JSON %q: %v", out, err)
	}
	var extra json.RawMessage
	if err := dec.Decode(&extra); err != io.EOF {
		t.Fatalf("envelope: more than one top-level document in %q", out)
	}

	env := Envelope{Raw: map[string]any{}}
	for k, v := range all {
		switch k {
		case "ok":
			env.OK, _ = v.(bool)
		case "error":
			env.Error, _ = v.(string)
		case "partial":
			if b, isBool := v.(bool); isBool {
				env.Partial = &b
			} else {
				env.Raw[k] = v
			}
		default:
			env.Raw[k] = v
		}
	}
	return env
}

// RequireOK decodes out and fails the test unless ok is true.
func RequireOK(t testing.TB, out string) Envelope {
	t.Helper()
	env := Decode(t, out)
	if !env.OK {
		t.Fatalf("envelope: ok = false, want true (error %q) in %q", env.Error, out)
	}
	return env
}

// RequireErr decodes out and fails the test unless ok is false and the error contains substr.
func RequireErr(t testing.TB, out, substr string) Envelope {
	t.Helper()
	env := Decode(t, out)
	if env.OK {
		t.Fatalf("envelope: ok = true, want false in %q", out)
	}
	if !strings.Contains(env.Error, substr) {
		t.Fatalf("envelope: error %q does not contain %q", env.Error, substr)
	}
	return env
}
