// main_test.go — tests for the module dispatcher (main.go).
//
// Drives run() directly: argument routing, unknown-module handling, and that a
// dispatched module's exit code and output propagate unchanged. The three
// tests that spawn gitexec's RunGit(["init"], …) to seed a real git repo live
// in main_integration_test.go per the Test Tier Purity Invariant.

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/logger"
)

// These tests cover main's own responsibility — module routing — not the board
// behaviour itself (that lives in internal/boardcli / internal/boardengine). They
// drive run() directly so no binary build or os.Exit is involved.

func TestRunNoArgs(t *testing.T) {
	var out bytes.Buffer
	// Cobra root with no subcommand prints help and exits 0.
	if code := run(nil, &out); code != 0 {
		t.Fatalf("expected exit 0 for no args, got %d; output: %q", code, out.String())
	}
	// Help output must be non-empty and name a representative set of modules so
	// the tree is self-documenting at the root level.
	got := out.String()
	if got == "" {
		t.Fatal("expected non-empty help output for no args")
	}
	for _, module := range []string{"board"} {
		if !strings.Contains(got, module) {
			t.Errorf("expected help output to name module %q; got:\n%s", module, got)
		}
	}
	// Help is plain-text, never a JSON error envelope.
	if strings.Contains(got, `"ok":false`) {
		t.Errorf("bare lyx emitted a JSON error envelope; help paths must not be wrapped; output:\n%s", got)
	}
}

func TestRunUnknownModule(t *testing.T) {
	var out bytes.Buffer
	if code := run([]string{"bogus", "list"}, &out); code != 1 {
		t.Fatalf("expected exit 1 for unknown module, got %d", code)
	}
	// The "unknown command" text must be present — now embedded in the JSON error value.
	got := out.String()
	if !strings.Contains(got, "unknown command") {
		t.Errorf("expected %q in output for unknown module; got: %q", "unknown command", got)
	}

	// The output must be a well-formed JSON envelope with ok=false.
	var env map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(got)), &env); err != nil {
		t.Fatalf("run([bogus list]) output is not valid JSON: %v; output: %q", err, got)
	}
	if ok, _ := env["ok"].(bool); ok {
		t.Errorf("run([bogus list]) envelope ok = true; want false")
	}
}

func TestRunDispatchesToIDE(t *testing.T) {
	// Create temp cwd with no _lyx/ directory.
	// This will cause ide.RunCLI to return an error (failed to resolve layout).
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

// TestRootHookSuppressedUnderTest pins that the root command's
// PersistentPreRunE (main.go's Card 27 wiring) mints/exports nothing and
// never opens the durable sink when testing.Testing() is true, mirroring
// TestHeaderLaunchLine's shape (internal/reedengine/headerpane_test.go): a
// known-empty LYX_TRACE_ID starting state must stay unchanged, and a
// SetDurableSinkDir-pointed temp directory must stay empty, after invoking
// the hook in-process.
func TestRootHookSuppressedUnderTest(t *testing.T) {
	// Pin a known starting environment state via t.Setenv so the assertion
	// below proves the hook left LYX_TRACE_ID untouched, not merely absent
	// by chance ordering of other tests in this package.
	t.Setenv("LYX_TRACE_ID", "")
	before := os.Getenv("LYX_TRACE_ID")

	// Point the durable sink's test seam at a temp dir and reset all sink
	// package state on cleanup, per SetDurableSinkDir's documented contract
	// (internal/logger/sink.go) — otherwise this test's state could leak
	// into any test that runs after it in the same binary.
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

	// Pin the precondition itself, like TestHeaderLaunchLine's final
	// assertion: the suppression wiring's testing.Testing() gate cannot
	// silently decay into a constant false without this test catching it.
	if !testing.Testing() {
		t.Fatalf("testing.Testing() = false inside a test binary; the root hook's suppression wiring relies on it being true here")
	}
}

func TestRunDispatchesToConfig(t *testing.T) {
	// Create temp cwd with no _lyx/ directory.
	// This will cause config resolution to fail, which configcli.RunCLI
	// will return as a JSON error envelope (ok:false) at exit code 1.
	cwd := t.TempDir()
	t.Chdir(cwd)

	var out bytes.Buffer
	code := run([]string{"config"}, &out)
	if code != 1 {
		t.Fatalf("expected exit 1 for config in uninitialized repo, got %d; output: %s", code, out.String())
	}
	// config errors are emitted as the JSON envelope (ok:false); exit code is the
	// only assertion here because the precise error text is an implementation detail.
}
