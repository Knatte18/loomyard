// seats_test.go pins, for every seat of the production seat tables, that the Codex manifest holds what Claude's does.
// The tables are built over seeded stencils and a fixture machine, and each seat is prepared on both real engines.

package shuttleparity

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/darnengine"
	"github.com/Knatte18/loomyard/internal/driftengine"
	"github.com/Knatte18/loomyard/internal/heddleengine"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// fixtureFileEntryText is the text of the heddle file entry both engines' marked instruction files hold.
const fixtureFileEntryText = "house file text"

// fixtureMarketplace is the marketplace folder of every plugin the fixture installs.
const fixtureMarketplace = "market"

// parityFixture is a fake machine: a worktree, a Claude config directory, a Codex skill root and a hub board.
type parityFixture struct {
	env         SeatEnv
	cfg         shuttleengine.Config
	stencilsDir string
	// claudeDir is the Claude config directory, and skillRoot the directory Codex's skill_roots names.
	claudeDir string
	skillRoot string
	// plugins maps each installed plugin's id to its install directory.
	plugins map[string]string
}

// newParityFixture seeds the stencils and writes the fake machine.
// The worktree holds a CLAUDE.md, a marked CLAUDE.local.md and a marked AGENTS.override.md with the file entry's text.
// The Claude config directory holds a global CLAUDE.md and a plugin with a session-start hook.
// The hub's language.yaml lists one language skill, and the codex.yaml layer sets sandbox_mode auto and the skill root.
// It sets CLAUDE_CONFIG_DIR and HOME and installs the process passage reader,
// so a test that calls it does not run in parallel.
func newParityFixture(t *testing.T) parityFixture {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolve temp dir: %v", err)
	}
	fx := parityFixture{
		stencilsDir: stencilkit.Seed(t),
		claudeDir:   filepath.Join(base, "claude-config"),
		skillRoot:   filepath.Join(base, "skill-root"),
		plugins:     map[string]string{},
	}
	anchor := filepath.Join(base, "worktree")
	board := filepath.Join(base, "board")
	lymHome, operatorHome := filepath.Join(base, "lym-home"), filepath.Join(base, "operator-home")
	files := map[string]string{
		filepath.Join(anchor, ".git", "HEAD"):                   "ref: refs/heads/main\n",
		filepath.Join(anchor, "CLAUDE.md"):                      "repository rules\n",
		filepath.Join(anchor, "CLAUDE.local.md"):                heddleengine.Marker + "\n\n" + fixtureFileEntryText + "\n",
		filepath.Join(anchor, "AGENTS.override.md"):             heddleengine.Marker + "\n\n" + fixtureFileEntryText + "\n",
		filepath.Join(operatorHome, "auth.json"):                "{}",
		filepath.Join(operatorHome, "sessions", ".placeholder"): "",
		filepath.Join(fx.claudeDir, "CLAUDE.md"):                "the global rules of the operator\n",
		configengine.ConfigFile(anchor, "codex"):                "sandbox_mode: auto\nwritable_roots: []\nskill_roots: [" + fx.skillRoot + "]\n",
		configengine.LocalFile(anchor, "codex"):                 "home: " + lymHome + "\noperator_home: " + operatorHome + "\n",
		configengine.ConfigFile(anchor, "claude"):               "session_scope:\n  enabled: false\n",
		configengine.ConfigFile(board, "codex"):                 "home: " + lymHome + "\noperator_home: " + operatorHome + "\n",
		configengine.ConfigFile(board, "language"):              "skills:\n  - scribe:prose\n",
		configengine.ConfigFile(board, "darn"):                  darnengine.ConfigTemplate(),
		configengine.ConfigFile(board, "landing"):               landingshed.ConfigTemplate(),
		configengine.ConfigFile(board, "drift"):                 driftengine.ConfigTemplate(),
		configengine.ConfigFile(board, "loom"):                  loomengine.ConfigTemplate(),
		configengine.ConfigFile(board, "tapestry"):              loomengine.TapestryConfigTemplate(),
	}
	for path, content := range files {
		writeFixtureFile(t, path, content)
	}
	for _, name := range []string{"objects", "refs"} {
		if err := os.MkdirAll(filepath.Join(anchor, ".git", name), 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
	}
	fx.addPlugin(t, "house", `{"hooks":{"SessionStart":[{"matcher":"","hooks":[{"type":"command","command":"emit"}]}]}}`)
	t.Setenv("CLAUDE_CONFIG_DIR", fx.claudeDir)
	t.Setenv("HOME", base)

	t.Cleanup(shuttleengine.InstallPassageReader(func(_, engine, _ string) (shuttleengine.Standing, error) {
		standing := shuttleengine.Standing{
			Passages:    []shuttleengine.Passage{{Slug: "agents-prompt", Text: "shared passage"}},
			FileEntries: []shuttleengine.Passage{{Slug: "agents-file", Text: fixtureFileEntryText}},
		}
		switch engine {
		case ReferenceEngine:
			standing.Passages = append(standing.Passages, shuttleengine.Passage{Slug: "claude-prompt", Text: "claude passage", Engine: ReferenceEngine})
		case ComparedEngine:
			standing.Passages = append(standing.Passages, shuttleengine.Passage{Slug: "codex-prompt", Text: "codex passage", Engine: ComparedEngine})
		}
		return standing, nil
	}))

	dirs := configengine.Dirs{Anchor: anchor, Board: board}
	fx.cfg, err = shuttleengine.LoadConfig(dirs, "shuttle")
	if err != nil {
		t.Fatalf("LoadConfig(shuttle) error: %v", err)
	}
	registry, err := modelspec.LoadRegistry(dirs)
	if err != nil {
		t.Fatalf("LoadRegistry() error: %v", err)
	}
	fx.env = SeatEnv{
		StencilsDir: fx.stencilsDir,
		SpecsDir:    filepath.Join(base, "specs"),
		Dir:         filepath.Join(base, "stand-ins"),
		Registry:    registry,
		Location:    &lymcwd.Location{RepoName: "repo", HubPath: base, WorktreeName: "worktree", AnchorRel: "."},
		Dirs:        dirs,
		Shortname:   "ly",
		Slug:        "task",
		ParentName:  "ly:orch",
	}
	return fx
}

// writeFixtureFile writes content to path, making its directories.
func writeFixtureFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// addPlugin installs the plugin called name for Claude with hooksJSON as its hooks file and rewrites the install list.
func (fx parityFixture) addPlugin(t *testing.T, name, hooksJSON string) {
	t.Helper()
	root := filepath.Join(filepath.Dir(fx.claudeDir), "plugins-installed", name)
	fx.plugins[name+"@"+fixtureMarketplace] = root
	if hooksJSON != "" {
		writeFixtureFile(t, filepath.Join(root, "hooks", "hooks.json"), hooksJSON)
	}
	installs := map[string][]map[string]string{}
	for id, installPath := range fx.plugins {
		installs[id] = []map[string]string{{"scope": "user", "installPath": installPath}}
	}
	data, err := json.Marshal(map[string]any{"version": 2, "plugins": installs})
	if err != nil {
		t.Fatalf("marshal installed plugins: %v", err)
	}
	writeFixtureFile(t, filepath.Join(fx.claudeDir, "plugins", "installed_plugins.json"), string(data))
}

// addSkills makes each skill of names, written `<plugin>:<name>`, resolve for both engines.
// Codex finds it at `<skill root>/<marketplace>/<plugin>/<version>/skills/<name>/SKILL.md`.
// Claude finds it in the skills directory of the installed plugin, which it lists as `<plugin>:<name>`.
// A name with no colon is a fixture error, since Codex lists no skill-load item for it whatever file exists.
func (fx parityFixture) addSkills(t *testing.T, names []string) {
	t.Helper()
	for _, skill := range names {
		plugin, name, ok := strings.Cut(skill, ":")
		if !ok || plugin == "" || name == "" {
			t.Fatalf("skill %q is not written <plugin>:<name>; way forward: name the skill that way in the fixture or the seat", skill)
		}
		body := "---\nname: " + name + "\ndescription: The " + name + " skill.\n---\nbody\n"
		writeFixtureFile(t, filepath.Join(fx.skillRoot, fixtureMarketplace, plugin, "1.0.0", "skills", name, "SKILL.md"), body)
		if _, installed := fx.plugins[plugin+"@"+fixtureMarketplace]; !installed {
			fx.addPlugin(t, plugin, "")
		}
		writeFixtureFile(t, filepath.Join(fx.plugins[plugin+"@"+fixtureMarketplace], "skills", name, "SKILL.md"), body)
	}
}

// TestSeats_CodexManifestHoldsClaudes prepares every seat of the production seat tables on both engines.
// It fails for each builder that cannot run on its stand-ins.
// It fails for each compared item of Claude's manifest that Codex's lacks.
// It covers a seat Codex cannot start too, since Prepare refuses no requirement.
// It sets CLAUDE_CONFIG_DIR and HOME and installs the process passage reader, so it does not run in parallel.
//
//lym:guard
func TestSeats_CodexManifestHoldsClaudes(t *testing.T) {
	tiergate.Require(t, 0)

	fx := newParityFixture(t)
	seats, failures := seatsOf(fx.env, seatTables)
	for _, err := range failures {
		t.Errorf("%v", err)
	}
	if len(seats) < len(seatTables) {
		t.Fatalf("seatsOf() built %d seats for %d builders; want at least one seat per builder; way forward: fix the builder errors above", len(seats), len(seatTables))
	}

	engines := []string{ReferenceEngine, ComparedEngine}
	specsOf := make([]map[string]shuttleengine.Spec, len(seats))
	var skills []string
	for i, seat := range seats {
		specsOf[i] = map[string]shuttleengine.Spec{}
		for _, engine := range engines {
			spec, err := seat.Specs(engine)
			if err != nil {
				t.Fatalf("seat %s: the spec on %s: %v", seat.Name, engine, err)
			}
			specsOf[i][engine] = spec
			if spec.ConfigRole != "" {
				roleSkills, err := fx.cfg.Agents.Skills(spec.ConfigRole, spec.Skills)
				if err != nil {
					t.Fatalf("seat %s: expand the skills of role %q: %v", seat.Name, spec.ConfigRole, err)
				}
				skills = append(skills, roleSkills...)
			} else {
				skills = append(skills, spec.Skills...)
			}
		}
	}
	slices.Sort(skills)
	fx.addSkills(t, slices.Compact(skills))

	set, err := shuttleengines.NewWithSeams(fx.cfg, fx.env.Dirs, shuttleengines.LaunchSeams{RunHook: UnrunHooks, SandboxProbe: ConfiguredSandbox})
	if err != nil {
		t.Fatalf("NewWithSeams() error: %v", err)
	}
	for i, seat := range seats {
		t.Run(seat.Name, func(t *testing.T) {
			manifests, err := PrepareEach(set, specsOf[i], fx.stencilsDir, t.TempDir(), fx.cfg)
			if err != nil {
				t.Fatalf("seat %s (builder %s): %v", seat.Name, seat.Builder, err)
			}
			for _, missing := range Compare(set, manifests) {
				t.Errorf("seat %s (builder %s): the Codex manifest lacks %s %q that Claude's holds; way forward: give the Codex engine a delivery for it in internal/shuttleengine/codexengine",
					seat.Name, seat.Builder, missing.Channel, missing.Key)
			}
			settings := manifestSettings(manifests[ComparedEngine], "sandbox-mode")
			if settings != "workspace-write" {
				t.Errorf("seat %s: the Codex sandbox settings item = %q; want the mode ConfiguredSandbox settles to, workspace-write", seat.Name, settings)
			}
		})
	}
}

// manifestSettings returns the detail of the settings item of m called key, empty when it has none.
func manifestSettings(m shuttleengine.Manifest, key string) string {
	for _, item := range m.Items {
		if item.Channel == shuttleengine.ChannelSettings && item.Key == key {
			return item.Detail
		}
	}
	return ""
}
