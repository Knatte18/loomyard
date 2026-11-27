// manifest_test.go covers the manifest verb's answers over a stubbed report,
// and the production report over a fixture hub.

package shuttlecli

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleparity"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestWorktreeManifestReport_RunsNoPluginHook is not parallel: it sets CLAUDE_CONFIG_DIR and HOME.
func TestWorktreeManifestReport_RunsNoPluginHook(t *testing.T) {
	tiergate.Require(t, 0)

	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolve temp dir: %v", err)
	}
	location := &lymcwd.Location{RepoName: "repo", HubPath: base, WorktreeName: "worktree"}
	anchor, board := location.AnchorPath(), fabricengine.BoltDir(base)
	claudeDir, plugin := filepath.Join(base, "claude-config"), filepath.Join(base, "plugin")
	operatorHome, marker := filepath.Join(base, "operator-home"), filepath.Join(base, "hook-ran")
	hookCommand := "touch " + marker

	for path, content := range map[string]string{
		filepath.Join(anchor, ".git", "HEAD"):                         "ref: refs/heads/main\n",
		filepath.Join(operatorHome, "auth.json"):                      "{}",
		filepath.Join(operatorHome, "sessions", ".placeholder"):       "",
		configengine.ConfigFile(anchor, "codex"):                      "sandbox_mode: auto\nwritable_roots: []\n",
		configengine.LocalFile(anchor, "codex"):                       "home: " + filepath.Join(base, "lym-home") + "\noperator_home: " + operatorHome + "\n",
		configengine.ConfigFile(anchor, "claude"):                     "session_scope:\n  enabled: false\n",
		configengine.ConfigFile(board, "drift"):                       configengine.SparseBaseHeader,
		filepath.Join(plugin, "hooks", "hooks.json"):                  `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"` + hookCommand + `"}]}]}}`,
		filepath.Join(claudeDir, "plugins", "installed_plugins.json"): `{"version":2,"plugins":{"hooked@market":[{"scope":"user","installPath":"` + plugin + `"}]}}`,
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"objects", "refs"} {
		if err := os.MkdirAll(filepath.Join(anchor, ".git", name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("CLAUDE_CONFIG_DIR", claudeDir)
	t.Setenv("HOME", base)
	stencilkit.SeedInto(t, fabricengine.StencilsDir(base))
	dirs := configengine.Dirs{Anchor: anchor, Board: board}
	cfg, err := shuttleengine.LoadConfig(dirs, "shuttle")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	report, err := worktreeManifestReport(location, reedengine.Geometry{NameShortname: "ly", ParentName: "ly:orch"}, cfg, dirs)("drift-judge")

	if err != nil {
		t.Fatalf("worktreeManifestReport() error: %v", err)
	}
	items := report.Manifests[shuttleparity.ComparedEngine].Items
	if !slices.ContainsFunc(items, func(item shuttleengine.ManifestItem) bool {
		return item.Channel == shuttleengine.ManifestChannelPluginContext && item.Key == "hooked@market/"+hookCommand &&
			item.Delivery == shuttleengine.ManifestOmitted && item.Reason == shuttleengine.ErrHookUnrun.Error()
	}) {
		t.Errorf("compared manifest = %+v; want the plugin context omitted as unrun", items)
	}
	if _, err := os.Stat(marker); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("stat the hook's marker: %v; want the hook not run", err)
	}
}

// TestManifestCmd pins the exit code and envelope of the manifest verb for each kind of report.
func TestManifestCmd(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	manifests := map[string]shuttleengine.Manifest{
		shuttleparity.ReferenceEngine: {Engine: shuttleparity.ReferenceEngine, Verdict: []shuttleengine.ManifestItem{
			{Channel: shuttleengine.ManifestChannelPluginContext, Key: "p@m/emit", Delivery: shuttleengine.ManifestOmitted},
		}},
		shuttleparity.ComparedEngine: {Engine: shuttleparity.ComparedEngine},
	}
	reports := map[string]shuttleparity.ManifestReport{
		"matching": {Seat: "matching", Manifests: manifests},
		"missing": {Seat: "missing", Manifests: manifests, Missing: []shuttleengine.ManifestItem{
			{Channel: shuttleengine.ManifestChannelNotice, Key: "deny-notice"},
		}},
		"unmet": {Seat: "unmet", Manifests: manifests, Unmet: map[string][]shuttleengine.Requirement{
			shuttleparity.ComparedEngine: {shuttleengine.RequirementForks},
		}},
	}
	c := &shuttleCLI{manifestReport: func(name string) (shuttleparity.ManifestReport, error) {
		report, ok := reports[name]
		if !ok {
			return shuttleparity.ManifestReport{}, errors.New("no seat is called " + name)
		}
		return report, nil
	}}

	tests := []struct {
		seat     string
		wantExit int
		want     []string
	}{
		{seat: "matching", want: []string{`"ok":true`, `"engine":"` + shuttleparity.ReferenceEngine + `"`, `"engine":"` + shuttleparity.ComparedEngine + `"`, `"missing":[]`}},
		{seat: "missing", wantExit: 1, want: []string{`"ok":false`, "notice deny-notice", "way forward", `"manifests"`}},
		{seat: "unmet", want: []string{`"ok":true`, shuttleparity.ComparedEngine + " cannot start this seat: unmet requirements " + string(shuttleengine.RequirementForks)}},
		{seat: "nobody", wantExit: 1, want: []string{`"ok":false`, "no seat is called nobody"}},
	}
	for _, tt := range tests {
		t.Run(tt.seat, func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			if got := clihelp.Execute(c.manifestCmd(), &out, []string{tt.seat}); got != tt.wantExit {
				t.Errorf("manifest %s exit = %d; want %d", tt.seat, got, tt.wantExit)
			}
			parseSingleEnvelope(t, out.Bytes())
			if strings.Contains(out.String(), `"verdict"`) {
				t.Errorf("manifest %s output = %s; want no stored verdict", tt.seat, out.String())
			}
			for _, want := range tt.want {
				if !strings.Contains(out.String(), want) {
					t.Errorf("manifest %s output = %s; want it to contain %q", tt.seat, out.String(), want)
				}
			}
		})
	}
}
