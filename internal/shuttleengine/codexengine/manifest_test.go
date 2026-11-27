// manifest_test.go covers the launch manifest Codex's Prepare writes.

package codexengine

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/agentconfig"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/heddleengine"
	"github.com/Knatte18/loomyard/internal/shellguard"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestPrepare_WritesTheManifest is not parallel: its rows install the process-wide passage reader.
func TestPrepare_WritesTheManifest(t *testing.T) {
	tiergate.Require(t, 0)

	const (
		global  = "/home/user/.claude/CLAUDE.md"
		found   = "scribe:prose"
		unfound = "ghost:none"
	)
	// The expected keys are written "channel/key".
	guardKeys := func(switches ...shellguard.Switch) []string {
		var keys []string
		for _, row := range shellguard.Rows(foregroundPhrase, nil) {
			if slices.Contains(switches, row.Switch) {
				keys = append(keys, "guard/"+row.Name)
			}
		}
		return keys
	}
	switched := guardKeys(shellguard.SwitchDenyPython, shellguard.SwitchDenyPublish)

	tests := []struct {
		name string
		spec shuttleengine.Spec
		cfg  shuttleengine.Config
		// codexExtra is added to the codex.yaml local layer.
		codexExtra string
		// files are written under the worktree, keyed by slash path.
		files map[string]string
		// want and absent are "channel/key" items the record holds and lacks.
		want   []string
		absent []string
	}{
		{
			name: "an autonomous run with skills",
			spec: shuttleengine.Spec{PromptEngine: engineName, Skills: []string{found, unfound}, ConfigRole: agentconfig.RoleDarn},
			want: append(guardKeys(shellguard.SwitchNone),
				"prompt-engine/prompt-engine", "guard/question-deny", "skill-load/scribe:prose",
				"notice/deny-notice", "notice/creel-trust", "notice/subagent-passage",
				"instruction-source/"+global, "skill-listing/p:x", "skill-listing/p:y", "standing-passage/said",
				"plugin-context/p@m/emit", "plugin-context/p@m/broken", "batching-nudge/batching-nudge",
				"settings/sandbox_mode", "settings/developer_instructions"),
			absent: append(switched, "skill-load/ghost:none"),
		},
		{
			name:   "an interactive run",
			spec:   shuttleengine.Spec{PromptEngine: engineName, Interactive: true},
			absent: []string{"guard/question-deny"},
		},
		{
			name: "the switched guards",
			spec: shuttleengine.Spec{PromptEngine: engineName, DenyPublish: true},
			cfg:  shuttleengine.Config{DenyPython: true},
			want: switched,
		},
		{
			name:       "the batching nudge off",
			spec:       shuttleengine.Spec{PromptEngine: engineName},
			codexExtra: "batching_nudge:\n  enabled: false\n",
			absent:     []string{"batching-nudge/batching-nudge"},
		},
		{
			name:  "a CLAUDE.md Codex reads itself",
			spec:  shuttleengine.Spec{PromptEngine: engineName},
			files: map[string]string{"CLAUDE.md": "repository rules"},
			want:  []string{"instruction-source/CLAUDE.md"},
		},
		{
			name:  "a CLAUDE.md only a pointer names",
			spec:  shuttleengine.Spec{PromptEngine: engineName},
			files: map[string]string{"CLAUDE.md": "repository rules", "AGENTS.md": "other rules"},
			want:  []string{"instruction-source/CLAUDE.md"},
		},
		{
			name:  "a file entry under a marked override",
			spec:  shuttleengine.Spec{PromptEngine: engineName},
			files: map[string]string{"AGENTS.override.md": heddleengine.Marker + "\n\nfiled text\n"},
			want:  []string{"instruction-source/heddle-file/filed"},
		},
		{
			name:   "a file entry under an unmarked override",
			spec:   shuttleengine.Spec{PromptEngine: engineName},
			files:  map[string]string{"AGENTS.override.md": "filed text\n"},
			absent: []string{"instruction-source/heddle-file/filed"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newRepoFixture(t)
			anchor, _ := repo.addWorktree(t, "wt")
			roots := t.TempDir()
			writeSkill(t, roots, "market", "scribe", "1.0.0", "prose")
			extra := "skill_roots: [" + roots + "]\n" + tt.codexExtra
			repo.writeCodexLayer(t, anchor, extra)
			for name, content := range tt.files {
				writeFile(t, filepath.Join(anchor, filepath.FromSlash(name)), content)
			}
			engine, _ := newEngine(anchor)
			engine.dirs.Board = t.TempDir()
			repo.writeCodexConfig(t, configengine.ConfigFile(engine.dirs.Board, "codex"), "")
			engine.userSetup = &fakeUserSetupSource{setup: shuttleengine.UserSetup{
				InstructionFiles: []shuttleengine.InstructionItem{{Name: global, Text: "global 'quoted' rules\nsecond line"}},
				SkillListing:     "listing of skills",
				ListedSkills:     []string{"p:x", "p:y"},
				PluginContexts:   []shuttleengine.InstructionItem{{Name: "emit", Text: "plugin context", Plugin: "p@m", Hook: "emit"}},
				Omitted:          []shuttleengine.OmittedHook{{Name: "broken", Reason: "exit status 3", Plugin: "p@m", Hook: "broken"}},
			}}
			t.Cleanup(shuttleengine.InstallPassageReader(func(string, string, string) (shuttleengine.Standing, error) {
				return shuttleengine.Standing{
					Passages:    []shuttleengine.Passage{{Slug: "said", Text: "said text", Engine: engineName}},
					FileEntries: []shuttleengine.Passage{{Slug: "filed", Text: "filed text", Engine: engineName}},
				}, nil
			}))
			cfg := tt.cfg
			agentsPath := configengine.ConfigFile(anchor, "agents")
			writeFile(t, agentsPath, "efforts:\n  default: high\n")
			agents, err := agentconfig.Load(configengine.Dirs{Anchor: anchor})
			if err != nil {
				t.Fatalf("agentconfig.Load() error: %v", err)
			}
			cfg.Agents = agents
			spec := tt.spec
			spec.Prompt = "p"
			runDir := t.TempDir()

			if _, err := engine.Prepare(runDir, spec, cfg); err != nil {
				t.Fatalf("Prepare() error: %v", err)
			}
			manifest, recorded, err := shuttleengine.ReadManifest(runDir)
			if err != nil || !recorded {
				t.Fatalf("ReadManifest() = found %v, error %v", recorded, err)
			}
			if manifest.Engine != engineName {
				t.Errorf("manifest engine = %q; want %q", manifest.Engine, engineName)
			}
			items := map[string]shuttleengine.ManifestItem{}
			for _, item := range manifest.Items {
				items[string(item.Channel)+"/"+item.Key] = item
			}
			for _, want := range tt.want {
				item, ok := items[want]
				if !ok {
					t.Errorf("manifest lacks %s; holds %+v", want, manifest.Items)
					continue
				}
				wantDelivery := shuttleengine.ManifestDelivered
				if want == "plugin-context/p@m/broken" {
					wantDelivery = shuttleengine.ManifestOmitted
				}
				if item.Delivery != wantDelivery {
					t.Errorf("%s delivery = %q; want %q", want, item.Delivery, wantDelivery)
				}
			}
			for _, absent := range tt.absent {
				if _, ok := items[absent]; ok {
					t.Errorf("manifest holds %s; want it absent", absent)
				}
			}
			if len(manifest.Verdict) != 1 || manifest.Verdict[0].Key != "p@m/broken" || manifest.Verdict[0].Reason != "exit status 3" {
				t.Errorf("verdict = %+v; want the omitted hook alone", manifest.Verdict)
			}
			sandbox := items["settings/sandbox_mode"]
			if sandbox.Engine != engineName || sandbox.Detail != sandboxWorkspaceWrite {
				t.Errorf("sandbox setting = %+v; want engine %q and detail %q", sandbox, engineName, sandboxWorkspaceWrite)
			}
			if got := items["settings/developer_instructions"].Engine; got != engineName {
				t.Errorf("developer instructions setting engine = %q; want %q", got, engineName)
			}
			if got := items["standing-passage/said"].Engine; got != engineName {
				t.Errorf("standing passage engine = %q; want %q", got, engineName)
			}
		})
	}
}
