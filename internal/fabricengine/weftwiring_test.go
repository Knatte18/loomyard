// weftwiring_test.go unit-tests removeJunctionRecords directly against
// synthetic hubgeometry.HostJunction slices — no build tag, since it touches
// only plain directories and fslink, never git. It exists because
// l.HostJunctions(slug) still returns exactly one entry in this batch (a
// second entry is batch 5's job), so removeHostJunction's best-effort,
// continue-past-failure contract cannot be driven through the exported
// (l, slug) surface with more than one junction; this file drives the
// extracted loop directly instead.

package fabricengine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
)

// TestRemoveJunctionRecords_ContinuesPastFailure is card 9's regression
// guard: with one junction in a state that makes its removal fail (a real,
// non-empty directory, which fslink.Remove cannot delete), the others —
// before and after it in the slice — are still removed. This is the opposite
// contract from unseedJunctionRecords (card 8), which aborts on the first
// failure; both are exercised the same way for the same reason: neither is
// drivable with more than one junction through l.HostJunctions(slug) yet.
func TestRemoveJunctionRecords_ContinuesPastFailure(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	firstLink := filepath.Join(root, "first-link")
	firstTarget := filepath.Join(root, "first-target")
	wireTestJunction(t, firstLink, firstTarget)

	// The middle junction's host path is a real, non-empty directory —
	// fslink.Remove (a bare os.Remove) cannot delete a non-empty directory,
	// so this is guaranteed to fail regardless of platform.
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

	junctions := []hubgeometry.HostJunction{
		{Name: "first", Link: firstLink, Target: firstTarget},
		{Name: "middle", Link: middleLink, Target: filepath.Join(root, "middle-target")},
		{Name: "last", Link: lastLink, Target: lastTarget},
	}

	err := removeJunctionRecords(junctions)
	if err == nil {
		t.Fatal("removeJunctionRecords = nil error; want a joined error from the middle junction")
	}

	// Both the junction before AND after the failing one are removed — proving
	// the loop continued rather than aborting at the first failure.
	if _, statErr := os.Lstat(firstLink); !os.IsNotExist(statErr) {
		t.Errorf("first junction %s still exists; want removed despite middle's failure", firstLink)
	}
	if _, statErr := os.Lstat(lastLink); !os.IsNotExist(statErr) {
		t.Errorf("last junction %s still exists; want removed despite middle's failure", lastLink)
	}

	// The failing directory itself is untouched (fslink.Remove never partially
	// deletes a real, non-empty directory).
	if info, statErr := os.Stat(middleLink); statErr != nil || !info.IsDir() {
		t.Errorf("middle host dir %s not left in place: stat err=%v", middleLink, statErr)
	}
}

// TestRemoveJunctionRecords_EmptyIsNoOp asserts that an empty junctions slice
// (matching l.HostJunctions(slug) before any junction has ever been wired) is
// a legitimate no-op.
func TestRemoveJunctionRecords_EmptyIsNoOp(t *testing.T) {
	t.Parallel()

	if err := removeJunctionRecords(nil); err != nil {
		t.Errorf("removeJunctionRecords(nil) = %v; want nil", err)
	}
}
