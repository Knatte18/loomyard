//go:build tier0

// main_test.go — tests for the module dispatcher (main.go).
//
// Drives run() directly: module routing from an uninitialized repo, that the root hook mints nothing under test, and that it keeps the init-time log level when no -v is given.
// Help paths and unknown modules live in exitcode_test.go.
// The three tests that spawn gitexec's RunGit(["init"], …) to seed a real git repo live in main_integration_test.go per the Test Tier Purity Invariant.

package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/tendercli"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

// These tests cover module routing, not board behaviour (that lives in internal/boardcli).

// TestRunDispatchesToUninitializedRepoModules asserts modules that need a lym tree fail with exit 1 when dispatched from a temp cwd that has no _lym/ directory.
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
	t.Setenv("LYM_TRACE_ID", "")
	before := os.Getenv("LYM_TRACE_ID")

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

	if after := os.Getenv("LYM_TRACE_ID"); after != before {
		t.Errorf("LYM_TRACE_ID = %q after running the root hook under testing.Testing(); want unchanged %q", after, before)
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

// TestRootHook_NoVerboseFlagKeepsTheLevelSetAtInit asserts a run with no -v leaves the level the logger held before the hook, so a Debug record still reaches stderr.
// SetVerbosity(2) stands in for LYM_LOG_LEVEL=debug, which the logger reads only at package init.
// It mutates the process-global logger level and output, so it does not run in parallel.
func TestRootHook_NoVerboseFlagKeepsTheLevelSetAtInit(t *testing.T) {
	buf := logcapture.Capture(t)
	logger.SetVerbosity(2)

	root := newRoot()
	if err := root.PersistentPreRunE(root, nil); err != nil {
		t.Fatalf("PersistentPreRunE(root, nil) returned error: %v", err)
	}
	logger.Debug("debug after the root hook")

	if !strings.Contains(buf.String(), "debug after the root hook") {
		t.Errorf("stderr output = %q after the root hook with no -v; want the Debug record", buf.String())
	}
}

// TestPauseSlugOverride covers the pause slug the root hook hands the network gate: a set flag's value, empty included, and nil for an unset or undeclared flag.
func TestPauseSlugOverride(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		declared bool
		args     []string
		want     *string
	}{
		{"SetToSlug", true, []string{"--pause-slug", "child"}, ptr("child")},
		{"SetToEmpty", true, []string{"--pause-slug="}, ptr("")},
		{"DeclaredButUnset", true, nil, nil},
		{"NotDeclared", false, nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cmd := &cobra.Command{Use: "push"}
			if tt.declared {
				cmd.Flags().String(fabricengine.PauseSlugFlag, "", "")
			}
			if err := cmd.ParseFlags(tt.args); err != nil {
				t.Fatalf("ParseFlags(%q): %v", tt.args, err)
			}
			got := pauseSlugOverride(cmd)
			switch {
			case got == nil && tt.want == nil:
			case got == nil || tt.want == nil:
				t.Errorf("pauseSlugOverride = %v; want %v", got, tt.want)
			case *got != *tt.want:
				t.Errorf("pauseSlugOverride = %q; want %q", *got, *tt.want)
			}
		})
	}
}

// TestDetachedChild covers the gate behaviour the root hook reads off the hidden flags: a set pause-slug or deferrable flag defers, a set deferrable flag also holds the hub scope, and a set parent-locked flag is read beside a pause slug.
func TestDetachedChild(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		declared []string
		args     []string
		want     tendercli.DetachedChild
	}{
		{"PauseSlugSet", []string{fabricengine.PauseSlugFlag}, []string{"--pause-slug", "child"}, tendercli.DetachedChild{Deferrable: true}},
		{"DeferrableSet", []string{boardengine.DeferrableSyncFlag}, []string{"--deferrable"}, tendercli.DetachedChild{Deferrable: true, HubScope: true}},
		{"ParentLockedBesidePauseSlug", []string{fabricengine.PauseSlugFlag, fabricengine.ParentLockedFlag}, []string{"--pause-slug=child", "--parent-locked"}, tendercli.DetachedChild{Deferrable: true, ParentLocked: true}},
		{"DeclaredButUnset", []string{fabricengine.PauseSlugFlag, boardengine.DeferrableSyncFlag, fabricengine.ParentLockedFlag}, nil, tendercli.DetachedChild{}},
		{"NotDeclared", nil, nil, tendercli.DetachedChild{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cmd := &cobra.Command{Use: "push"}
			for _, name := range tt.declared {
				if name == fabricengine.PauseSlugFlag {
					cmd.Flags().String(name, "", "")
				} else {
					cmd.Flags().Bool(name, false, "")
				}
			}
			if err := cmd.ParseFlags(tt.args); err != nil {
				t.Fatalf("ParseFlags(%q): %v", tt.args, err)
			}
			if got := detachedChild(cmd); got != tt.want {
				t.Errorf("detachedChild = %+v; want %+v", got, tt.want)
			}
		})
	}
}

// ptr returns a pointer to a copy of s.
func ptr(s string) *string {
	return &s
}
