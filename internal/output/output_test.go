package output_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/Knatte18/loomyard/internal/output"
)

func decodeEnvelope(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("output is not valid JSON: %v; output: %q", err, buf.String())
	}
	return result
}

// TestOk_EmitsValidJSON asserts that Ok returns exit code 0, emits a JSON line with ok=true and the supplied fields, and adds ok to the caller's map.
func TestOk_EmitsValidJSON(t *testing.T) {
	t.Parallel()
	buf := &bytes.Buffer{}
	fields := map[string]any{"count": 42, "message": "success"}

	exitCode := output.Ok(buf, fields)

	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}

	result := decodeEnvelope(t, buf)
	if ok, exists := result["ok"]; !exists || ok != true {
		t.Errorf("expected ok=true in output, got: %v", result)
	}
	if count, exists := result["count"]; !exists || count != float64(42) {
		t.Errorf("expected count=42 in output, got: %v", result)
	}
	if msg, exists := result["message"]; !exists || msg != "success" {
		t.Errorf("expected message='success' in output, got: %v", result)
	}
	if ok, exists := fields["ok"]; !exists || ok != true {
		t.Errorf("expected Ok to mutate fields map by adding ok=true, got: %v", fields)
	}
}

// TestErr_EmitsErrorEnvelope asserts that Err returns exit code 1 and emits ok=false with the message in the error field.
// The message is stripped of surrounding whitespace, so embedded tool output such as "fatal: not a git repository\n" does not leak newline characters into the JSON value.
//
//testtiming:keep pins ok=false, exit code 1 and the trimmed error field, which TestErrFields_NilFieldsMatchesErr only compares byte-for-byte
func TestErr_EmitsErrorEnvelope(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		message string
		want    string
	}{
		{name: "plain message", message: "something went wrong", want: "something went wrong"},
		{name: "trailing newline trimmed", message: "fatal: not a git repository\n", want: "fatal: not a git repository"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			buf := &bytes.Buffer{}

			exitCode := output.Err(buf, tt.message)

			if exitCode != 1 {
				t.Errorf("Err(%q) = %d; want 1", tt.message, exitCode)
			}
			result := decodeEnvelope(t, buf)
			if ok, exists := result["ok"]; !exists || ok != false {
				t.Errorf("Err(%q) ok = %v; want false", tt.message, result["ok"])
			}
			if errField, _ := result["error"].(string); errField != tt.want {
				t.Errorf("Err(%q) error field = %q; want %q", tt.message, errField, tt.want)
			}
		})
	}
}

// TestErrFields_EmitsEnvelopeWithFields asserts that ErrFields returns exit code 1, emits ok=false and the trimmed message, keeps the supplied fields, and lets the injected "ok" and "error" override a caller-supplied pair.
//
//testtiming:keep pins the supplied fields kept and the reserved keys overridden, which TestErr_EmitsErrorEnvelope does not assert
func TestErrFields_EmitsEnvelopeWithFields(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		message   string
		fields    map[string]any
		wantError string
		wantExtra map[string]any
	}{
		{
			name:      "supplied fields kept and message trimmed",
			message:   "  something failed\n",
			fields:    map[string]any{"mutations": []string{"a", "b"}, "partial": true},
			wantError: "something failed",
			wantExtra: map[string]any{"partial": true, "mutations": []any{"a", "b"}},
		},
		{
			name:      "reserved keys override caller-supplied",
			message:   "the real error",
			fields:    map[string]any{"ok": true, "error": "caller-supplied"},
			wantError: "the real error",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			buf := &bytes.Buffer{}

			exitCode := output.ErrFields(buf, tt.message, tt.fields)

			if exitCode != 1 {
				t.Errorf("ErrFields(...) = %d; want 1", exitCode)
			}
			result := decodeEnvelope(t, buf)
			if ok, exists := result["ok"]; !exists || ok != false {
				t.Errorf("ErrFields(...) ok = %v; want false", result["ok"])
			}
			if errField, _ := result["error"].(string); errField != tt.wantError {
				t.Errorf("ErrFields(...) error = %q; want %q", errField, tt.wantError)
			}
			for key, want := range tt.wantExtra {
				got, exists := result[key]
				if !exists {
					t.Errorf("ErrFields(...) missing supplied field %q, got: %v", key, result)
					continue
				}
				gotJSON, _ := json.Marshal(got)
				wantJSON, _ := json.Marshal(want)
				if string(gotJSON) != string(wantJSON) {
					t.Errorf("ErrFields(...) field %q = %s; want %s", key, gotJSON, wantJSON)
				}
			}
		})
	}
}

// TestErrFields_NilFieldsMatchesErr asserts that ErrFields with a nil map emits byte-identical output to Err for the same message.
//
//testtiming:keep pins byte-identical output of ErrFields(nil) and Err, which TestErr_EmitsErrorEnvelope and TestErrFields_EmitsEnvelopeWithFields do not compare
func TestErrFields_NilFieldsMatchesErr(t *testing.T) {
	t.Parallel()
	msg := "something went wrong"

	errBuf := &bytes.Buffer{}
	output.Err(errBuf, msg)

	errFieldsBuf := &bytes.Buffer{}
	output.ErrFields(errFieldsBuf, msg, nil)

	if errBuf.String() != errFieldsBuf.String() {
		t.Errorf("ErrFields(w, msg, nil) = %q; want byte-identical to Err(w, msg) = %q", errFieldsBuf.String(), errBuf.String())
	}
}
