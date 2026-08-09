//go:build integration

// clone_explicit_root_subpath_test.go pins the one value that used to escape clone's
// never-silently-re-anchor rule: an explicit `--subpath .` against a hub whose recorded anchor is a
// real subpath.
// Every other disagreeing value was already refused; "." was exempted because the CLI flag's own
// default was "." and CloneHub could not tell an explicit request from an unset one.
//
// Package fabricengine_test to reuse the bare-remote fixture helpers from clone_adopt_test.go;
// shares the TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// TestCloneHub_ExplicitRootSubpathRefusesRecordedSubpath asserts an explicit "." is refused against
// a recorded "backend", and that leaving Subpath unset still adopts the record.
func TestCloneHub_ExplicitRootSubpathRefusesRecordedSubpath(t *testing.T) {
	fixtures := t.TempDir()

	warpBare := makeBareRemoteWithSubdir(t, fixtures, "explicit-root-warp", "backend")
	weftBare := makeBareRemote(t, fixtures, "explicit-root-weft")
	commitFileOnBranch(t, fixtures, weftBare, "main", lyxcwd.AnchorFileName, "backend\n")

	cloneParent := t.TempDir()

	_, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
		WeftURL: filepath.ToSlash(weftBare),
		WarpURL: filepath.ToSlash(warpBare),
		Subpath: ".",
	})
	if err == nil {
		t.Fatal("CloneHub(Subpath: \".\") = nil error against a hub recorded at \"backend\"; want a refusal")
	}
	if !strings.Contains(err.Error(), "does not match the recorded anchor") {
		t.Errorf("CloneHub() error = %v; want it to name the recorded-anchor mismatch", err)
	}

	// The refusal leaves no hub behind, exactly like every other disagreeing value.
	if entries, readErr := os.ReadDir(cloneParent); readErr != nil {
		t.Fatalf("read clone parent: %v", readErr)
	} else if len(entries) != 0 {
		t.Errorf("clone parent holds %d entries after a refused clone; want none", len(entries))
	}

	// An UNSET subpath still adopts the record — the empty default must not become a refusal.
	res, err := fabricengine.CloneHub(cloneParent, fabricengine.CloneOptions{
		WeftURL: filepath.ToSlash(weftBare),
		WarpURL: filepath.ToSlash(warpBare),
	})
	if err != nil {
		t.Fatalf("CloneHub() with no Subpath error = %v; want the recorded anchor adopted", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(res.HubPath) })
	if res.Anchor != "backend" {
		t.Errorf("res.Anchor = %q; want %q", res.Anchor, "backend")
	}
}
