// reedgeom_symlink_test.go pins ReedGeometry's session name against a real filesystem.
// A symlinked and a real spelling of one target directory must produce one session name:
// they produce one hash8, one socket key and one state directory.
// It needs a real filesystem to create a symlink, but spawns nothing, so it stays in tier 0.

package standalonegeom

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/standalonestate"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestReedGeometry_SessionNameAgreesAcrossSymlinkedSpellings pins that both spellings get one session name.
// standalonestate.Derive resolves symlinks first, so both spellings share one hash8, socket key and state directory.
// ReedGeometry builds SessionName from the resolved target, not the path as typed.
//
// The test first asserts that hash8 from the real Derive is equal for both spellings.
// The session-name assertion means nothing unless the identity halves already agree.
//
//testtiming:keep pins that a symlinked and a real spelling of one target share one session name on a real filesystem, which the tmux naming test covering its blocks never builds
func TestReedGeometry_SessionNameAgreesAcrossSymlinkedSpellings(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	base := t.TempDir()
	realDir := filepath.Join(base, "distinctive-repo-name")
	if err := os.Mkdir(realDir, 0o755); err != nil {
		t.Fatalf("os.Mkdir(%q) error = %v", realDir, err)
	}

	linkPath := filepath.Join(base, "link-to-repo")
	if err := os.Symlink(realDir, linkPath); err != nil {
		if os.IsPermission(err) {
			t.Skip("os.Symlink not permitted on this host; skipping")
		}
		t.Fatalf("os.Symlink(%q, %q) error = %v", realDir, linkPath, err)
	}

	realStateDir, realHash8, err := standalonestate.Derive(realDir)
	if err != nil {
		t.Fatalf("standalonestate.Derive(%q) = %v; want nil error", realDir, err)
	}
	linkStateDir, linkHash8, err := standalonestate.Derive(linkPath)
	if err != nil {
		t.Fatalf("standalonestate.Derive(%q) = %v; want nil error", linkPath, err)
	}
	if realHash8 != linkHash8 || realStateDir != linkStateDir {
		t.Fatalf("Derive disagreed across spellings: real = (%q, %q), symlink = (%q, %q); fixture invalid",
			realStateDir, realHash8, linkStateDir, linkHash8)
	}

	realGeom := ReedGeometry(realDir, realStateDir, realHash8)
	linkGeom := ReedGeometry(linkPath, linkStateDir, linkHash8)

	if realGeom.SocketKey != linkGeom.SocketKey {
		t.Fatalf("ReedGeometry().SocketKey real = %q, symlink = %q; want equal (fixture invalid)", realGeom.SocketKey, linkGeom.SocketKey)
	}
	if realGeom.SessionName != linkGeom.SessionName {
		t.Errorf("ReedGeometry().SessionName real = %q, symlink = %q; want equal -- two session names on the one socket %q share one reed.json",
			realGeom.SessionName, linkGeom.SessionName, realGeom.SocketKey)
	}
	if want := "distinctive-repo-name-" + realHash8; linkGeom.SessionName != want {
		t.Errorf("ReedGeometry(symlink).SessionName = %q; want %q (the resolved directory's own name)", linkGeom.SessionName, want)
	}

	// PaneCwd and WorktreeRoot name a directory rather than an identity, so they keep the path as told.
	// Only the session name is resolved; the path the operator typed is not rewritten.
	if linkGeom.PaneCwd != linkPath {
		t.Errorf("ReedGeometry(symlink).PaneCwd = %q; want %q (told verbatim)", linkGeom.PaneCwd, linkPath)
	}
	if linkGeom.WorktreeRoot != linkPath {
		t.Errorf("ReedGeometry(symlink).WorktreeRoot = %q; want %q (told verbatim)", linkGeom.WorktreeRoot, linkPath)
	}
}
