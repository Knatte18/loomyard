//go:build integration

// ready_integration_test.go covers Ready(l)'s three outcomes: sibling absent, sibling present
// (fixture-backed via gitkit.CopyPaired), and a stat failure other than not-exist.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// TestReady_SiblingAbsent asserts that Ready returns (false, nil) when the weft sibling worktree
// does not exist.
func TestReady_SiblingAbsent(t *testing.T) {
	fixture := gitkit.CopyPaired(t)

	if err := os.RemoveAll(fixture.WeftPrime); err != nil {
		t.Fatalf("RemoveAll(weftPrime): %v", err)
	}

	ready, err := fabricengine.Ready(fixture.Layout)
	if err != nil {
		t.Fatalf("Ready() error = %v; want nil", err)
	}
	if ready {
		t.Error("Ready() = true; want false when the sibling worktree is absent")
	}
}

// TestReady_SiblingPresent asserts that Ready returns (true, nil) when the weft sibling worktree
// exists.
func TestReady_SiblingPresent(t *testing.T) {
	fixture := gitkit.CopyPaired(t)

	ready, err := fabricengine.Ready(fixture.Layout)
	if err != nil {
		t.Fatalf("Ready() error = %v; want nil", err)
	}
	if !ready {
		t.Error("Ready() = false; want true when the sibling worktree is present")
	}
}

// TestReady_StatFailure asserts that Ready returns (false, err) for a stat failure other than
// not-exist — simulated with a path whose parent path component is a regular file, since no
// portable chmod-based simulation exists across platforms.
func TestReady_StatFailure(t *testing.T) {
	tmp := t.TempDir()
	regularFile := filepath.Join(tmp, "not-a-hub")
	if err := os.WriteFile(regularFile, []byte("x"), 0o644); err != nil {
		t.Fatalf("write regular file: %v", err)
	}

	// A Location whose HubPath sits under a regular file: WeftWorktree(l)'s stat
	// fails with "not a directory", not "not exist".
	l := &lyxcwd.Location{HubPath: filepath.Join(regularFile, "hub"), WorktreeName: "worktree"}

	ready, err := fabricengine.Ready(l)
	if err == nil {
		t.Fatal("Ready() error = nil; want a non-not-exist stat failure")
	}
	if ready {
		t.Error("Ready() = true; want false on a stat failure")
	}
}
