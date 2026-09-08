//go:build integration

// reedgeom_symlink_integration_test.go pins ReedGeometry's session name against a real filesystem: a
// symlinked and a real spelling of one target directory must produce ONE session name, because they
// already produce one hash8, one socket key, and one state directory.
// It needs a real filesystem to create a symlink, so it is tagged integration to keep tier 1 clean,
// exactly as internal/standalonestate's symlink_integration_test.go is, even though a symlink is a
// filesystem operation rather than a git spawn.
// This package spawns no git and builds no hubforge/gitkit fixture, so it needs no TestMain.

package standalonegeom

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/standalonestate"
)

// TestReedGeometry_SessionNameAgreesAcrossSymlinkedSpellings is the regression guard for the R4
// review's R4-24: standalonestate.Derive runs filepath.EvalSymlinks before hashing, so a symlinked
// and a real spelling of one repository share one hash8, one socket key and one state directory --
// but ReedGeometry built SessionName from the UN-normalized target, so the two spellings produced
// two different tmux session names on the same socket, sharing one reed.json. reed's foreign-session
// guard caught the collision loudly, with advice that did not fit the situation.
//
// hash8 is derived through the real Derive for both spellings and asserted equal first, since the
// session-name assertion below means nothing if the identity halves already disagree.
func TestReedGeometry_SessionNameAgreesAcrossSymlinkedSpellings(t *testing.T) {
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

	// PaneCwd and WorktreeRoot name a directory rather than an identity, and both spellings reach
	// the same one, so they stay exactly as told -- pinned here so the fix is not quietly widened
	// into rewriting the path the operator typed.
	if linkGeom.PaneCwd != linkPath {
		t.Errorf("ReedGeometry(symlink).PaneCwd = %q; want %q (told verbatim)", linkGeom.PaneCwd, linkPath)
	}
	if linkGeom.WorktreeRoot != linkPath {
		t.Errorf("ReedGeometry(symlink).WorktreeRoot = %q; want %q (told verbatim)", linkGeom.WorktreeRoot, linkPath)
	}
}
