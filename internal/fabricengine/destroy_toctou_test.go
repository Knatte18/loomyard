// destroy_toctou_test.go guards the act-time containment binding that closed R3's containment TOCTOU:
// removeContainedPath must refuse to carry a removal through a symlink escaping its container, must
// still remove a legitimate nested entry and a final-component link (a junction), and must treat an
// absent target as an idempotent no-op.
//
// It is a hermetic, single-instant test of the primitive the fix moved the enforcement into, and that
// is the point: the original defect escaped only in the check-then-act window (dangling at check,
// live-escaping at act), so a single-instant test of the OLD nominal-path act would have missed it —
// the old check refused a symlink that was already live. The fix makes the ACT itself refuse an
// escaping component regardless of when the symlink went live, so pinning that act-time property with
// a live-escaping symlink present at removal time is exactly what would fail on a regression back to a
// nominal-path unlink.

package fabricengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRemoveContainedPath_RefusesEscapingIntermediate asserts that a removal whose target is reachable
// only by traversing a symlink that escapes the container is refused, with the outside file preserved.
func TestRemoveContainedPath_RefusesEscapingIntermediate(t *testing.T) {
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

	// An intermediate segment of the target escapes the container.
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

// TestRemoveContainedPath_RemovesLegitimateNested asserts a real nested entry is removed and recorded
// as a directory or single entry correctly.
func TestRemoveContainedPath_RemovesLegitimateNested(t *testing.T) {
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

// TestRemoveContainedPath_RemovesFinalLinkNotTarget asserts a final-component link (a junction) is
// removed as a link, leaving the escaping target it pointed at untouched.
func TestRemoveContainedPath_RemovesFinalLinkNotTarget(t *testing.T) {
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
func TestRemoveContainedPath_AbsentIsNoOp(t *testing.T) {
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

// TestCreateExclusiveDir_RefusesLeafSymlink pins createExclusiveDir's actual containment guarantee — the
// dedicated regression round 5's F2/F3 lacked (they leaned on the shared-mechanism coverage): a symlink
// planted at the leaf createExclusiveDir would mint is refused as an EEXIST rather than followed, so the
// gate never mints a createdToken for a directory it did not itself bring into being. This is the leaf
// property CloneHub's hub bootstrap relies on; the intermediate-ancestor case is documented as NOT
// refused (os.OpenRoot resolves the parent), so this test deliberately pins only the leaf guarantee.
func TestCreateExclusiveDir_RefusesLeafSymlink(t *testing.T) {
	base := t.TempDir()
	container := filepath.Join(base, "container")
	outside := filepath.Join(base, "outside")
	if err := os.MkdirAll(container, 0o755); err != nil {
		t.Fatalf("mkdir container: %v", err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}

	// Plant a symlink at the exact leaf createExclusiveDir would mint, pointing at an existing outside
	// directory — the shape that would let a mkdir-follows-symlink adopt an out-of-container directory as
	// though the gate had created it.
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
