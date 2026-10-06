// webstergeom_test.go — pure path-math unit tests for the webster geometry accessors
// (Dir/ReportsDir/PromptsDir/ScratchDir).
// These tests do not require a git repository and run under standard unit test verification.
// They pin the scratch-dir split: Dir and ReportsDir (durable, fabric-synced) stay under
// lyxdirs.LyxDirName, while ScratchDir and PromptsDir (never-tracked) resolve under
// lyxdirs.DotLyxDirName at the same mirrored subpath, for both an unanchored and a
// subpath-anchored *lyxcwd.Location's AnchorPath, and for a plain told directory driven with no
// Location at all.

package websterengine

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// TestWebsterGeometryHelpers pins every webster path constructor for an unanchored Location
// (AnchorRel == "."), for a nested AnchorRel (every accessor moves down by AnchorRel together,
// since both trees share the Cwd Resolution Invariant's per-module anchoring) and for a plain told
// directory that is not derived from any *lyxcwd.Location at all (the property
// internal/standalonegeom depends on).
func TestWebsterGeometryHelpers(t *testing.T) {
	t.Parallel()

	baseDir := "/home/user/project"
	unanchored := &lyxcwd.Location{HubPath: filepath.Dir(baseDir), WorktreeName: filepath.Base(baseDir), AnchorRel: "."}
	subpathAnchored := &lyxcwd.Location{HubPath: filepath.Dir(baseDir), WorktreeName: filepath.Base(baseDir), AnchorRel: "backend"}

	tests := []struct {
		name       string
		anchorRoot string
		// wantAnchorRoot, when set, is the anchor the Location must resolve to.
		wantAnchorRoot string
	}{
		{name: "unanchored location", anchorRoot: unanchored.AnchorPath(), wantAnchorRoot: baseDir},
		{name: "subpath-anchored location", anchorRoot: subpathAnchored.AnchorPath()},
		{name: "told directory", anchorRoot: "/var/lib/lyx-standalone/state"},
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
				{"Dir", Dir(tt.anchorRoot), filepath.Join(tt.anchorRoot, lyxdirs.LyxDirName, "webster")},
				{"ReportsDir", ReportsDir(tt.anchorRoot), filepath.Join(tt.anchorRoot, lyxdirs.LyxDirName, "webster", "reports")},
				{"ScratchDir", ScratchDir(tt.anchorRoot), filepath.Join(tt.anchorRoot, lyxdirs.DotLyxDirName, "webster")},
				{"PromptsDir", PromptsDir(tt.anchorRoot), filepath.Join(tt.anchorRoot, lyxdirs.DotLyxDirName, "webster", "prompts")},
			}
			for _, a := range accessors {
				if a.got != a.want {
					t.Errorf("%s(%q) = %q; want %q", a.name, tt.anchorRoot, a.got, a.want)
				}
			}
		})
	}

	// The verify directory both tellers fill sits under lyxdirs.DotLyxDirName, beside webster's
	// scratch directory, for a plain told anchor.
	t.Run("verify directory beside scratch", func(t *testing.T) {
		t.Parallel()

		anchorRoot := "/var/lib/lyx-standalone/state"
		g := Geometry{ScratchDir: ScratchDir(anchorRoot), VerifyDir: verifytree.Dir(anchorRoot)}

		if got, want := filepath.Dir(g.VerifyDir), filepath.Dir(g.ScratchDir); got != want {
			t.Errorf("filepath.Dir(VerifyDir) = %q; want %q (the directory holding ScratchDir)", got, want)
		}
	})
}
