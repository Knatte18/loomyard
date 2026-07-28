// junction_test.go unit-tests unseedJunctionRecords directly against synthetic
// hubgeometry.HostJunction slices — no build tag, since it touches only plain
// directories and fslink, never git. It exists because l.HostJunctions(slug)
// still returns exactly one entry in this batch (a second entry is batch 5's
// job), so the abort-and-accumulate contract this card gives unseedLyxJunction
// cannot be driven through the exported (l, slug) surface with more than one
// junction; this file drives the extracted loop directly instead.

package fabricengine

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
)

// wireTestJunction creates a real link at link pointing to target (creating
// target first, since fslink.CreateDirLink does not require it to pre-exist
// but this helper wants a healthy, resolvable junction for the "removable"
// cases below).
func wireTestJunction(t *testing.T, link, target string) {
	t.Helper()

	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("mkdir target %s: %v", target, err)
	}
	if err := fslink.CreateDirLink(link, target); err != nil {
		t.Fatalf("CreateDirLink(%s, %s): %v", link, target, err)
	}
}

// TestUnseedJunctionRecords_AccumulatesBeforeAbort is card 8's regression
// guard for the bug it fixes: when a later junction in the slice fails to
// unwire after an earlier one succeeded, the returned removed slice names the
// earlier one and is not the zero value. This is only exercisable against a
// synthetic multi-record slice — see the file doc comment for why.
func TestUnseedJunctionRecords_AccumulatesBeforeAbort(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	firstLink := filepath.Join(root, "first-link")
	firstTarget := filepath.Join(root, "first-target")
	wireTestJunction(t, firstLink, firstTarget)

	// The second junction's host path is a real, non-link directory —
	// exactly the "not a junction" refusal unseedJunctionRecords must abort
	// on, without having touched anything past the first junction.
	secondLink := filepath.Join(root, "second-link")
	if err := os.MkdirAll(secondLink, 0o755); err != nil {
		t.Fatalf("mkdir real second-link dir: %v", err)
	}

	junctions := []hubgeometry.HostJunction{
		{Name: "first", Link: firstLink, Target: firstTarget},
		{Name: "second", Link: secondLink, Target: filepath.Join(root, "second-target")},
	}

	removed, err := unseedJunctionRecords(junctions)
	if err == nil {
		t.Fatal("unseedJunctionRecords = nil error; want error from the second (real-directory) junction")
	}
	if want := []string{"first"}; !slices.Equal(removed, want) {
		t.Errorf("removed = %v; want %v (the earlier junction, accumulated before the abort)", removed, want)
	}

	// The first junction really is gone; the second (never a junction) is untouched.
	if _, statErr := os.Lstat(firstLink); !os.IsNotExist(statErr) {
		t.Errorf("first junction %s still exists after removal", firstLink)
	}
	if info, statErr := os.Stat(secondLink); statErr != nil || !info.IsDir() {
		t.Errorf("second host dir %s not left untouched: stat err=%v", secondLink, statErr)
	}
}

// TestUnseedJunctionRecords_EmptyIsNoOp asserts that an empty junctions slice
// (matching l.HostJunctions(slug) before any junction has ever been wired) is
// a legitimate no-op: (nil, nil), not an error.
func TestUnseedJunctionRecords_EmptyIsNoOp(t *testing.T) {
	t.Parallel()

	removed, err := unseedJunctionRecords(nil)
	if err != nil {
		t.Fatalf("unseedJunctionRecords(nil) = %v; want nil", err)
	}
	if len(removed) != 0 {
		t.Errorf("removed = %v; want empty", removed)
	}
}

// TestUnseedJunctionRecords_RemovesEveryHealthyJunction asserts the base case
// this card's generalisation must not regress: every junction in the slice
// that is present and healthy is removed, and every removed Name is reported.
func TestUnseedJunctionRecords_RemovesEveryHealthyJunction(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	firstLink := filepath.Join(root, "first-link")
	firstTarget := filepath.Join(root, "first-target")
	wireTestJunction(t, firstLink, firstTarget)

	secondLink := filepath.Join(root, "second-link")
	secondTarget := filepath.Join(root, "second-target")
	wireTestJunction(t, secondLink, secondTarget)

	junctions := []hubgeometry.HostJunction{
		{Name: "first", Link: firstLink, Target: firstTarget},
		{Name: "second", Link: secondLink, Target: secondTarget},
	}

	removed, err := unseedJunctionRecords(junctions)
	if err != nil {
		t.Fatalf("unseedJunctionRecords = %v; want nil", err)
	}
	if want := []string{"first", "second"}; !slices.Equal(removed, want) {
		t.Errorf("removed = %v; want %v", removed, want)
	}

	for _, link := range []string{firstLink, secondLink} {
		if _, statErr := os.Lstat(link); !os.IsNotExist(statErr) {
			t.Errorf("junction %s still exists after removal", link)
		}
	}
}
