// weftwiring_test.go unit-tests removeJunctionRecords directly against synthetic WarpJunction
// slices — no build tag, since it touches only plain directories and fslink, never git.
// It exists because WarpJunctions(l, slug) still returns exactly one entry in this batch (a second
// entry is batch 5's job), so removeWarpJunction's best-effort, continue-past-failure contract
// cannot be driven through the exported (l, slug) surface with more than one junction;
// this file drives the extracted loop directly instead.

package fabricengine

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRemoveJunctionRecords_ContinuesPastFailure proves the function continues removing junctions
// after a per-junction failure.
func TestRemoveJunctionRecords_ContinuesPastFailure(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	firstLink := filepath.Join(root, "first-link")
	firstTarget := filepath.Join(root, "first-target")
	wireTestJunction(t, firstLink, firstTarget)

	middleLink := filepath.Join(root, "middle-link")
	if err := os.MkdirAll(middleLink, 0o755); err != nil {
		t.Fatalf("mkdir real middle-link dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(middleLink, "marker.txt"), []byte("content"), 0o644); err != nil {
		t.Fatalf("write marker file: %v", err)
	}

	lastLink := filepath.Join(root, "last-link")
	lastTarget := filepath.Join(root, "last-target")
	wireTestJunction(t, lastLink, lastTarget)

	junctions := []WarpJunction{
		{Name: "first", Link: firstLink, Target: firstTarget},
		{Name: "middle", Link: middleLink, Target: filepath.Join(root, "middle-target")},
		{Name: "last", Link: lastLink, Target: lastTarget},
	}

	err := removeJunctionRecords(root, junctions)
	if err == nil {
		t.Fatal("removeJunctionRecords = nil error; want a joined error from the middle junction")
	}

	if _, statErr := os.Lstat(firstLink); !os.IsNotExist(statErr) {
		t.Errorf("first junction %s still exists; want removed despite middle's failure", firstLink)
	}
	if _, statErr := os.Lstat(lastLink); !os.IsNotExist(statErr) {
		t.Errorf("last junction %s still exists; want removed despite middle's failure", lastLink)
	}

	if info, statErr := os.Stat(middleLink); statErr != nil || !info.IsDir() {
		t.Errorf("middle warp dir %s not left in place: stat err=%v", middleLink, statErr)
	}
}

// TestRemoveJunctionRecords_EmptyIsNoOp asserts that an empty junctions slice is a no-op.
func TestRemoveJunctionRecords_EmptyIsNoOp(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	if err := removeJunctionRecords(root, nil); err != nil {
		t.Errorf("removeJunctionRecords(root, nil) = %v; want nil", err)
	}
}
