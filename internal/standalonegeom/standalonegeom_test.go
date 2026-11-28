// standalonegeom_test.go pins every field of both told-mode geometry builders.
// They take a fixed target directory and literal stateDir and hash8 values.
// Nothing here calls standalonestate.Derive or reads an environment variable;
// the tests therefore need no t.Setenv and run t.Parallel().
// Every target below is a fictional absolute path that does not exist,
// so the one filesystem read in ReedGeometry (standalonestate.Normalize) resolves each of them to itself.
// The cases stay deterministic with no fixture.
// The symlink behaviour is pinned in reedgeom_symlink_test.go, which needs a real filesystem.

package standalonegeom

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/verifytree"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

func TestBurlerGeometry(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	// target is not under stateDir, so WorktreeRoot and AnchorPath resolve to different roots.
	target := filepath.Join(string(filepath.Separator), "home", "operator", "src", "distinctive-repo-name")
	stateDir := filepath.Join(string(filepath.Separator), "var", "lib", "lym-state", "abcd1234")

	hash8 := "abcd1234"

	got := BurlerGeometry(target, stateDir, hash8)

	if want := agentname.StandaloneShortname(hash8); got.Shortname != want || got.Slug != "" {
		t.Errorf("BurlerGeometry().Shortname/Slug = %q/%q; want %q and an empty slug", got.Shortname, got.Slug, want)
	}
	if got.WorktreeRoot != target {
		t.Errorf("BurlerGeometry().WorktreeRoot = %q; want %q (target)", got.WorktreeRoot, target)
	}
	if got.AnchorPath != stateDir {
		t.Errorf("BurlerGeometry().AnchorPath = %q; want %q (stateDir)", got.AnchorPath, stateDir)
	}
	if got.RepoRoot != target {
		t.Errorf("BurlerGeometry().RepoRoot = %q; want %q (target)", got.RepoRoot, target)
	}
	if want := SpecsDir(stateDir); got.SpecsDir != want {
		t.Errorf("BurlerGeometry().SpecsDir = %q; want %q", got.SpecsDir, want)
	}
	// WorktreeRoot and AnchorPath are asserted to differ in the same case, since the fixture target is not under stateDir.
	if got.WorktreeRoot == got.AnchorPath {
		t.Errorf("BurlerGeometry().WorktreeRoot = %q; want != AnchorPath %q", got.WorktreeRoot, got.AnchorPath)
	}
}

// TestStandaloneDirs pins each told-stateDir directory builder to its own subdirectory of stateDir.
// No two of them converge: StencilsDir and SpecsDir are separate directories under the durable lym directory.
// LogsDir is a third one, under the ephemeral directory.
func TestStandaloneDirs(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	stateDir := filepath.Join(string(filepath.Separator), "var", "lib", "lym-state", "abcd1234")

	cases := []struct {
		name string
		got  string
		want string
	}{
		{"StencilsDir", StencilsDir(stateDir), filepath.Join(stateDir, lymdirs.LymDirName, "stencils")},
		{"SpecsDir", SpecsDir(stateDir), filepath.Join(stateDir, lymdirs.LymDirName, "specs")},
		{"LogsDir", LogsDir(stateDir), filepath.Join(stateDir, lymdirs.DotLymDirName, "logs")},
		{"PauseGeometry", PauseGeometry(stateDir).RecordPath, filepath.Join(stateDir, lymdirs.DotLymDirName, lymdirs.PauseDirName, pauserecord.RecordFileName)},
	}
	if slug := PauseGeometry(stateDir).Slug; slug != "" {
		t.Errorf("PauseGeometry(%q).Slug = %q; want empty", stateDir, slug)
	}
	seen := map[string]string{}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			if c.got != c.want {
				t.Errorf("%s(%q) = %q; want %q", c.name, stateDir, c.got, c.want)
			}
		})
		if other, ok := seen[c.got]; ok {
			t.Errorf("%s(%q) = %q; want != %s's result", c.name, stateDir, c.got, other)
		}
		seen[c.got] = c.name
	}

	reedLogsDir := ReedGeometry(filepath.Join(string(filepath.Separator), "home", "operator", "src", "distinctive-repo-name"), stateDir, "abcd1234").LogsDir
	if reedLogsDir == LogsDir(stateDir) {
		t.Errorf("LogsDir(%q) = %q; want != ReedGeometry(...).LogsDir %q", stateDir, LogsDir(stateDir), reedLogsDir)
	}
}

//testtiming:keep pins every ReedGeometry field at once, including the two-root split and the identity fields, which the session-name table covering its blocks never reads
func TestReedGeometry(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	// target is not under stateDir, so AnchorPath and PaneCwd resolve to different paths.
	target := filepath.Join(string(filepath.Separator), "home", "operator", "src", "distinctive-repo-name")
	stateDir := filepath.Join(string(filepath.Separator), "var", "lib", "lym-state", "abcd1234")
	hash8 := "abcd1234"

	got := ReedGeometry(target, stateDir, hash8)

	if want := agentname.StandaloneShortname(hash8); got.NameShortname != want {
		t.Errorf("ReedGeometry().NameShortname = %q; want %q", got.NameShortname, want)
	}
	if err := agentname.ValidateShortname(got.NameShortname); err != nil {
		t.Errorf("ReedGeometry().NameShortname %q fails ValidateShortname: %v", got.NameShortname, err)
	}
	if got.NameSlug != "" || got.ParentName != "" {
		t.Errorf("ReedGeometry() NameSlug/ParentName = %q/%q; want both empty", got.NameSlug, got.ParentName)
	}

	if want := "lym-" + hash8; got.SocketKey != want {
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
	// PaneCwd and AnchorPath are asserted to differ in the same case, since the fixture target is not under stateDir.
	if got.PaneCwd == got.AnchorPath {
		t.Errorf("ReedGeometry().PaneCwd = %q; want != AnchorPath %q", got.PaneCwd, got.AnchorPath)
	}
	if got.WorktreeRoot != target {
		t.Errorf("ReedGeometry().WorktreeRoot = %q; want %q (target)", got.WorktreeRoot, target)
	}
	if want := filepath.Join(stateDir, "logs"); got.LogsDir != want {
		// fabricengine.HubLogsDir is one call away and returns a board-shaped path,
		// so this row spells the expected value out.
		t.Errorf("ReedGeometry().LogsDir = %q; want %q (stateDir joined with \"logs\")", got.LogsDir, want)
	}
	if got.HubPath != stateDir {
		t.Errorf("ReedGeometry().HubPath = %q; want %q (stateDir)", got.HubPath, stateDir)
	}
	if want := "distinctive-repo-name"; got.WorktreeName != want {
		t.Errorf("ReedGeometry().WorktreeName = %q; want %q", got.WorktreeName, want)
	}
}

// TestReedGeometry_SessionNameSanitizesTheReadableHalf pins that the readable half of SessionName is tmux-safe.
// reedengine's validateToldTmuxIdentity refuses '.', ':', '\', control characters and invalid UTF-8,
// so a repository named "my.repo" needs its readable half sanitized.
// The hash8 suffix is never sanitized, since it carries the identity.
func TestReedGeometry_SessionNameSanitizesTheReadableHalf(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	stateDir := filepath.Join(string(filepath.Separator), "var", "lib", "lym-state", "abcd1234")
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
			// The sanitizer is reedengine's own, so the rule lives in the package that refuses violations of it.
			if want := reedengine.SanitizeSessionName(c.basename) + "-" + hash8; got.SessionName != want {
				t.Errorf("ReedGeometry(%q).SessionName = %q; want %q (reedengine.SanitizeSessionName + hash8)", target, got.SessionName, want)
			}
			// WorktreeName is a display value, not a tmux target, so it is never sanitized.
			if got.WorktreeName != c.basename {
				t.Errorf("ReedGeometry(%q).WorktreeName = %q; want %q (raw basename)", target, got.WorktreeName, c.basename)
			}
		})
	}
}

func TestWebsterGeometry(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	target := filepath.Join(string(filepath.Separator), "home", "operator", "src", "distinctive-repo-name")
	stateDir := filepath.Join(string(filepath.Separator), "var", "lib", "lym-state", "abcd1234")

	got := WebsterGeometry(target, stateDir)

	if got.AnchorRoot != stateDir {
		t.Errorf("WebsterGeometry().AnchorRoot = %q; want %q (stateDir)", got.AnchorRoot, stateDir)
	}
	if got.WorktreeRoot != target {
		t.Errorf("WebsterGeometry().WorktreeRoot = %q; want %q (target)", got.WorktreeRoot, target)
	}
	if got.PaneCwd != target {
		t.Errorf("WebsterGeometry().PaneCwd = %q; want %q (target)", got.PaneCwd, target)
	}
	// AnchorRoot and WorktreeRoot are asserted to differ in the same case.
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
	if want := filepath.Join(stateDir, lymdirs.LymDirName, "stencils"); got.StencilsDir != want {
		t.Errorf("WebsterGeometry().StencilsDir = %q; want %q", got.StencilsDir, want)
	}
	// This equality pins that the StencilsDir field comes from the shared StencilsDir helper, not a re-derived path.
	if want := StencilsDir(stateDir); got.StencilsDir != want {
		t.Errorf("WebsterGeometry().StencilsDir = %q; want %q (StencilsDir(stateDir))", got.StencilsDir, want)
	}
	if want := filepath.Join(stateDir, lymdirs.LymDirName, "specs"); got.SpecsDir != want {
		t.Errorf("WebsterGeometry().SpecsDir = %q; want %q", got.SpecsDir, want)
	}
	// This equality pins that the SpecsDir field comes from the shared SpecsDir helper, not a re-derived path.
	if want := SpecsDir(stateDir); got.SpecsDir != want {
		t.Errorf("WebsterGeometry().SpecsDir = %q; want %q (SpecsDir(stateDir))", got.SpecsDir, want)
	}
	if want := verifytree.Dir(stateDir); got.VerifyDir != want {
		t.Errorf("WebsterGeometry().VerifyDir = %q; want %q (verifytree.Dir(stateDir))", got.VerifyDir, want)
	}

	// GateConfig reads the target's own gate.yaml at each call, so it needs a real directory.
	realTarget := t.TempDir()
	withConfig := WebsterGeometry(realTarget, stateDir)
	if withConfig.GateConfig == nil {
		t.Fatal("WebsterGeometry().GateConfig = nil; want the worktree's gate config reader")
	}
	if _, err := withConfig.GateConfig(); err == nil || !strings.Contains(err.Error(), "lym config gate") {
		t.Errorf("GateConfig() over a worktree with no gate.yaml = %v; want an error naming lym config gate", err)
	}
	configFile := configengine.ConfigFile(realTarget, "gate")
	if err := os.MkdirAll(filepath.Dir(configFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configFile, []byte("slots: 4\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if cfg, err := withConfig.GateConfig(); err != nil || cfg.Slots != 4 {
		t.Errorf("GateConfig() = (slots %d, %v); want the worktree's file, slots 4", cfg.Slots, err)
	}
}
