// add_test.go — unit tests for Add's slug validation.
// Validation runs before any git operation, so these tests need no git fixture and stay untagged
// Tier-1 (no spawn).

package fabricengine_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/weftname"
)

// TestAdd_RejectsSeparatorSlug asserts that Add refuses a slug containing a path separator before
// touching git or the filesystem.
// A slug is by contract a single path component: consumers re-derive it via filepath.Base, so a
// separator-containing slug would create a pair the module cannot re-identify (pairs would report
// it broken, reconcile would misattribute it, prune could never see it).
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
			layout := &lyxcwd.Location{HubPath: filepath.Dir(t.TempDir()), WorktreeName: filepath.Base(t.TempDir())}

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

// TestAdd_RejectsEmptySlug asserts that Add refuses an empty or whitespace-only slug before
// touching git or the filesystem.
// An empty slug has no name for the pair and would otherwise fail deep in step 4 with a misleading
// "worktree directory <HUB> already exists" (fabricengine.WorktreePath(l, "") is the hub root).
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
			layout := &lyxcwd.Location{HubPath: filepath.Dir(t.TempDir()), WorktreeName: filepath.Base(t.TempDir())}

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

// TestAdd_RejectsWeftSuffixSlug asserts that Add refuses a slug ending in the weft suffix before
// touching git or the filesystem.
// Such a slug names a host worktree directory (fabricengine.WorktreePath(l, slug)) that is
// indistinguishable from a weft worktree directory: lyxcwd.WeftHostSlug accepts it, so prune's hub
// scan misclassifies the host worktree as an orphaned weft and — under --apply — os.RemoveAll's it,
// destroying the host worktree and any uncommitted work.
// Rejecting the collision at the source is fabric's job (it owns the weft suffix namespace).
// Validation runs before any git op, so this stays untagged Tier-1.
func TestAdd_RejectsWeftSuffixSlug(t *testing.T) {
	tests := []struct {
		name string
		slug string
	}{
		{"PlainWeftSuffix", "zed" + weftname.Suffix},
		{"BareWeftSuffix", weftname.Suffix},
		{"NestedLookingWeftSuffix", "feature" + weftname.Suffix},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			topology := fabricengine.NewTopology(fabricengine.Config{})
			// The layout points at a non-repo temp dir: validation must reject
			// the slug before Add consults the layout, so no git or stat error
			// can mask the validation error.
			layout := &lyxcwd.Location{HubPath: filepath.Dir(t.TempDir()), WorktreeName: filepath.Base(t.TempDir())}

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

// TestAdd_RejectsReservedHubNameSlug asserts that Add refuses a slug naming a reserved hub-level
// geometry entry before touching git or the filesystem.
// A host worktree directory named after a geometry token collides with the paths lyx composes at
// the hub level — a "_portals" worktree on a fresh hub would have portal junctions created inside
// it,
// and a hub-level "_lyx" worktree shadows the config-dir token.
// Validation runs before any git op, so this stays untagged Tier-1.
func TestAdd_RejectsReservedHubNameSlug(t *testing.T) {
	tests := []struct {
		name string
		slug string
	}{
		{"LyxDir", "_lyx"},
		{"BoardDir", "_board"},
		{"PortalsDir", "_portals"},
		{"LaunchersDir", "_launchers"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Config{Pathspec: "_lyx _extra"} injects the junction-name half of
			// the reserved union, but neither of its rejections below actually
			// depends on that injection: _lyx is reserved via IsReservedHubName's
			// own structuralCommittedDirs union, never via the injected pathspec
			// — that was true even before this task — and _board/_portals/
			// _launchers are reserved via fabricengine.HubReservedNames()
			// regardless of pathspec.
			// _raddle dropped out of this table entirely: HubReservedNames() no
			// longer reserves it, so it is now an accepted slug (see
			// TestAdd_AcceptsRaddleSlug below).
			topology := fabricengine.NewTopology(fabricengine.Config{Pathspec: "_lyx _extra"})
			layout := &lyxcwd.Location{HubPath: filepath.Dir(t.TempDir()), WorktreeName: filepath.Base(t.TempDir())}

			_, err := topology.Add(layout, tt.slug, fabricengine.AddOptions{})
			if err == nil {
				t.Fatalf("Add(%q) error = nil; want invalid-slug error", tt.slug)
			}
			if !strings.Contains(err.Error(), "invalid slug") {
				t.Errorf("Add(%q) error = %v; want error containing %q", tt.slug, err, "invalid slug")
			}
			if !strings.Contains(err.Error(), "reserved for lyx hub geometry") {
				t.Errorf("Add(%q) error = %v; want error containing %q", tt.slug, err, "reserved for lyx hub geometry")
			}
		})
	}
}

// TestAdd_AcceptsRaddleSlug asserts that Add no longer refuses a "_raddle" slug on reserved-name
// grounds: HubReservedNames() dropped "_raddle" (raddle has converged on an anchor-level
// `_lyx/raddle/` design with no hub-level presence), so the slug-validation error this test's sibling
// rejection tests assert on must not appear for "_raddle" — whatever error Add does return past
// validation (the layout here points at a non-repo temp dir, so a later git-related failure is
// expected) must not be the reserved-hub-name one.
func TestAdd_AcceptsRaddleSlug(t *testing.T) {
	topology := fabricengine.NewTopology(fabricengine.Config{Pathspec: "_lyx _extra"})
	layout := &lyxcwd.Location{HubPath: filepath.Dir(t.TempDir()), WorktreeName: filepath.Base(t.TempDir())}

	_, err := topology.Add(layout, "_raddle", fabricengine.AddOptions{})
	if err != nil && strings.Contains(err.Error(), "reserved for lyx hub geometry") {
		t.Errorf("Add(%q) error = %v; want no reserved-hub-name rejection", "_raddle", err)
	}
}

// TestAdd_RejectsPathspecJunctionNameSlug asserts that Add refuses a slug equal to a current
// pathspec junction name that is NOT one of fabricengine.HubReservedNames()'s hub-structural tokens
// — proving the config-driven arm of IsReservedHubName's union, not only the hub-structural arm
// TestAdd_RejectsReservedHubNameSlug already covers. "_extra" here is reserved only because it is
// in this Topology's configured pathspec.
// Validation runs before any git op, so this stays untagged Tier-1.
func TestAdd_RejectsPathspecJunctionNameSlug(t *testing.T) {
	topology := fabricengine.NewTopology(fabricengine.Config{Pathspec: "_lyx _extra _other"})
	layout := &lyxcwd.Location{HubPath: filepath.Dir(t.TempDir()), WorktreeName: filepath.Base(t.TempDir())}

	_, err := topology.Add(layout, "_extra", fabricengine.AddOptions{})
	if err == nil {
		t.Fatalf("Add(%q) error = nil; want invalid-slug error", "_extra")
	}
	if !strings.Contains(err.Error(), "invalid slug") {
		t.Errorf("Add(%q) error = %v; want error containing %q", "_extra", err, "invalid slug")
	}
	if !strings.Contains(err.Error(), "reserved for lyx hub geometry") {
		t.Errorf("Add(%q) error = %v; want error containing %q", "_extra", err, "reserved for lyx hub geometry")
	}
}

// TestAdd_RejectsDotLyxSlug asserts that Add refuses a slug of ".lyx" even for a Config naming
// neither structural directory: the refusal must come from IsReservedHubName's
// structuralNeverCommittedDirs union, not from any pathspec the caller happens to configure. A
// worktree slug of ".lyx" would collide with the hub-level "<hub>/.lyx" batch 8 recognises.
// Validation runs before any git op, so this stays untagged Tier-1.
func TestAdd_RejectsDotLyxSlug(t *testing.T) {
	topology := fabricengine.NewTopology(fabricengine.Config{})
	layout := &lyxcwd.Location{HubPath: filepath.Dir(t.TempDir()), WorktreeName: filepath.Base(t.TempDir())}

	_, err := topology.Add(layout, ".lyx", fabricengine.AddOptions{})
	if err == nil {
		t.Fatalf("Add(%q) error = nil; want invalid-slug error", ".lyx")
	}
	if !strings.Contains(err.Error(), "invalid slug") {
		t.Errorf("Add(%q) error = %v; want error containing %q", ".lyx", err, "invalid slug")
	}
	if !strings.Contains(err.Error(), "reserved for lyx hub geometry") {
		t.Errorf("Add(%q) error = %v; want error containing %q", ".lyx", err, "reserved for lyx hub geometry")
	}
}
