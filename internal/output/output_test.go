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

	// Parse the output
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

	// Parse the output
	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v", err)
	}

	// Check that ok is false
	if ok, exists := result["ok"]; !exists || ok != false {
		t.Errorf("expected ok=false in output, got: %v", result)
	}

	// Check that error message is present
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

// TestOk_MutatesFieldsMap tests that Ok mutates the supplied fields map by adding ok
func TestOk_MutatesFieldsMap(t *testing.T) {
	buf := &bytes.Buffer{}
	fields := map[string]any{"data": "test"}

	output.Ok(buf, fields)

	// Check that the map was mutated (ok was added)
	if ok, exists := fields["ok"]; !exists || ok != true {
		t.Errorf("expected Ok to mutate fields map by adding ok=true, got: %v", fields)
	}
}
