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
	"strings"
	"testing"
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
	for _, module := range []string{"board", "warp"} {
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

func TestRunDispatchesToWarp(t *testing.T) {
	// Create temp cwd with no _lyx/ directory.
	// This will cause LoadConfig to fail, which warp.RunCLI will return
	// as an error envelope.
	cwd := t.TempDir()
	t.Chdir(cwd)

	var out bytes.Buffer
	code := run([]string{"warp", "list"}, &out)
	if code != 1 {
		t.Fatalf("expected exit 1 for warp in uninitialized repo, got %d; output: %s", code, out.String())
	}
	if !strings.Contains(out.String(), `"ok":false`) {
		t.Fatalf("expected error JSON on out, got %q", out.String())
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

func TestRunDispatchesToWeft(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "1")
	// Create temp cwd with no _lyx/ directory.
	// This will cause config/layout resolution to fail, which weft.RunCLI
	// will return as an error envelope.
	cwd := t.TempDir()
	t.Chdir(cwd)

	var out bytes.Buffer
	code := run([]string{"weft", "status"}, &out)
	if code != 1 {
		t.Fatalf("expected exit 1 for weft in uninitialized repo, got %d; output: %s", code, out.String())
	}
	if !strings.Contains(out.String(), `"ok":false`) {
		t.Fatalf("expected error JSON on out, got %q", out.String())
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

func TestRunDispatchesToWarpClone(t *testing.T) {
	// Test dispatching to warp clone with missing arguments.
	// warp.RunCLI should return an error envelope with ok=false and exit code 1.
	var out bytes.Buffer
	code := run([]string{"warp", "clone"}, &out)
	if code != 1 {
		t.Fatalf("expected exit 1 for warp clone with no args, got %d; output: %s", code, out.String())
	}
	if !strings.Contains(out.String(), `"ok":false`) {
		t.Fatalf("expected error JSON on out, got %q", out.String())
	}
}
