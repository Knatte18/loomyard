// add_test.go — unit tests for Add's slug validation. Validation runs before
// any git operation, so these tests need no git fixture and stay untagged
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

// TestAdd_RejectsEmptySlug asserts that Add refuses an empty or whitespace-only
// slug before touching git or the filesystem. An empty slug has no name for the
// pair and would otherwise fail deep in step 4 with a misleading "worktree
// directory <HUB> already exists" (fabricengine.WorktreePath(l, "") is the hub root).
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

// TestAdd_RejectsWeftSuffixSlug asserts that Add refuses a slug ending in the
// weft suffix before touching git or the filesystem. Such a slug names a host
// worktree directory (fabricengine.WorktreePath(l, slug)) that is indistinguishable from a
// weft worktree directory: lyxcwd.WeftHostSlug accepts it, so prune's hub
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

// TestAdd_RejectsReservedHubNameSlug asserts that Add refuses a slug naming a
// reserved hub-level geometry entry before touching git or the filesystem. A
// host worktree directory named after a geometry token collides with the paths
// lyx composes at the hub level — a "_portals" worktree on a fresh hub would
// have portal junctions created inside it, and a hub-level "_lyx" worktree
// shadows the config-dir token. Validation runs before any git op, so this
// stays untagged Tier-1.
func TestAdd_RejectsReservedHubNameSlug(t *testing.T) {
	tests := []struct {
		name string
		slug string
	}{
		{"LyxDir", "_lyx"},
		{"RaddleDir", "_raddle"},
		{"BoardDir", "_board"},
		{"PortalsDir", "_portals"},
		{"LaunchersDir", "_launchers"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Config{Pathspec: "_lyx _pattern"} injects the junction-name half
			// of the reserved union: after card 1 removed _lyx/_pattern from
			// lyxcwd.HubReservedNames(), those two are rejected only via
			// this injected pathspec, while _board/_portals/_launchers/_raddle
			// stay rejected via HubReservedNames() regardless of pathspec.
			topology := fabricengine.NewTopology(fabricengine.Config{Pathspec: "_lyx _pattern"})
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

// TestAdd_RejectsPathspecJunctionNameSlug asserts that Add refuses a slug
// equal to a current pathspec junction name that is NOT one of
// lyxcwd.HubReservedNames()'s hub-structural tokens — proving the
// config-driven arm of IsReservedHubName's union, not only the hub-structural
// arm TestAdd_RejectsReservedHubNameSlug already covers. "_extra" here is
// reserved only because it is in this Topology's configured pathspec.
// Validation runs before any git op, so this stays untagged Tier-1.
func TestAdd_RejectsPathspecJunctionNameSlug(t *testing.T) {
	topology := fabricengine.NewTopology(fabricengine.Config{Pathspec: "_lyx _pattern _extra"})
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
