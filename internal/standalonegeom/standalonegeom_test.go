// standalonegeom_test.go pins every field of both told-mode geometry builders against a fixed
// target directory and told literal stateDir/hash8 values. Nothing here calls
// standalonestate.Derive, reads an environment variable, or touches disk — the whole point of the
// builders' told parameters is that these tests need no t.Setenv and can run t.Parallel().

package standalonegeom

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

func TestReedGeometry(t *testing.T) {
	t.Parallel()

	// target is deliberately not under stateDir, since that is the whole reason
	// AnchorPath and PaneCwd exist as separate fields.
	target := filepath.Join(string(filepath.Separator), "home", "operator", "src", "distinctive-repo-name")
	stateDir := filepath.Join(string(filepath.Separator), "var", "lib", "lyx-state", "abcd1234")
	hash8 := "abcd1234"

	got := ReedGeometry(target, stateDir, hash8)

	if want := "lyx-" + hash8; got.SocketKey != want {
		t.Errorf("ReedGeometry().SocketKey = %q; want %q", got.SocketKey, want)
	}
	if want := "distinctive-repo-name-" + hash8; got.SessionName != want {
		t.Errorf("ReedGeometry().SessionName = %q; want %q", got.SessionName, want)
	}
	if got.AnchorPath != stateDir {
		t.Errorf("ReedGeometry().AnchorPath = %q; want %q (stateDir)", got.AnchorPath, stateDir)
	}
	if got.PaneCwd != target {
		t.Errorf("ReedGeometry().PaneCwd = %q; want %q (target)", got.PaneCwd, target)
	}
	// PaneCwd and AnchorPath asserted in the same case: the whole reason the field
	// exists is that the two differ here, with a fixture target not under stateDir.
	if got.PaneCwd == got.AnchorPath {
		t.Errorf("ReedGeometry().PaneCwd = %q; want != AnchorPath %q", got.PaneCwd, got.AnchorPath)
	}
	if got.WorktreeRoot != target {
		t.Errorf("ReedGeometry().WorktreeRoot = %q; want %q (target)", got.WorktreeRoot, target)
	}
	if want := filepath.Join(stateDir, "logs"); got.LogsDir != want {
		// The wrong answer is one fabricengine.HubLogsDir call away and is a
		// board-shaped path, so this row is explicit rather than derived.
		t.Errorf("ReedGeometry().LogsDir = %q; want %q (stateDir joined with \"logs\")", got.LogsDir, want)
	}
	if want := "distinctive-repo-name"; got.RepoName != want {
		t.Errorf("ReedGeometry().RepoName = %q; want %q", got.RepoName, want)
	}
	if got.HubPath != stateDir {
		t.Errorf("ReedGeometry().HubPath = %q; want %q (stateDir)", got.HubPath, stateDir)
	}
}

func TestWebsterGeometry(t *testing.T) {
	t.Parallel()

	target := filepath.Join(string(filepath.Separator), "home", "operator", "src", "distinctive-repo-name")
	stateDir := filepath.Join(string(filepath.Separator), "var", "lib", "lyx-state", "abcd1234")

	got := WebsterGeometry(target, stateDir)

	if got.AnchorRoot != stateDir {
		t.Errorf("WebsterGeometry().AnchorRoot = %q; want %q (stateDir)", got.AnchorRoot, stateDir)
	}
	if got.WorktreeRoot != target {
		t.Errorf("WebsterGeometry().WorktreeRoot = %q; want %q (target)", got.WorktreeRoot, target)
	}
	// AnchorRoot and WorktreeRoot asserted in the same case, for the same reason
	// as the reed test's PaneCwd/AnchorPath pair.
	if got.AnchorRoot == got.WorktreeRoot {
		t.Errorf("WebsterGeometry().AnchorRoot = %q; want != WorktreeRoot %q", got.AnchorRoot, got.WorktreeRoot)
	}
	if want := websterengine.Dir(stateDir); got.WebsterDir != want {
		t.Errorf("WebsterGeometry().WebsterDir = %q; want %q", got.WebsterDir, want)
	}
	if want := websterengine.ReportsDir(stateDir); got.ReportsDir != want {
		t.Errorf("WebsterGeometry().ReportsDir = %q; want %q", got.ReportsDir, want)
	}
	if want := websterengine.ScratchDir(stateDir); got.ScratchDir != want {
		t.Errorf("WebsterGeometry().ScratchDir = %q; want %q", got.ScratchDir, want)
	}
	if want := websterengine.PromptsDir(stateDir); got.PromptsDir != want {
		t.Errorf("WebsterGeometry().PromptsDir = %q; want %q", got.PromptsDir, want)
	}
	if want := planparser.PlanDir(stateDir); got.PlanDir != want {
		t.Errorf("WebsterGeometry().PlanDir = %q; want %q", got.PlanDir, want)
	}
	if want := filepath.Join(stateDir, lyxdirs.LyxDirName, "stencils"); got.StencilsDir != want {
		t.Errorf("WebsterGeometry().StencilsDir = %q; want %q", got.StencilsDir, want)
	}
}
