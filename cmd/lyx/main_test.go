// main_test.go — tests for the module dispatcher (main.go).
//
// Drives run() directly: argument routing, unknown-module handling, and that a dispatched module's exit code and output propagate unchanged.
// The three tests that spawn gitexec's RunGit(["init"], …) to seed a real git repo live in main_integration_test.go per the Test Tier Purity Invariant.

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/logger"
)

// These tests cover module routing, not board behaviour (that lives in internal/boardcli).

func TestRunNoArgs(t *testing.T) {
	var out bytes.Buffer
	if code := run(nil, &out); code != 0 {
		t.Fatalf("expected exit 0 for no args, got %d; output: %q", code, out.String())
	}
	got := out.String()
	if got == "" {
		t.Fatal("expected non-empty help output for no args")
	}
	for _, module := range []string{"board"} {
		if !strings.Contains(got, module) {
			t.Errorf("expected help output to name module %q; got:\n%s", module, got)
		}
	}
	if strings.Contains(got, `"ok":false`) {
		t.Errorf("bare lyx emitted a JSON error envelope; help paths must not be wrapped; output:\n%s", got)
	}
}

func TestRunUnknownModule(t *testing.T) {
	var out bytes.Buffer
	if code := run([]string{"bogus", "list"}, &out); code != 1 {
		t.Fatalf("expected exit 1 for unknown module, got %d", code)
	}
	got := out.String()
	if !strings.Contains(got, "unknown command") {
		t.Errorf("expected %q in output for unknown module; got: %q", "unknown command", got)
	}

	var env map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(got)), &env); err != nil {
		t.Fatalf("run([bogus list]) output is not valid JSON: %v; output: %q", err, got)
	}
	if ok, _ := env["ok"].(bool); ok {
		t.Errorf("run([bogus list]) envelope ok = true; want false")
	}
}

func TestRunDispatchesToIDE(t *testing.T) {
	// Create temp cwd with no _lyx/ directory, causing ide.RunCLI to fail.
	cwd := t.TempDir()
	t.Chdir(cwd)

	var out bytes.Buffer
	code := run([]string{"ide", "spawn", "test"}, &out)
	if code != 1 {
		t.Fatalf("expected exit 1 for ide in uninitialized repo, got %d; output: %s", code, out.String())
	}
	if !strings.Contains(out.String(), `"ok":false`) {
		t.Fatalf("expected error JSON on out, got %q", out.String())
	}
}

// TestRootHookSuppressedUnderTest verifies the root hook mints/exports nothing under testing.Testing().
func TestRootHookSuppressedUnderTest(t *testing.T) {
	t.Setenv("LYX_TRACE_ID", "")
	before := os.Getenv("LYX_TRACE_ID")

	sinkDir := t.TempDir()
	logger.SetDurableSinkDir(sinkDir)
	t.Cleanup(func() { logger.SetDurableSinkDir("") })

	root := newRoot()
	if root.PersistentPreRunE == nil {
		t.Fatal("newRoot() root command has no PersistentPreRunE")
	}
	if err := root.PersistentPreRunE(root, nil); err != nil {
		t.Fatalf("PersistentPreRunE(root, nil) returned error: %v", err)
	}

	if after := os.Getenv("LYX_TRACE_ID"); after != before {
		t.Errorf("LYX_TRACE_ID = %q after running the root hook under testing.Testing(); want unchanged %q", after, before)
	}

	entries, err := os.ReadDir(sinkDir)
	if err != nil {
		t.Fatalf("ReadDir(%q) after root hook: %v", sinkDir, err)
	}
	if len(entries) != 0 {
		t.Errorf("durable sink dir has %d entries after root hook under testing.Testing(); want 0 — the sink must never open", len(entries))
	}

	if !testing.Testing() {
		t.Fatalf("testing.Testing() = false inside a test binary; the root hook's suppression wiring relies on it being true here")
	}
}

func TestRunDispatchesToConfig(t *testing.T) {
	// Create temp cwd with no _lyx/ directory, causing config resolution to fail.
	cwd := t.TempDir()
	t.Chdir(cwd)

	var out bytes.Buffer
	code := run([]string{"config"}, &out)
	if code != 1 {
		t.Fatalf("expected exit 1 for config in uninitialized repo, got %d; output: %s", code, out.String())
	}
}
