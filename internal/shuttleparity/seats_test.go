// seats_test.go holds every production seat's Codex launch to Claude's, over a fixture worktree and configuration.

package shuttleparity

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/heddleengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// fixtureMarketplace and fixtureVersion place the fixture's skills in the plugin cache layout.
const (
	fixtureMarketplace = "market"
	fixtureVersion     = "1.0.0"
)

// parityFixture is a fake hub with one worktree and the user's Claude setup, under one temp dir.
type parityFixture struct {
	base       string
	claudeDir  string
	skillRoots string
	env        SeatEnv
}

// seatsEnv returns a seat environment over a temp worktree, for tables that read no configuration.
func seatsEnv(t *testing.T, stencils string) SeatEnv {
	t.Helper()
	location := &lymcwd.Location{HubPath: t.TempDir(), WorktreeName: "worktree"}
	return SeatEnv{StencilsDir: stencils, Location: location, Dir: t.TempDir(), Shortname: "ly", Slug: "task"}
}

// writeFiles writes each path of files with its content, making the directories.
func writeFiles(t *testing.T, files map[string]string) {
	t.Helper()
	for path, content := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
}

// newParityFixture writes the fixture and installs the passage reader.
// It sets CLAUDE_CONFIG_DIR and HOME, so a test that calls it does not run in parallel.
func newParityFixture(t *testing.T) parityFixture {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolve temp dir: %v", err)
	}
	// The anchor lies below the repository root, so Codex gets the root's CLAUDE.md through its pointers alone.
	location := &lymcwd.Location{RepoName: "repo", HubPath: base, WorktreeName: "worktree", AnchorRel: "sub"}
	root, anchor, board := location.WorktreePath(), location.AnchorPath(), fabricengine.BoltDir(base)
	fx := parityFixture{base: base, claudeDir: filepath.Join(base, "claude-config"), skillRoots: filepath.Join(base, "skill-cache")}
	lymHome, operatorHome := filepath.Join(base, "lym-home"), filepath.Join(base, "operator-home")
	homes := "home: " + lymHome + "\noperator_home: " + operatorHome + "\n"

	files := map[string]string{
		filepath.Join(root, ".git", "HEAD"):                     "ref: refs/heads/main\n",
		filepath.Join(root, "CLAUDE.md"):                        "the root's rules",
		filepath.Join(root, "AGENTS.md"):                        "the root's agent rules",
		filepath.Join(anchor, "CLAUDE.md"):                      "the repository's rules",
		configengine.ConfigFile(board, "gate"):                  configengine.SparseBaseHeader + "refuse_raw: [\"go test\"]\n",
		filepath.Join(anchor, "CLAUDE.local.md"):                heddleengine.Marker + "\n\nthe filed entry\n",
		filepath.Join(anchor, "AGENTS.override.md"):             heddleengine.Marker + "\n\nthe filed entry\n",
		filepath.Join(operatorHome, "auth.json"):                "{}",
		filepath.Join(operatorHome, "sessions", ".placeholder"): "",
		filepath.Join(fx.claudeDir, "CLAUDE.md"):                "the global rules of the operator",
		configengine.ConfigFile(anchor, "codex"):                "sandbox_mode: auto\nwritable_roots: []\nskill_roots: [" + fx.skillRoots + "]\n",
		configengine.LocalFile(anchor, "codex"):                 homes,
		configengine.ConfigFile(anchor, "claude"):               "session_scope:\n  enabled: false\n",
		configengine.ConfigFile(board, "language"):              "skills: [\"lang:style\"]\n",
	}
	// A hub's base files of the strictly loaded modules are present and tune nothing.
	for _, module := range []string{"darn", "landing", "drift", "loom", "tapestry"} {
		files[configengine.ConfigFile(board, module)] = configengine.SparseBaseHeader
	}
	writeFiles(t, files)
	for _, name := range []string{"objects", "refs"} {
		if err := os.MkdirAll(filepath.Join(root, ".git", name), 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
	}
	t.Setenv("CLAUDE_CONFIG_DIR", fx.claudeDir)
	t.Setenv("HOME", base)
	t.Cleanup(shuttleengine.InstallPassageReader(func(_, engine, _ string) (shuttleengine.Standing, error) {
		return shuttleengine.Standing{
			Passages: []shuttleengine.Passage{
				{Slug: "agents-prompt", Text: "the shared passage"},
				{Slug: engine + "-prompt", Text: "the passage of " + engine, Engine: engine},
			},
			FileEntries: []shuttleengine.Passage{{Slug: "agents-file", Text: "the filed entry"}},
		}, nil
	}))

	dirs := configengine.Dirs{Anchor: anchor, Board: board}
	registry, err := modelspec.LoadRegistry(dirs)
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	fx.env = SeatEnv{
		StencilsDir: stencilkit.Seed(t),
		SpecsDir:    filepath.Join(base, "specs"),
		Dir:         filepath.Join(base, "stand-ins"),
		Registry:    registry,
		Location:    location,
		Dirs:        dirs,
		Shortname:   "ly",
		Slug:        "task",
		ParentName:  "ly:orch",
	}
	return fx
}

// addSkills writes each `<plugin>:<name>` skill where each engine resolves it:
// in the Codex skill root's plugin cache layout, and in an installed plugin of the Claude config directory.
// The plugin installs also carry one plugin with a session-start hook.
func (fx parityFixture) addSkills(t *testing.T, skills []string) {
	t.Helper()
	installs := map[string]string{"hooked@" + fixtureMarketplace: filepath.Join(fx.base, "plugins", "hooked")}
	files := map[string]string{
		filepath.Join(fx.base, "plugins", "hooked", "hooks", "hooks.json"): `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"emit"}]}]}}`,
	}
	for _, skill := range skills {
		plugin, name, ok := strings.Cut(skill, ":")
		if !ok {
			t.Fatalf("fixture error: the skill %q names no plugin, so Codex would list no item for it", skill)
		}
		root := filepath.Join(fx.base, "plugins", plugin)
		installs[plugin+"@"+fixtureMarketplace] = root
		files[filepath.Join(root, "skills", name, "SKILL.md")] = "# " + name + "\n"
		files[filepath.Join(fx.skillRoots, fixtureMarketplace, plugin, fixtureVersion, "skills", name, "SKILL.md")] = "# " + name + "\n"
	}
	var entries []string
	for id, root := range installs {
		entries = append(entries, fmt.Sprintf(`%q:[{"scope":"user","installPath":%q}]`, id, root))
	}
	slices.Sort(entries)
	files[filepath.Join(fx.claudeDir, "plugins", "installed_plugins.json")] = `{"version":2,"plugins":{` + strings.Join(entries, ",") + `}}`
	writeFiles(t, files)
}

// codexDeliveryWayForward is what a failure over a missing compared item tells its reader to do.
const codexDeliveryWayForward = "way forward: give the Codex engine a delivery for it in internal/shuttleengine/codexengine"

// addSkillsOfSpecs adds every skill the specs hold, a config role's skills included, as a start expands them.
func (fx parityFixture) addSkillsOfSpecs(t *testing.T, cfg shuttleengine.Config, specs []map[string]shuttleengine.Spec) {
	t.Helper()
	var skills []string
	for _, byEngine := range specs {
		for _, spec := range byEngine {
			named := spec.Skills
			if spec.ConfigRole != "" {
				var err error
				if named, err = cfg.Agents.Skills(spec.ConfigRole, spec.Skills); err != nil {
					t.Fatalf("expand the skills of role %q: %v", spec.ConfigRole, err)
				}
			}
			for _, skill := range named {
				if !slices.Contains(skills, skill) {
					skills = append(skills, skill)
				}
			}
		}
	}
	slices.Sort(skills)
	fx.addSkills(t, skills)
}

// TestSeats_CodexManifestHoldsClaudes pins Codex's launch against Claude's for every seat of every production table.
// Each compared item Claude's launch gets, Codex's gets too.
// It sets CLAUDE_CONFIG_DIR and HOME and installs the process passage reader, so it does not run in parallel.
//
//lym:guard
func TestSeats_CodexManifestHoldsClaudes(t *testing.T) {
	tiergate.Require(t, 0)

	fx := newParityFixture(t)
	cfg, err := shuttleengine.LoadConfig(fx.env.Dirs, "shuttle")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	seats, failures := seatsOf(fx.env, seatTables)
	for _, failure := range failures {
		t.Error(failure)
	}

	specs := make([]map[string]shuttleengine.Spec, len(seats))
	for i, seat := range seats {
		specs[i] = map[string]shuttleengine.Spec{}
		for _, engine := range []string{ReferenceEngine, ComparedEngine} {
			spec, err := seat.Specs(engine)
			if err != nil {
				t.Fatalf("seat %s: %v", seat.Name, err)
			}
			specs[i][engine] = spec
		}
	}
	fx.addSkillsOfSpecs(t, cfg, specs)

	set, err := shuttleengines.NewWithSeams(cfg, fx.env.Dirs, shuttleengines.LaunchSeams{RunHook: UnrunHooks, SandboxProbe: ConfiguredSandbox})
	if err != nil {
		t.Fatalf("NewWithSeams: %v", err)
	}
	for i, seat := range seats {
		manifests, err := PrepareEach(set, cfg, fx.env.StencilsDir, t.TempDir(), specs[i])
		if err != nil {
			t.Errorf("seat %s: %v", seat.Name, err)
			continue
		}
		for _, item := range Compare(set, manifests) {
			t.Errorf("seat %s: Codex's launch lacks %s %q; %s", seat.Name, item.Channel, item.Key, codexDeliveryWayForward)
		}
		// The comparison holds Codex only to what Claude's manifest lists, so the fixture must keep these listed.
		for _, want := range []struct {
			channel shuttleengine.ManifestChannel
			key     string
		}{
			{shuttleengine.ManifestChannelGuard, "refuse-raw"},
			{shuttleengine.ManifestChannelInstructionSource, "CLAUDE.md"},
		} {
			if !slices.ContainsFunc(manifests[ReferenceEngine].Items, func(item shuttleengine.ManifestItem) bool {
				return item.Channel == want.channel && item.Key == want.key
			}) {
				t.Errorf("seat %s: the Claude manifest lacks %s %q, so the fixture no longer exercises its delivery", seat.Name, want.channel, want.key)
			}
		}
		sandbox := slices.IndexFunc(manifests[ComparedEngine].Items, func(item shuttleengine.ManifestItem) bool {
			return item.Channel == shuttleengine.ManifestChannelSettings && item.Detail == "workspace-write"
		})
		if sandbox < 0 {
			t.Errorf("seat %s: the Codex manifest lists no workspace-write sandbox setting: %+v", seat.Name, manifests[ComparedEngine].Items)
		}
	}
}
