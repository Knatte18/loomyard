// webstergeom_test.go holds the unit tests for the webster path accessors Dir, ReportsDir, PromptsDir and ScratchDir.
// The tests need no git repository.
// They pin the scratch split: Dir and ReportsDir, durable and fabric-synced, stay under lymdirs.LymDirName;
// ScratchDir and PromptsDir, never tracked, resolve under lymdirs.DotLymDirName at the same mirrored subpath.

package websterengine

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// TestWebsterGeometryHelpers pins every webster path constructor for each of these anchors:
// an unanchored Location (AnchorRel == "."); a nested AnchorRel, where every accessor moves down by AnchorRel together;
// and a plain told directory not derived from any *lymcwd.Location.
// The nested case holds because both trees share the Cwd Resolution Invariant's per-module anchoring.
// The told-directory case is the property internal/standalonegeom depends on.
func TestWebsterGeometryHelpers(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	baseDir := "/home/user/project"
	unanchored := &lymcwd.Location{HubPath: filepath.Dir(baseDir), WorktreeName: filepath.Base(baseDir), AnchorRel: "."}
	subpathAnchored := &lymcwd.Location{HubPath: filepath.Dir(baseDir), WorktreeName: filepath.Base(baseDir), AnchorRel: "backend"}

	tests := []struct {
		name       string
		anchorRoot string
		// wantAnchorRoot is the anchor the Location must resolve to; an empty value skips the check.
		wantAnchorRoot string
	}{
		{name: "unanchored location", anchorRoot: unanchored.AnchorPath(), wantAnchorRoot: baseDir},
		{name: "subpath-anchored location", anchorRoot: subpathAnchored.AnchorPath()},
		{name: "told directory", anchorRoot: "/var/lib/lym-standalone/state"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.wantAnchorRoot != "" && tt.anchorRoot != tt.wantAnchorRoot {
				t.Fatalf("AnchorPath() = %q; want %q", tt.anchorRoot, tt.wantAnchorRoot)
			}
			accessors := []struct {
				name string
				got  string
				want string
			}{
				{"Dir", Dir(tt.anchorRoot), filepath.Join(tt.anchorRoot, lymdirs.LymDirName, "webster")},
				{"ReportsDir", ReportsDir(tt.anchorRoot), filepath.Join(tt.anchorRoot, lymdirs.LymDirName, "webster", "reports")},
				{"ScratchDir", ScratchDir(tt.anchorRoot), filepath.Join(tt.anchorRoot, lymdirs.DotLymDirName, "webster")},
				{"PromptsDir", PromptsDir(tt.anchorRoot), filepath.Join(tt.anchorRoot, lymdirs.DotLymDirName, "webster", "prompts")},
			}
			for _, a := range accessors {
				if a.got != a.want {
					t.Errorf("%s(%q) = %q; want %q", a.name, tt.anchorRoot, a.got, a.want)
				}
			}
		})
	}

	// The verify directory both tellers fill sits under lymdirs.DotLymDirName.
	// It sits beside webster's scratch directory for a plain told anchor.
	t.Run("verify directory beside scratch", func(t *testing.T) {
		t.Parallel()

		anchorRoot := "/var/lib/lym-standalone/state"
		g := Geometry{ScratchDir: ScratchDir(anchorRoot), VerifyDir: verifytree.Dir(anchorRoot)}

		if got, want := filepath.Dir(g.VerifyDir), filepath.Dir(g.ScratchDir); got != want {
			t.Errorf("filepath.Dir(VerifyDir) = %q; want %q (the directory holding ScratchDir)", got, want)
		}
	})
}
