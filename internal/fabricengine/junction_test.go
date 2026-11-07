// junction_test.go unit-tests unseedJunctionRecords directly against synthetic WarpJunction slices.
// It has no build tag, since it touches only plain directories and fslink, never git.
// The synthetic slices let a test set up any number of junctions.

package fabricengine

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// wireTestJunction creates a real link at link that points to target, creating target first.
// fslink.CreateDirLink does not need target to exist.
// The helper creates it so the junction resolves and tests can remove it as a healthy junction.
func wireTestJunction(t *testing.T, link, target string) {
	t.Helper()

	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("mkdir target %s: %v", target, err)
	}
	if err := fslink.CreateDirLink(link, target); err != nil {
		t.Fatalf("CreateDirLink(%s, %s): %v", link, target, err)
	}
}

// TestWorktreePath verifies that WorktreePath(l, slug) joins HubPath and slug.
//
//testtiming:keep WorktreePath joining the hub path and slug; coverage of its blocks by other tests does not show an assertion of this
func TestWorktreePath(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	l := &lymcwd.Location{HubPath: filepath.Join("home", "user", "project-LYMHUB")}
	slug := "test-wt"

	got := WorktreePath(l, slug)
	want := filepath.Join(l.HubPath, slug)
	if got != want {
		t.Errorf("WorktreePath(l, %q) = %q; want %q", slug, got, want)
	}
}

// TestUnseedJunctionRecords_AccumulatesBeforeAbort checks that removals survive a later unwire failure.
// The returned removed slice names the earlier junction, which was removed before the abort.
// The case needs a synthetic slice with more than one record.
func TestUnseedJunctionRecords_AccumulatesBeforeAbort(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	root := t.TempDir()

	firstLink := filepath.Join(root, "first-link")
	firstTarget := filepath.Join(root, "first-target")
	wireTestJunction(t, firstLink, firstTarget)

	// The second junction's warp path is a real, non-link directory.
	// unseedJunctionRecords must refuse it as not a junction and abort there.
	// It must not touch anything past the first junction.
	secondLink := filepath.Join(root, "second-link")
	if err := os.MkdirAll(secondLink, 0o755); err != nil {
		t.Fatalf("mkdir real second-link dir: %v", err)
	}

	junctions := []WarpJunction{
		{Name: "first", Link: firstLink, Target: firstTarget},
		{Name: "second", Link: secondLink, Target: filepath.Join(root, "second-target")},
	}

	removed, err := unseedJunctionRecords(NewMutations(""), root, junctions)
	if err == nil {
		t.Fatal("unseedJunctionRecords = nil error; want error from the second (real-directory) junction")
	}
	if want := []string{"first"}; !slices.Equal(removed, want) {
		t.Errorf("removed = %v; want %v (the earlier junction, accumulated before the abort)", removed, want)
	}

	// The first junction is gone; the second, never a junction, is untouched.
	if _, statErr := os.Lstat(firstLink); !os.IsNotExist(statErr) {
		t.Errorf("first junction %s still exists after removal", firstLink)
	}
	if info, statErr := os.Stat(secondLink); statErr != nil || !info.IsDir() {
		t.Errorf("second warp dir %s not left untouched: stat err=%v", secondLink, statErr)
	}
}

// TestUnseedJunctionRecords_RemovesEveryHealthyJunction asserts that every healthy junction in the slice is removed.
// It also asserts that every removed Name is reported.
// An empty junctions slice, the state before any junction is wired, is a no-op: (nil, nil), not an error.
//
//testtiming:keep every healthy junction being removed with each removed name reported, and an empty slice being a (nil, nil) no-op; coverage of its blocks by other tests does not show an assertion of this
func TestUnseedJunctionRecords_RemovesEveryHealthyJunction(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	root := t.TempDir()

	emptyRemoved, err := unseedJunctionRecords(NewMutations(""), root, nil)
	if err != nil {
		t.Fatalf("unseedJunctionRecords(root, nil) = %v; want nil", err)
	}
	if len(emptyRemoved) != 0 {
		t.Errorf("removed = %v; want empty", emptyRemoved)
	}

	firstLink := filepath.Join(root, "first-link")
	firstTarget := filepath.Join(root, "first-target")
	wireTestJunction(t, firstLink, firstTarget)

	secondLink := filepath.Join(root, "second-link")
	secondTarget := filepath.Join(root, "second-target")
	wireTestJunction(t, secondLink, secondTarget)

	junctions := []WarpJunction{
		{Name: "first", Link: firstLink, Target: firstTarget},
		{Name: "second", Link: secondLink, Target: secondTarget},
	}

	removed, err := unseedJunctionRecords(NewMutations(""), root, junctions)
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

// TestExcludePatternFor_AnchorsToRepoRoot pins the anchored git-exclude pattern in Tier 1.
// A bare name matches at every depth in gitignore semantics,
// so the pattern must be slash-anchored at the repo root and carry the anchor subpath.
// The integration-tagged exclude tests prove the on-disk file, but an untagged run never compiles them.
//
//testtiming:keep the git-exclude pattern being slash-anchored at the repo root and carrying the anchor subpath, in the untagged tier; coverage of its blocks by other tests does not show an assertion of this
func TestExcludePatternFor_AnchorsToRepoRoot(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name      string
		anchorRel string
		junction  string
		want      string
	}{
		{"RootAnchor", ".", "_lym", "/_lym"},
		{"SubpathAnchor", "backend", "_lym", "/backend/_lym"},
		{"SubpathAnchorDotLym", "backend", ".lym", "/backend/.lym"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := excludePatternFor(tt.anchorRel, tt.junction); got != tt.want {
				t.Errorf("excludePatternFor(%q, %q) = %q; want %q", tt.anchorRel, tt.junction, got, tt.want)
			}
		})
	}
}
