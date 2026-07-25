// add_test.go — unit tests for Add's slug validation. Validation runs before
// any git operation, so these tests need no git fixture and stay untagged
// Tier-1 (no spawn).

package fabricengine_test

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
)

// TestAdd_RejectsSeparatorSlug asserts that Add refuses a slug containing a
// path separator before touching git or the filesystem. A slug is by contract
// a single path component: consumers re-derive it via filepath.Base, so a
// separator-containing slug would create a pair the module cannot re-identify
// (pairs would report it broken, reconcile would misattribute it, prune could
// never see it).
func TestAdd_RejectsSeparatorSlug(t *testing.T) {
	tests := []struct {
		name string
		slug string
	}{
		{"ForwardSlash", "nested/slug"},
		{"Backslash", `nested\slug`},
		{"LeadingSlash", "/slug"},
		{"TrailingSlash", "slug/"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			topology := fabricengine.NewTopology(fabricengine.Config{})
			// The layout points at a non-repo temp dir: validation must
			// reject the slug before Add ever consults the layout, so no
			// git error can mask the validation error.
			layout := &hubgeometry.Layout{WorktreeRoot: t.TempDir()}

			_, err := topology.Add(layout, tt.slug, fabricengine.AddOptions{})
			if err == nil {
				t.Fatalf("Add(%q) error = nil; want invalid-slug error", tt.slug)
			}
			if !strings.Contains(err.Error(), "invalid slug") {
				t.Errorf("Add(%q) error = %v; want error containing %q", tt.slug, err, "invalid slug")
			}
		})
	}
}

// TestAdd_RejectsEmptySlug asserts that Add refuses an empty or whitespace-only
// slug before touching git or the filesystem. An empty slug has no name for the
// pair and would otherwise fail deep in step 4 with a misleading "worktree
// directory <HUB> already exists" (l.WorktreePath("") is the hub root).
// Validation runs before any git op, so this stays untagged Tier-1.
func TestAdd_RejectsEmptySlug(t *testing.T) {
	tests := []struct {
		name string
		slug string
	}{
		{"Empty", ""},
		{"Whitespace", "   "},
		{"Tab", "\t"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			topology := fabricengine.NewTopology(fabricengine.Config{})
			layout := &hubgeometry.Layout{WorktreeRoot: t.TempDir()}

			_, err := topology.Add(layout, tt.slug, fabricengine.AddOptions{})
			if err == nil {
				t.Fatalf("Add(%q) error = nil; want invalid-slug error", tt.slug)
			}
			if !strings.Contains(err.Error(), "invalid slug") {
				t.Errorf("Add(%q) error = %v; want error containing %q", tt.slug, err, "invalid slug")
			}
		})
	}
}

// TestAdd_RejectsWeftSuffixSlug asserts that Add refuses a slug ending in the
// weft suffix before touching git or the filesystem. Such a slug names a host
// worktree directory (l.WorktreePath(slug)) that is indistinguishable from a
// weft worktree directory: hubgeometry.WeftHostSlug accepts it, so prune's hub
// scan misclassifies the host worktree as an orphaned weft and — under
// --apply — os.RemoveAll's it, destroying the host worktree and any uncommitted
// work. Rejecting the collision at the source is fabric's job (it owns the weft
// suffix namespace). Validation runs before any git op, so this stays untagged
// Tier-1.
func TestAdd_RejectsWeftSuffixSlug(t *testing.T) {
	tests := []struct {
		name string
		slug string
	}{
		{"PlainWeftSuffix", "zed" + hubgeometry.WeftSuffix},
		{"BareWeftSuffix", hubgeometry.WeftSuffix},
		{"NestedLookingWeftSuffix", "feature" + hubgeometry.WeftSuffix},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			topology := fabricengine.NewTopology(fabricengine.Config{})
			// The layout points at a non-repo temp dir: validation must reject
			// the slug before Add consults the layout, so no git or stat error
			// can mask the validation error.
			layout := &hubgeometry.Layout{WorktreeRoot: t.TempDir()}

			_, err := topology.Add(layout, tt.slug, fabricengine.AddOptions{})
			if err == nil {
				t.Fatalf("Add(%q) error = nil; want invalid-slug error", tt.slug)
			}
			if !strings.Contains(err.Error(), "invalid slug") {
				t.Errorf("Add(%q) error = %v; want error containing %q", tt.slug, err, "invalid slug")
			}
		})
	}
}
