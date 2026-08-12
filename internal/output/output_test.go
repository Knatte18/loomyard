package output_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/Knatte18/loomyard/internal/output"
)

// TestOk_EmitsValidJSON tests that Ok emits a valid JSON line that parses correctly
func TestOk_EmitsValidJSON(t *testing.T) {
	buf := &bytes.Buffer{}
	fields := map[string]any{"count": 42, "message": "success"}

	exitCode := output.Ok(buf, fields)

	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}

	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v", err)
	}

	// Check that ok is true
	if ok, exists := result["ok"]; !exists || ok != true {
		t.Errorf("expected ok=true in output, got: %v", result)
	}

	// Check that supplied fields are present
	if count, exists := result["count"]; !exists || count != float64(42) {
		t.Errorf("expected count=42 in output, got: %v", result)
	}
	if msg, exists := result["message"]; !exists || msg != "success" {
		t.Errorf("expected message='success' in output, got: %v", result)
	}
}

// TestOk_ReturnsZeroExitCode tests that Ok returns exit code 0
func TestOk_ReturnsZeroExitCode(t *testing.T) {
	buf := &bytes.Buffer{}
	fields := map[string]any{}

	exitCode := output.Ok(buf, fields)

	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
}

// TestErr_EmitsValidJSON tests that Err emits a valid JSON line
func TestErr_EmitsValidJSON(t *testing.T) {
	buf := &bytes.Buffer{}
	errMsg := "something went wrong"

	exitCode := output.Err(buf, errMsg)

	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}

	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v", err)
	}

	if ok, exists := result["ok"]; !exists || ok != false {
		t.Errorf("expected ok=false in output, got: %v", result)
	}

	if errField, exists := result["error"]; !exists || errField != errMsg {
		t.Errorf("expected error='%s' in output, got: %v", errMsg, result)
	}
}

// TestErr_ReturnsOneExitCode tests that Err returns exit code 1
func TestErr_ReturnsOneExitCode(t *testing.T) {
	buf := &bytes.Buffer{}

	exitCode := output.Err(buf, "error")

	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}
}

// TestErr_TrimsTrailingNewline asserts that Err strips leading and trailing whitespace from the
// message before encoding it into the JSON error field.
// This covers embedded tool output such as "fatal: not a git repository\n" which must not leak
// newline characters into the JSON value.
func TestErr_TrimsTrailingNewline(t *testing.T) {
	buf := &bytes.Buffer{}

	// Simulate a git error string that ends with a newline.
	exitCode := output.Err(buf, "fatal: not a git repository\n")

	if exitCode != 1 {
		t.Errorf("Err(newline msg) = %d; want 1", exitCode)
	}

	// The envelope must be valid JSON.
	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("Err(newline msg) output is not valid JSON: %v; output: %q", err, buf.String())
	}

	if ok, exists := result["ok"]; !exists || ok != false {
		t.Errorf("Err(newline msg) ok = %v; want false", result["ok"])
	}

	// The error field must have the newline stripped.
	errField, _ := result["error"].(string)
	if errField != "fatal: not a git repository" {
		t.Errorf("Err(newline msg) error field = %q; want %q", errField, "fatal: not a git repository")
	}
}

// TestOk_MutatesFieldsMap tests that Ok mutates the supplied fields map by adding ok
func TestOk_MutatesFieldsMap(t *testing.T) {
	buf := &bytes.Buffer{}
	fields := map[string]any{"data": "test"}

	output.Ok(buf, fields)

	if ok, exists := fields["ok"]; !exists || ok != true {
		t.Errorf("expected Ok to mutate fields map by adding ok=true, got: %v", fields)
	}
}

// TestErrFields_EmitsOkFalseMessageAndFields asserts that ErrFields emits ok:false, the trimmed
// message, and the supplied fields.
func TestErrFields_EmitsOkFalseMessageAndFields(t *testing.T) {
	buf := &bytes.Buffer{}
	fields := map[string]any{"mutations": []string{"a", "b"}, "partial": true}

	exitCode := output.ErrFields(buf, "  something failed\n", fields)

	if exitCode != 1 {
		t.Errorf("ErrFields(...) = %d; want 1", exitCode)
	}

	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("ErrFields(...) output is not valid JSON: %v; output: %q", err, buf.String())
	}

	if ok, exists := result["ok"]; !exists || ok != false {
		t.Errorf("ErrFields(...) ok = %v; want false", result["ok"])
	}
	if errField, _ := result["error"].(string); errField != "something failed" {
		t.Errorf("ErrFields(...) error = %q; want %q", errField, "something failed")
	}
	if partial, exists := result["partial"]; !exists || partial != true {
		t.Errorf("ErrFields(...) partial = %v; want true", result["partial"])
	}
	if _, exists := result["mutations"]; !exists {
		t.Errorf("ErrFields(...) missing supplied field %q, got: %v", "mutations", result)
	}
}

// TestErrFields_NilFieldsMatchesErr asserts that ErrFields with a nil map emits byte-identical
// output to Err for the same message.
func TestErrFields_NilFieldsMatchesErr(t *testing.T) {
	msg := "something went wrong"

	errBuf := &bytes.Buffer{}
	output.Err(errBuf, msg)

	errFieldsBuf := &bytes.Buffer{}
	output.ErrFields(errFieldsBuf, msg, nil)

	if errBuf.String() != errFieldsBuf.String() {
		t.Errorf("ErrFields(w, msg, nil) = %q; want byte-identical to Err(w, msg) = %q", errFieldsBuf.String(), errBuf.String())
	}
}

// TestErrFields_ReservedKeysOverrideCallerSupplied asserts that a caller supplying "ok" or "error"
// in fields is overridden by the injected values.
func TestErrFields_ReservedKeysOverrideCallerSupplied(t *testing.T) {
	buf := &bytes.Buffer{}
	fields := map[string]any{"ok": true, "error": "caller-supplied"}

	output.ErrFields(buf, "the real error", fields)

	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("ErrFields(...) output is not valid JSON: %v; output: %q", err, buf.String())
	}

	if ok, exists := result["ok"]; !exists || ok != false {
		t.Errorf("ErrFields(...) ok = %v; want false (caller-supplied true must be overridden)", result["ok"])
	}
	if errField, _ := result["error"].(string); errField != "the real error" {
		t.Errorf("ErrFields(...) error = %q; want %q (caller-supplied value must be overridden)", errField, "the real error")
	}
}

// TestErrFields_ReturnsOneExitCode is the explicitly-named regression case for ErrFields alongside
// TestErr_ReturnsOneExitCode: ErrFields keeps returning exit code 1.
func TestErrFields_ReturnsOneExitCode(t *testing.T) {
	buf := &bytes.Buffer{}

	exitCode := output.ErrFields(buf, "error", map[string]any{"key": "value"})

	if exitCode != 1 {
		t.Errorf("ErrFields(...) = %d; want 1", exitCode)
	}
}
