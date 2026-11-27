// manifest_test.go covers the launch manifest Claude's Prepare writes.

package claudeengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/agentconfig"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/heddleengine"
	"github.com/Knatte18/loomyard/internal/shellguard"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestClaudeManifest_ListsNoTextTheLaunchLacks(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	got := claudeManifest(manifestInputs{
		settings:          []byte("{}"),
		appendPrompt:      "other text",
		launchCmd:         "claude",
		passages:          []shuttleengine.Passage{{Slug: "said", Text: "said text"}},
		denyNotice:        "the deny notice",
		subagentPassage:   "the subagent passage",
		skills:            []string{"p:x"},
		skillLoadArgument: "'load p:x'",
		guards:            []guardCandidate{{key: "denied", command: "deny it"}},
		batchingCommand:   "nudge",
		fileEntries:       []shuttleengine.Passage{{Slug: "filed", Text: "filed text"}},
		localInstructions: heddleengine.Marker + "\n\nother text\n",
	})

	for _, item := range got.Items {
		if item.Channel != shuttleengine.ManifestChannelSettings {
			t.Errorf("claudeManifest() lists %s/%s, which the launch does not hold", item.Channel, item.Key)
		}
	}
}

// TestPrepare_WritesTheManifest is not parallel: its rows install the process-wide passage reader.
func TestPrepare_WritesTheManifest(t *testing.T) {
	tiergate.Require(t, 0)

	const (
		skill  = "p:x"
		plugin = "p@m"
		hook   = "emit"
		role   = "writer"
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
		// noRepository leaves the anchor with no .git, and anchorBelowRoot moves it into a sub directory of the repository.
		noRepository    bool
		anchorBelowRoot bool
		// emptyGlobal and unreadableGlobal replace the global instruction file with an empty file or a directory.
		emptyGlobal      bool
		unreadableGlobal bool
		// blockedRecord puts a directory where the record goes.
		blockedRecord bool
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
		{
			name:         "an anchor in no repository",
			spec:         shuttleengine.Spec{PromptEngine: engineName},
			noRepository: true,
			wantNative:   []string{"instruction-source/CLAUDE.md"},
			absent:       []string{"instruction-source/GLOBAL"},
		},
		{
			name:            "an anchor below the repository root",
			spec:            shuttleengine.Spec{PromptEngine: engineName},
			anchorBelowRoot: true,
			wantNative:      []string{"instruction-source/CLAUDE.md", "instruction-source/sub/CLAUDE.md"},
		},
		{
			name:        "an empty global instruction file",
			spec:        shuttleengine.Spec{PromptEngine: engineName},
			emptyGlobal: true,
			wantNative:  []string{"instruction-source/CLAUDE.md"},
			absent:      []string{"instruction-source/GLOBAL"},
		},
		{
			name:             "an unreadable global instruction file",
			spec:             shuttleengine.Spec{PromptEngine: engineName},
			unreadableGlobal: true,
			wantNative:       []string{"instruction-source/CLAUDE.md"},
			absent:           []string{"instruction-source/GLOBAL", "skill-listing/p:x"},
		},
		{
			name:          "a record that cannot be written",
			spec:          shuttleengine.Spec{PromptEngine: engineName},
			blockedRecord: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine, anchor := engineWithAnchor(t)
			configDir := t.TempDir()
			engine.claudeDir = configDir
			global := filepath.Join(configDir, instructionFileName)
			switch {
			case tt.unreadableGlobal:
				if err := os.Mkdir(global, 0o755); err != nil {
					t.Fatal(err)
				}
			case tt.emptyGlobal:
				writeUserSetupFile(t, global, "")
			default:
				writeUserSetupFile(t, global, "global rules")
			}
			root := filepath.Join(t.TempDir(), "p")
			writePluginInstalls(t, configDir, []pluginInstall{{ID: plugin, Scope: pluginScopeUser, Root: root}})
			writeUserSetupFile(t, filepath.Join(root, pluginHooksDirName, pluginHooksFileName), sessionStartHooksJSON("", 0, hook))
			writeSkillFile(t, root, "x", "Does x.")
			if !tt.noRepository {
				writeUserSetupFile(t, filepath.Join(anchor, ".git", "HEAD"), "ref: refs/heads/trunk\n")
				for _, dir := range []string{"objects", "refs"} {
					if err := os.MkdirAll(filepath.Join(anchor, ".git", dir), 0o755); err != nil {
						t.Fatal(err)
					}
				}
			}
			writeUserSetupFile(t, filepath.Join(anchor, instructionFileName), "repository rules")
			if tt.anchorBelowRoot {
				engine.dirs.Anchor = filepath.Join(anchor, "sub")
				writeUserSetupFile(t, filepath.Join(engine.dirs.Anchor, instructionFileName), "sub rules")
			}
			local := heddleengine.Marker + "\n\nfiled text\n"
			if tt.unmarked {
				local = "filed text\n"
			}
			writeUserSetupFile(t, filepath.Join(anchor, heddleengine.ClaudeLocalFile), local)
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
			spec.Role = role
			runDir := t.TempDir()
			record := filepath.Join(runDir, shuttleengine.ManifestFileName)
			if tt.blockedRecord {
				if err := os.Mkdir(record, 0o755); err != nil {
					t.Fatal(err)
				}
			}

			_, err := engine.Prepare(runDir, spec, cfg)
			if tt.blockedRecord {
				if err == nil || !strings.Contains(err.Error(), record) {
					t.Errorf("Prepare() error = %v; want a refusal naming %s", err, record)
				}
				return
			}
			if err != nil {
				t.Fatalf("Prepare() error: %v", err)
			}
			manifest, found, err := shuttleengine.ReadManifest(runDir)
			if err != nil || !found {
				t.Fatalf("ReadManifest() = found %v, error %v", found, err)
			}
			if manifest.Engine != engineName || len(manifest.Verdict) != 0 {
				t.Errorf("manifest engine = %q, verdict = %+v; want %q and an empty verdict", manifest.Engine, manifest.Verdict, engineName)
			}
			if manifest.Role != role || manifest.PromptEngine != tt.spec.PromptEngine {
				t.Errorf("manifest role = %q, prompt engine = %q; want %q and %q", manifest.Role, manifest.PromptEngine, role, tt.spec.PromptEngine)
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
