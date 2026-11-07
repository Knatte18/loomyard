// destroy_toctou_test.go pins the act-time containment of removeContainedPath.
// The act refuses a removal that reaches its target through a symlink escaping the container.
// It does so regardless of when the symlink went live, since a symlink can go live between a check and the act.
// The tests also pin removal of a nested entry and a final-component link (a junction).
// An absent target is an idempotent no-op.

package fabricengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestRemoveContainedPath_RefusesEscapingIntermediate asserts that a removal is refused:
// the target is reachable only by traversing a symlink that escapes the container, and the outside file survives.
//
//testtiming:keep a removal reachable only through an escaping symlink being refused with the outside file preserved; coverage of its blocks by other tests does not show an assertion of this
func TestRemoveContainedPath_RefusesEscapingIntermediate(t *testing.T) {
	tiergate.Require(t, 0)
	base := t.TempDir()
	container := filepath.Join(base, "container")
	outside := filepath.Join(base, "outside")
	if err := os.MkdirAll(container, 0o755); err != nil {
		t.Fatalf("mkdir container: %v", err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}
	precious := filepath.Join(outside, "precious.txt")
	if err := os.WriteFile(precious, []byte("PRECIOUS"), 0o644); err != nil {
		t.Fatalf("write precious: %v", err)
	}

	if err := os.Symlink(outside, filepath.Join(container, "escape")); err != nil {
		t.Fatalf("symlink escape: %v", err)
	}

	target := filepath.Join(container, "escape", "precious.txt")
	removed, _, err := removeContainedPath(container, target, true)
	if err == nil {
		t.Fatalf("removeContainedPath must refuse an escaping-intermediate target, got nil error")
	}
	if !strings.Contains(err.Error(), "escapes") {
		t.Fatalf("expected an escape error, got: %v", err)
	}
	if removed {
		t.Fatalf("removeContainedPath reported removed=true for a refused escape")
	}
	if _, statErr := os.Stat(precious); statErr != nil {
		t.Fatalf("outside file was deleted through the escaping symlink: %v", statErr)
	}
}

// TestRemoveContainedPath_RemovesLegitimateNested asserts that a real nested directory is removed.
// It also asserts that the removal reports the entry as a directory.
//
//testtiming:keep a real nested entry being removed and recorded as a directory or single entry; coverage of its blocks by other tests does not show an assertion of this
func TestRemoveContainedPath_RemovesLegitimateNested(t *testing.T) {
	tiergate.Require(t, 0)
	base := t.TempDir()
	container := filepath.Join(base, "c")
	if err := os.MkdirAll(filepath.Join(container, "sub"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	file := filepath.Join(container, "sub", "f.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	removed, wasDir, err := removeContainedPath(container, file, false)
	if err != nil {
		t.Fatalf("unexpected error removing a legitimate nested file: %v", err)
	}
	if !removed || wasDir {
		t.Fatalf("expected removed=true wasDir=false, got removed=%v wasDir=%v", removed, wasDir)
	}
	if _, statErr := os.Stat(file); !os.IsNotExist(statErr) {
		t.Fatalf("legitimate nested file was not removed: %v", statErr)
	}

	dir := filepath.Join(container, "sub")
	removed, wasDir, err = removeContainedPath(container, dir, true)
	if err != nil {
		t.Fatalf("unexpected error removing a legitimate nested dir: %v", err)
	}
	if !removed || !wasDir {
		t.Fatalf("expected removed=true wasDir=true, got removed=%v wasDir=%v", removed, wasDir)
	}
}

// TestRemoveContainedPath_RemovesFinalLinkNotTarget asserts that a final-component link (a junction) is removed.
// The escaping target the link points at stays untouched.
//
//testtiming:keep a final-component junction being removed as a link while its escaping target stays untouched; coverage of its blocks by other tests does not show an assertion of this
func TestRemoveContainedPath_RemovesFinalLinkNotTarget(t *testing.T) {
	tiergate.Require(t, 0)
	base := t.TempDir()
	container := filepath.Join(base, "c")
	outside := filepath.Join(base, "outside")
	if err := os.MkdirAll(container, 0o755); err != nil {
		t.Fatalf("mkdir container: %v", err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}
	leaf := filepath.Join(outside, "leaf.txt")
	if err := os.WriteFile(leaf, []byte("keep"), 0o644); err != nil {
		t.Fatalf("write leaf: %v", err)
	}

	link := filepath.Join(container, "junction")
	if err := os.Symlink(leaf, link); err != nil {
		t.Fatalf("symlink junction: %v", err)
	}

	removed, _, err := removeContainedPath(container, link, false)
	if err != nil {
		t.Fatalf("unexpected error removing a final-component link: %v", err)
	}
	if !removed {
		t.Fatalf("expected the link to be removed")
	}
	if _, statErr := os.Lstat(link); !os.IsNotExist(statErr) {
		t.Fatalf("junction link was not removed: %v", statErr)
	}
	if _, statErr := os.Stat(leaf); statErr != nil {
		t.Fatalf("junction target was deleted along with the link: %v", statErr)
	}
}

// TestRemoveContainedPath_AbsentIsNoOp asserts an already-absent target is an idempotent no-op.
//
//testtiming:keep an already-absent target being an idempotent no-op; coverage of its blocks by other tests does not show an assertion of this
func TestRemoveContainedPath_AbsentIsNoOp(t *testing.T) {
	tiergate.Require(t, 0)
	base := t.TempDir()
	container := filepath.Join(base, "c")
	if err := os.MkdirAll(container, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	removed, _, err := removeContainedPath(container, filepath.Join(container, "nope"), true)
	if err != nil {
		t.Fatalf("absent target must be a no-op, got error: %v", err)
	}
	if removed {
		t.Fatalf("absent target must report removed=false")
	}
}

// TestCreateExclusiveDir_RefusesLeafSymlink asserts that createExclusiveDir refuses a symlink planted at the leaf.
// It is refused as EEXIST rather than followed, so the gate never mints a token for a directory it did not create.
// A symlink at an intermediate ancestor is not refused, since os.OpenRoot resolves the parent.
// This test pins only the leaf guarantee.
//
//testtiming:keep createExclusiveDir refusing a symlink planted at the leaf as EEXIST so the gate never mints a token for a directory it did not create; coverage of its blocks by other tests does not show an assertion of this
func TestCreateExclusiveDir_RefusesLeafSymlink(t *testing.T) {
	tiergate.Require(t, 0)
	base := t.TempDir()
	container := filepath.Join(base, "container")
	outside := filepath.Join(base, "outside")
	if err := os.MkdirAll(container, 0o755); err != nil {
		t.Fatalf("mkdir container: %v", err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}

	// Plant a symlink at the leaf createExclusiveDir would mint, pointing at an existing outside directory.
	// A mkdir that follows the symlink would adopt an out-of-container directory as one the gate created.
	leaf := filepath.Join(container, "hub")
	if err := os.Symlink(outside, leaf); err != nil {
		t.Fatalf("plant leaf symlink: %v", err)
	}

	rec := NewMutations(base)
	if _, err := createExclusiveDir(rec, leaf); err == nil {
		t.Fatalf("createExclusiveDir minted a token for a pre-planted leaf symlink; want a refusal")
	}
	// The refusal must not have recorded a dir_created for a directory it did not create.
	if got := rec.Len(); got != 0 {
		t.Fatalf("createExclusiveDir recorded %d mutation(s) on a refused leaf symlink; want 0", got)
	}
}
