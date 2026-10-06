// main_test.go — tests for the module dispatcher (main.go).
//
// Drives run() directly: module routing from an uninitialized repo, and that the root hook mints nothing under test.
// Help paths and unknown modules live in exitcode_test.go.
// The three tests that spawn gitexec's RunGit(["init"], …) to seed a real git repo live in main_integration_test.go per the Test Tier Purity Invariant.

package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/logger"
)

// These tests cover module routing, not board behaviour (that lives in internal/boardcli).

// TestRunDispatchesToUninitializedRepoModules asserts modules that need a lyx tree fail with exit 1 when dispatched from a temp cwd that has no _lyx/ directory.
// Each row chdirs, which is process-global state, so neither the test nor its rows run in parallel.
func TestRunDispatchesToUninitializedRepoModules(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"ide", []string{"ide", "spawn", "test"}},
		{"config", []string{"config", "--print"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())

			var out bytes.Buffer
			code := run(tt.args, &out)
			if code != 1 {
				t.Fatalf("expected exit 1 for %v in uninitialized repo, got %d; output: %s", tt.args, code, out.String())
			}
			if !strings.Contains(out.String(), `"ok":false`) {
				t.Fatalf("expected error JSON on out, got %q", out.String())
			}
		})
	}
}

// TestRootHookSuppressedUnderTest verifies the root hook mints/exports nothing under testing.Testing().
//
//testtiming:keep pins that the root hook mints no trace id and opens no durable sink under testing.Testing(), which the tree walk never reads
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
