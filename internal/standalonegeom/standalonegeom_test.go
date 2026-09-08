// standalonegeom_test.go pins every field of both told-mode geometry builders against a fixed
// target directory and told literal stateDir/hash8 values. Nothing here calls
// standalonestate.Derive or reads an environment variable — the whole point of the builders' told
// parameters is that these tests need no t.Setenv and can run t.Parallel().
// Every target below is a fictional absolute path that does not exist, so ReedGeometry's one
// filesystem read (standalonestate.Normalize, see doc.go) resolves each of them to itself and these
// cases stay deterministic with no fixture; the symlink behaviour that read exists for is pinned
// separately in reedgeom_symlink_integration_test.go, which needs a real filesystem.

package standalonegeom

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

func TestBurlerGeometry(t *testing.T) {
	t.Parallel()

	// target is deliberately not under stateDir, since that is the whole reason
	// WorktreeRoot and AnchorPath exist as separate fields.
	target := filepath.Join(string(filepath.Separator), "home", "operator", "src", "distinctive-repo-name")
	stateDir := filepath.Join(string(filepath.Separator), "var", "lib", "lyx-state", "abcd1234")

	got := BurlerGeometry(target, stateDir)

	if got.WorktreeRoot != target {
		t.Errorf("BurlerGeometry().WorktreeRoot = %q; want %q (target)", got.WorktreeRoot, target)
	}
	if got.AnchorPath != stateDir {
		t.Errorf("BurlerGeometry().AnchorPath = %q; want %q (stateDir)", got.AnchorPath, stateDir)
	}
	// WorktreeRoot and AnchorPath asserted in the same case: the whole reason the two-root
	// split exists is that they differ here, with a fixture target not under stateDir.
	if got.WorktreeRoot == got.AnchorPath {
		t.Errorf("BurlerGeometry().WorktreeRoot = %q; want != AnchorPath %q", got.WorktreeRoot, got.AnchorPath)
	}
}

func TestStencilsDir(t *testing.T) {
	t.Parallel()

	stateDir := filepath.Join(string(filepath.Separator), "var", "lib", "lyx-state", "abcd1234")

	got := StencilsDir(stateDir)

	if want := filepath.Join(stateDir, lyxdirs.LyxDirName, "stencils"); got != want {
		t.Errorf("StencilsDir(%q) = %q; want %q", stateDir, got, want)
	}
}

func TestLogsDir(t *testing.T) {
	t.Parallel()

	stateDir := filepath.Join(string(filepath.Separator), "var", "lib", "lyx-state", "abcd1234")

	got := LogsDir(stateDir)

	if want := filepath.Join(stateDir, lyxdirs.DotLyxDirName, "logs"); got != want {
		t.Errorf("LogsDir(%q) = %q; want %q", stateDir, got, want)
	}

	// LogsDir and ReedGeometry's LogsDir field are deliberately different directories for
	// different producers -- pin the non-convergence the doc comment records.
	target := filepath.Join(string(filepath.Separator), "home", "operator", "src", "distinctive-repo-name")
	hash8 := "abcd1234"
	if reedLogsDir := ReedGeometry(target, stateDir, hash8).LogsDir; got == reedLogsDir {
		t.Errorf("LogsDir(%q) = %q; want != ReedGeometry(...).LogsDir %q", stateDir, got, reedLogsDir)
	}
}

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

// TestReedGeometry_SessionNameSanitizesTheReadableHalf is the regression guard for the R4 review's
// R4-10: SessionName was built from a RAW filepath.Base(target), and reedengine's
// validateToldTmuxIdentity refuses a session name carrying '.', ':', '\', a control character, or an
// invalid UTF-8 byte — so standalone mode, whose whole premise is "point it at any plain checkout
// the operator already has", died on the routine repository name "my.repo" with advice to rename the
// operator's own directory.
// The hash8 suffix is asserted intact on every row, since sanitizing the readable half must never
// touch the half that carries the identity.
func TestReedGeometry_SessionNameSanitizesTheReadableHalf(t *testing.T) {
	t.Parallel()

	stateDir := filepath.Join(string(filepath.Separator), "var", "lib", "lyx-state", "abcd1234")
	hash8 := "abcd1234"

	cases := []struct {
		name     string
		basename string
		want     string
	}{
		{"dotted repository name", "my.repo", "my_repo-" + hash8},
		{"dotted extension-shaped name", "site.com", "site_com-" + hash8},
		{"colon", "svc:v2", "svc_v2-" + hash8},
		{"control character", "svc\tv3", "svc_v3-" + hash8},
		{"plain name passes through unchanged", "distinctive-repo-name", "distinctive-repo-name-" + hash8},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			target := filepath.Join(string(filepath.Separator), "home", "operator", "src", c.basename)

			got := ReedGeometry(target, stateDir, hash8)

			if got.SessionName != c.want {
				t.Errorf("ReedGeometry(%q).SessionName = %q; want %q", target, got.SessionName, c.want)
			}
			// The sanitizer is reedengine's own, never re-implemented here — the rule belongs to
			// the package that also refuses violations of it.
			if want := reedengine.SanitizeSessionName(c.basename) + "-" + hash8; got.SessionName != want {
				t.Errorf("ReedGeometry(%q).SessionName = %q; want %q (reedengine.SanitizeSessionName + hash8)", target, got.SessionName, want)
			}
			// RepoName is the header pane's display token, not a tmux target, so it stays raw.
			if got.RepoName != c.basename {
				t.Errorf("ReedGeometry(%q).RepoName = %q; want %q (raw basename)", target, got.RepoName, c.basename)
			}
		})
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
	// This equality pins the one-construction-site property: WebsterGeometry's StencilsDir
	// field must always come from the shared StencilsDir helper, never a re-derived literal.
	if want := StencilsDir(stateDir); got.StencilsDir != want {
		t.Errorf("WebsterGeometry().StencilsDir = %q; want %q (StencilsDir(stateDir))", got.StencilsDir, want)
	}
}
