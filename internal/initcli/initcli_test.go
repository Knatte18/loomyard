//go:build integration

// initcli_test.go covers the lyx init cobra surface: flag dispatch between
// plain init and --undo, JSON envelope formatting, and error passthrough.
// The behavioral matrix (junction wiring, gitignore reconciliation, undo
// reversal, idempotency, abort guards, partial recovery) is covered directly
// against internal/initengine's Init/Undo, not duplicated here.

package initcli_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/Knatte18/loomyard/internal/initcli"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// TestRunInit_Smoke verifies that plain `lyx init` wires through to
// initengine.Init and formats its result as the {"ok":true,...} envelope
// with the expected top-level keys.
func TestRunInit_Smoke(t *testing.T) {
	f := lyxtest.CopyPairedLocal(t)
	t.Chdir(f.Layout.WorktreeRoot)

	var buf bytes.Buffer
	if code := initcli.RunInit(&buf, []string{}); code != 0 {
		t.Fatalf("RunInit() = %d; want 0, output: %s", code, buf.String())
	}

	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("parse JSON: %v, output: %s", err, buf.String())
	}
	if ok, _ := result["ok"].(bool); !ok {
		t.Error("ok flag is not true")
	}
	for _, key := range []string{"lyx_dir", "gitignore", "modules"} {
		if _, present := result[key]; !present {
			t.Errorf("result missing %q key; output: %s", key, buf.String())
		}
	}
}

// TestRunInit_UndoFlagDispatch verifies that the --undo flag routes to
// initengine.Undo (not Init) and formats its result as the {"ok":true,...}
// envelope with the undo-specific top-level keys.
func TestRunInit_UndoFlagDispatch(t *testing.T) {
	f := lyxtest.CopyPairedLocal(t)
	t.Chdir(f.Layout.WorktreeRoot)
	// CopyPairedLocal's weft-prime origin is left pointing at the shared
	// template bare (never rewritten); skip push so --undo cannot reach it.
	t.Setenv("WEFT_SKIP_PUSH", "1")

	var buf bytes.Buffer
	if code := initcli.RunInit(&buf, []string{}); code != 0 {
		t.Fatalf("RunInit() = %d; want 0, output: %s", code, buf.String())
	}

	var buf2 bytes.Buffer
	code := initcli.RunInit(&buf2, []string{"--undo"})
	if code != 0 {
		t.Fatalf("RunInit(--undo) = %d; want 0, output: %s", code, buf2.String())
	}

	var result map[string]any
	if err := json.Unmarshal(buf2.Bytes(), &result); err != nil {
		t.Fatalf("parse JSON: %v, output: %s", err, buf2.String())
	}
	if ok, _ := result["ok"].(bool); !ok {
		t.Errorf("ok flag is not true; output: %s", buf2.String())
	}
	for _, key := range []string{"lyx_junction", "weft_content", "git_exclude", "gitignore"} {
		if _, present := result[key]; !present {
			t.Errorf("result missing %q key; output: %s", key, buf2.String())
		}
	}
}

// TestRunInit_NotAGitRepo verifies that lyx init run from a non-git temp
// directory surfaces initengine's bare error unprefixed through the JSON
// error envelope — the cli layer must not restate or wrap it.
func TestRunInit_NotAGitRepo(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	var buf bytes.Buffer
	runExitCode := initcli.RunInit(&buf, []string{})

	if runExitCode == 0 {
		t.Errorf("RunInit() = 0; want non-zero (error) when not a git repository")
	}

	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("parse JSON: %v, output: %s", err, buf.String())
	}
	errMsg, _ := result["error"].(string)
	if errMsg != "not a git repository" {
		t.Errorf("RunInit() error = %q; want exactly \"not a git repository\"", errMsg)
	}
}
