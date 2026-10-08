//go:build integration

// wiring_parentbranch_integration_test.go covers the webster RunDeps.ParentBranch seam wire() fills: it reads the parent branch from the pair's origin record, which needs a git repository for the records side's lock directory.
// This package's own testmain_test.go arms the hermetic git test environment for the whole binary, so this file adds no TestMain.

package loomcli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// TestWire_WebsterParentBranchReadsPairOrigin asserts the wired RunDeps.ParentBranch returns the parent branch the pair's origin record names.
func TestWire_WebsterParentBranchReadsPairOrigin(t *testing.T) {
	loc := hubLocation(t, "pair", ".")

	c := &loomCLI{runID: shedrun.SelfRunID}
	if err := c.wire(loc, loc.AnchorPath()); err != nil {
		t.Fatalf("wire() = %v; want nil", err)
	}
	if c.runDeps.ParentBranch == nil {
		t.Fatal("runDeps.ParentBranch = nil; want the origin-record reader")
	}

	// The records sibling is a repository of its own, whose exclude file the origin read seeds.
	records := fabricengine.RecordsWorktree(loc)
	if err := os.MkdirAll(records, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", records, err)
	}
	gitkit.Git(t, records, "init")

	recordPath := fabricengine.OriginRecordPath(loc)
	if err := os.MkdirAll(filepath.Dir(recordPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", filepath.Dir(recordPath), err)
	}
	if err := os.WriteFile(recordPath, []byte(`{"parent_branch":"parent-branch"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", recordPath, err)
	}

	got, err := c.runDeps.ParentBranch()
	if err != nil {
		t.Fatalf("runDeps.ParentBranch() error = %v; want nil", err)
	}
	if got != "parent-branch" {
		t.Errorf("runDeps.ParentBranch() = %q; want %q", got, "parent-branch")
	}
}
