// manifest_test.go covers the launch manifest Claude's Prepare writes.

package claudeengine

import (
	"os"
	"path/filepath"
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
		skill  = "p:x"
		plugin = "p@m"
		hook   = "emit"
	)
	// The expected keys are written "channel/key".
	guardKeys := func(rows ...shellguard.Switch) []string {
		var keys []string
		for _, row := range shellguard.Rows(bashForegroundPhrase, nil) {
			for _, installed := range rows {
				if row.Switch == installed {
					keys = append(keys, "guard/"+row.Name)
				}
			}
		}
		return keys
	}
	switched := guardKeys(shellguard.SwitchDenyPython, shellguard.SwitchDenyPublish)

	tests := []struct {
		name string
		spec shuttleengine.Spec
		cfg  shuttleengine.Config
		// claudeLayer is the worktree's claude.yaml.
		claudeLayer string
		// passages installs a reader with one passage and one file entry, both keyed to claude.
		passages bool
		unmarked bool
		// want and absent are "channel/key" items the record holds and lacks.
		want   []string
		absent []string
		// wantNative are the items that must be listed as native.
		wantNative []string
	}{
		{
			name: "an autonomous run with skills",
			spec: shuttleengine.Spec{PromptEngine: engineName, Skills: []string{skill}, ConfigRole: agentconfig.RoleDarn},
			want: append(guardKeys(shellguard.SwitchNone),
				"prompt-engine/prompt-engine", "guard/question-deny", "skill-load/p:x", "notice/deny-notice", "notice/creel-trust", "notice/subagent-passage",
				"batching-nudge/batching-nudge", "settings/settings.json"),
			absent:     append(switched, "guard/webster-fork-deny"),
			wantNative: []string{"instruction-source/CLAUDE.md", "instruction-source/GLOBAL", "skill-listing/p:x", "plugin-context/p@m/emit"},
		},
		{
			name:   "a hand-written prompt",
			spec:   shuttleengine.Spec{HandWrittenPrompt: true},
			want:   []string{"prompt-engine/hand-written"},
			absent: []string{"prompt-engine/prompt-engine"},
		},
		{
			name:   "an interactive run",
			spec:   shuttleengine.Spec{PromptEngine: engineName, Interactive: true},
			absent: []string{"guard/question-deny"},
		},
		{
			name: "forked subagents",
			spec: shuttleengine.Spec{PromptEngine: engineName, ForkSubagents: true},
			want: []string{"guard/webster-fork-deny"},
		},
		{
			name: "the switched guards",
			spec: shuttleengine.Spec{PromptEngine: engineName, DenyPublish: true},
			cfg:  shuttleengine.Config{DenyPython: true},
			want: switched,
		},
		{
			name:        "the batching nudge off",
			spec:        shuttleengine.Spec{PromptEngine: engineName},
			claudeLayer: "batching_nudge:\n  enabled: false\n",
			absent:      []string{"batching-nudge/batching-nudge"},
		},
		{
			name:       "standing entries",
			spec:       shuttleengine.Spec{PromptEngine: engineName},
			passages:   true,
			want:       []string{"standing-passage/said"},
			wantNative: []string{"instruction-source/heddle-file/filed"},
		},
		{
			name:     "an unmarked local instruction file",
			spec:     shuttleengine.Spec{PromptEngine: engineName},
			passages: true,
			unmarked: true,
			absent:   []string{"instruction-source/heddle-file/filed"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine, anchor := engineWithAnchor(t)
			configDir := t.TempDir()
			engine.claudeDir = configDir
			global := filepath.Join(configDir, instructionFileName)
			writeUserSetupFile(t, global, "global rules")
			root := filepath.Join(t.TempDir(), "p")
			writePluginInstalls(t, configDir, []pluginInstall{{ID: plugin, Scope: pluginScopeUser, Root: root}})
			writeUserSetupFile(t, filepath.Join(root, pluginHooksDirName, pluginHooksFileName), sessionStartHooksJSON("", 0, hook))
			writeSkillFile(t, root, "x", "Does x.")
			writeUserSetupFile(t, filepath.Join(anchor, ".git", "HEAD"), "ref: refs/heads/trunk\n")
			for _, dir := range []string{"objects", "refs"} {
				if err := os.MkdirAll(filepath.Join(anchor, ".git", dir), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			writeUserSetupFile(t, filepath.Join(anchor, instructionFileName), "repository rules")
			local := heddleengine.Marker + "\n\nfiled text\n"
			if tt.unmarked {
				local = "filed text\n"
			}
			writeUserSetupFile(t, filepath.Join(anchor, localInstructionFileName), local)
			if tt.claudeLayer != "" {
				writeClaudeLayer(t, configengine.ConfigFile(anchor, "claude"), tt.claudeLayer)
			}
			if tt.passages {
				engine.dirs.Board = t.TempDir()
				t.Cleanup(shuttleengine.InstallPassageReader(func(string, string, string) (shuttleengine.Standing, error) {
					return shuttleengine.Standing{
						Passages:    []shuttleengine.Passage{{Slug: "said", Text: "said text", Engine: engineName}},
						FileEntries: []shuttleengine.Passage{{Slug: "filed", Text: "filed text", Engine: engineName}},
					}, nil
				}))
			}
			cfg := tt.cfg
			cfg.Agents = templateAgentsConfig(t).Agents
			spec := tt.spec
			spec.Prompt = "p"
			runDir := t.TempDir()

			if _, err := engine.Prepare(runDir, spec, cfg); err != nil {
				t.Fatalf("Prepare() error: %v", err)
			}
			manifest, found, err := shuttleengine.ReadManifest(runDir)
			if err != nil || !found {
				t.Fatalf("ReadManifest() = found %v, error %v", found, err)
			}
			if manifest.Engine != engineName || len(manifest.Verdict) != 0 {
				t.Errorf("manifest engine = %q, verdict = %+v; want %q and an empty verdict", manifest.Engine, manifest.Verdict, engineName)
			}
			items := map[string]shuttleengine.ManifestItem{}
			for _, item := range manifest.Items {
				key := item.Key
				if key == global {
					key = "GLOBAL"
				}
				items[string(item.Channel)+"/"+key] = item
			}
			for _, want := range append(tt.want, tt.wantNative...) {
				if _, ok := items[want]; !ok {
					t.Errorf("manifest lacks %s; holds %+v", want, manifest.Items)
				}
			}
			for _, want := range tt.wantNative {
				if got := items[want].Delivery; got != shuttleengine.ManifestNative {
					t.Errorf("%s delivery = %q; want %q", want, got, shuttleengine.ManifestNative)
				}
			}
			for _, absent := range tt.absent {
				if _, ok := items[absent]; ok {
					t.Errorf("manifest holds %s; want it absent", absent)
				}
			}
			if got := items["settings/settings.json"].Engine; got != engineName {
				t.Errorf("settings item engine = %q; want %q", got, engineName)
			}
			if got := items["guard/webster-fork-deny"].Engine; got != "" {
				t.Errorf("fork deny engine = %q; want a compared item", got)
			}
			if tt.passages {
				if got := items["standing-passage/said"].Engine; got != engineName {
					t.Errorf("standing passage engine = %q; want %q", got, engineName)
				}
			}
		})
	}
}
