// manifest_test.go covers the launch manifest Prepare writes: which items a launch holds, read back from its launch line.

package codexengine

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/agentconfig"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/heddleengine"
	"github.com/Knatte18/loomyard/internal/shellguard"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// manifestItemsOf returns a record's items of the given channel and key, matched on channel and key alone.
func manifestItemsOf(m shuttleengine.Manifest, channel shuttleengine.ManifestChannel, key string) []shuttleengine.ManifestItem {
	var found []shuttleengine.ManifestItem
	for _, item := range m.Items {
		if item.Channel == channel && item.Key == key {
			found = append(found, item)
		}
	}
	return found
}

// shellRowNames returns the names of the shell guard rows hanging on sw, for a hub whose refuse list is empty.
func shellRowNames(sw shellguard.Switch) []string {
	var names []string
	for _, row := range shellguard.Rows(foregroundPhrase, nil) {
		if row.Switch == sw {
			names = append(names, row.Name)
		}
	}
	return names
}

// TestPrepare_WritesTheManifest reads back the manifest Prepare writes for each kind of spec and checkout.
// Every row holds the items its spec and config deliver and none of the items they do not.
// It installs the process-wide passage reader, so it does not run in parallel.
func TestPrepare_WritesTheManifest(t *testing.T) {
	tiergate.Require(t, 0)

	const (
		globalFile = "/home/user/.claude/CLAUDE.md"
		houseFile  = "house file text"
	)
	house := shuttleengine.Standing{
		Passages:    []shuttleengine.Passage{{Slug: "house", Text: "house rule", Engine: engineName}},
		FileEntries: []shuttleengine.Passage{{Slug: "house-file", Text: houseFile}},
	}
	setup := shuttleengine.UserSetup{
		InstructionFiles: []shuttleengine.InstructionItem{{Name: globalFile, Text: "global rules"}},
		SkillListing:     "listing of skills",
		ListedSkills:     []string{"scribe:prose", "mine"},
		PluginContexts:   []shuttleengine.InstructionItem{{Name: "plugin p@m SessionStart hook emit", Text: "plugin context", Plugin: "p@m", Hook: "emit"}},
		Omitted:          []shuttleengine.OmittedHook{{Name: "plugin q@m SessionStart hook slow", Reason: "exit status 3", Plugin: "q@m", Hook: "slow"}},
	}
	item := func(channel shuttleengine.ManifestChannel, key string, delivery shuttleengine.ManifestDelivery) shuttleengine.ManifestItem {
		return shuttleengine.ManifestItem{Channel: channel, Key: key, Delivery: delivery}
	}
	delivered := func(channel shuttleengine.ManifestChannel, key string) shuttleengine.ManifestItem {
		return item(channel, key, shuttleengine.DeliveryDelivered)
	}
	guards := func(keys []string) []shuttleengine.ManifestItem {
		var items []shuttleengine.ManifestItem
		for _, key := range keys {
			items = append(items, delivered(shuttleengine.ChannelGuard, key))
		}
		return items
	}
	switched := append(guards(shellRowNames(shellguard.SwitchDenyPython)), guards(shellRowNames(shellguard.SwitchDenyPublish))...)
	nudge := delivered(shuttleengine.ChannelBatchingNudge, "batching-nudge")

	base := shuttleengine.Spec{
		Prompt: "do it", PromptEngine: engineName, Role: "writer", ConfigRole: agentconfig.RoleDarn,
		Skills: []string{"scribe:prose", "gone:missing"},
	}
	tests := []struct {
		name string
		spec shuttleengine.Spec
		// denyPython is the shuttle config's python switch.
		denyPython bool
		// codexExtra is appended to the codex.yaml local layer.
		codexExtra string
		// agentsMD writes an AGENTS.md beside the checkout's CLAUDE.md, so only the instruction-file pointers name it.
		agentsMD bool
		// unmarked writes the AGENTS.override.md without heddle's marker.
		unmarked bool
		// extraOmitted is appended to the setup's omitted hooks.
		extraOmitted []shuttleengine.OmittedHook
		// blockedManifest plants a directory where manifest.json is written, so the write fails.
		blockedManifest bool
		want            []shuttleengine.ManifestItem
		wantAbsent      []shuttleengine.ManifestItem
	}{
		{
			name: "AutonomousSpecWithSkills", spec: base,
			want: append(guards(slices.Concat(shellRowNames(shellguard.SwitchNone), shellRowNames(shellguard.SwitchDenyPython), shellRowNames(shellguard.SwitchDenyRawTmux))),
				delivered(shuttleengine.ChannelPromptEngine, "prompt-engine"),
				delivered(shuttleengine.ChannelGuard, "question-deny"),
				delivered(shuttleengine.ChannelSkillLoad, "scribe:prose"),
				delivered(shuttleengine.ChannelNotice, "deny-notice"),
				delivered(shuttleengine.ChannelNotice, "creel-trust"),
				delivered(shuttleengine.ChannelNotice, "subagent-passage"),
				nudge,
				delivered(shuttleengine.ChannelInstructionSource, globalFile),
				item(shuttleengine.ChannelInstructionSource, "CLAUDE.md", shuttleengine.DeliveryNative),
				delivered(shuttleengine.ChannelSkillListing, "scribe:prose"),
				delivered(shuttleengine.ChannelSkillListing, "mine"),
				delivered(shuttleengine.ChannelPluginContext, "p@m/emit"),
				shuttleengine.ManifestItem{Channel: shuttleengine.ChannelStandingPassage, Key: "house", Engine: engineName, Delivery: shuttleengine.DeliveryDelivered},
				delivered(shuttleengine.ChannelInstructionSource, "heddle-file/house-file"),
			),
			wantAbsent: append(guards(shellRowNames(shellguard.SwitchDenyPublish)), delivered(shuttleengine.ChannelSkillLoad, "gone:missing"), delivered(shuttleengine.ChannelGuard, shuttleengine.WebsterForkDenyKey)),
		},
		{
			name: "HandWrittenPrompt", spec: func() shuttleengine.Spec {
				spec := base
				spec.PromptEngine, spec.HandWrittenPrompt = "", true
				return spec
			}(),
			want:       []shuttleengine.ManifestItem{delivered(shuttleengine.ChannelPromptEngine, "hand-written")},
			wantAbsent: []shuttleengine.ManifestItem{delivered(shuttleengine.ChannelPromptEngine, "prompt-engine")},
		},
		{
			name: "InteractiveSpecHoldsNoQuestionDenyAndTheTmuxScriptRow", spec: func() shuttleengine.Spec {
				spec := base
				spec.Interactive = true
				return spec
			}(),
			want:       guards(shellRowNames(shellguard.SwitchDenyRawTmux)),
			wantAbsent: []shuttleengine.ManifestItem{delivered(shuttleengine.ChannelGuard, "question-deny")},
		},
		{
			name: "AllowRawTmuxSpecHoldsNoTmuxScriptRow", spec: func() shuttleengine.Spec {
				spec := base
				spec.AllowRawTmux = true
				return spec
			}(),
			wantAbsent: guards(shellRowNames(shellguard.SwitchDenyRawTmux)),
		},
		{
			name: "SwitchedGuardRows", spec: func() shuttleengine.Spec {
				spec := base
				spec.DenyPublish = true
				return spec
			}(),
			denyPython: true,
			want:       switched,
		},
		{
			name: "BatchingNudgeOffHoldsNoNudge", spec: base, codexExtra: "batching_nudge:\n  enabled: false\n",
			wantAbsent: []shuttleengine.ManifestItem{nudge},
		},
		{
			name: "ClaudeFileOnlyAPointerNamesIsDelivered", spec: base, agentsMD: true, unmarked: true,
			want: []shuttleengine.ManifestItem{delivered(shuttleengine.ChannelInstructionSource, "CLAUDE.md")},
		},
		{
			name: "UnmarkedOverrideHoldsNoFileEntry", spec: base, unmarked: true,
			want:       []shuttleengine.ManifestItem{{Channel: shuttleengine.ChannelStandingPassage, Key: "house", Engine: engineName, Delivery: shuttleengine.DeliveryDelivered}},
			wantAbsent: []shuttleengine.ManifestItem{delivered(shuttleengine.ChannelInstructionSource, "heddle-file/house-file")},
		},
		{
			name: "OmittedHookWithNoPluginIsListedByItsName", spec: base,
			extraOmitted: []shuttleengine.OmittedHook{{Name: "/plugins/p/hooks/hooks.json", Reason: "unreadable"}},
			want:         []shuttleengine.ManifestItem{item(shuttleengine.ChannelPluginContext, "/plugins/p/hooks/hooks.json", shuttleengine.DeliveryOmitted)},
		},
		{name: "UnwritableManifestRefusesTheLaunch", spec: base, blockedManifest: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newRepoFixture(t)
			anchor, _ := repo.addWorktree(t, "wt")
			skillRoot, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatalf("resolve temp dir: %v", err)
			}
			writeSkill(t, skillRoot, "market", "scribe", "1.0.0", "prose")
			repo.writeCodexLayer(t, anchor, "skill_roots: ["+skillRoot+"]\n"+tt.codexExtra)
			writeFile(t, filepath.Join(anchor, instructionFallbackFile), "repository rules\n")
			if tt.agentsMD {
				writeFile(t, filepath.Join(anchor, agentsFile), "agents rules\n")
			}
			override := houseFile + "\n"
			if !tt.unmarked {
				override = heddleengine.Marker + "\n\n" + override
			}
			writeFile(t, filepath.Join(anchor, agentsOverrideFile), override)
			agentsPath := configengine.ConfigFile(anchor, "agents")
			mkdirAll(t, filepath.Dir(agentsPath))
			writeFile(t, agentsPath, "efforts:\n  default: high\n")
			agents, err := agentconfig.Load(configengine.Dirs{Anchor: anchor})
			if err != nil {
				t.Fatalf("agentconfig.Load() error: %v", err)
			}

			engine, _ := newEngine(anchor)
			board := t.TempDir()
			repo.writeCodexConfig(t, configengine.ConfigFile(board, "codex"), "")
			engine.dirs.Board = board
			rowSetup := setup
			rowSetup.Omitted = append(slices.Clone(setup.Omitted), tt.extraOmitted...)
			engine.userSetup = &fakeUserSetupSource{setup: rowSetup}
			engine.runHook = func(shuttleengine.HookRun) ([]byte, error) { return nil, nil }
			t.Cleanup(shuttleengine.InstallPassageReader(func(string, string, string) (shuttleengine.Standing, error) { return house, nil }))

			runDir := t.TempDir()
			if tt.blockedManifest {
				mkdirAll(t, filepath.Join(runDir, shuttleengine.ManifestFileName))
				if _, err := engine.Prepare(runDir, tt.spec, shuttleengine.Config{Agents: agents, DenyPython: tt.denyPython}); err == nil || !strings.Contains(err.Error(), "way forward") {
					t.Errorf("Prepare() error = %v; want a refusal naming the way forward", err)
				}
				return
			}
			if _, err := engine.Prepare(runDir, tt.spec, shuttleengine.Config{Agents: agents, DenyPython: tt.denyPython}); err != nil {
				t.Fatalf("Prepare() error: %v", err)
			}
			got, found, err := shuttleengine.ReadManifest(filepath.Join(runDir, shuttleengine.ManifestFileName))
			if err != nil || !found {
				t.Fatalf("ReadManifest() = found %v, error %v; want the manifest Prepare wrote", found, err)
			}

			for _, want := range tt.want {
				if items := manifestItemsOf(got, want.Channel, want.Key); len(items) != 1 || items[0].Engine != want.Engine || items[0].Delivery != want.Delivery {
					t.Errorf("manifest items %s/%s = %+v; want one with engine %q and delivery %q", want.Channel, want.Key, items, want.Engine, want.Delivery)
				}
			}
			for _, absent := range tt.wantAbsent {
				if items := manifestItemsOf(got, absent.Channel, absent.Key); len(items) != 0 {
					t.Errorf("manifest items %s/%s = %+v; want none", absent.Channel, absent.Key, items)
				}
			}
			omitted := manifestItemsOf(got, shuttleengine.ChannelPluginContext, "q@m/slow")
			if len(omitted) != 1 || omitted[0].Delivery != shuttleengine.DeliveryOmitted || omitted[0].Reason != "exit status 3" {
				t.Errorf("manifest items plugin-context/q@m/slow = %+v; want one omitted with reason %q", omitted, "exit status 3")
			}
			wantVerdict := []string{"q@m/slow"}
			for _, hook := range tt.extraOmitted {
				wantVerdict = append(wantVerdict, hook.Name)
			}
			var gotVerdict []string
			for _, verdict := range got.Verdict {
				gotVerdict = append(gotVerdict, verdict.Key)
			}
			if !slices.Equal(gotVerdict, wantVerdict) {
				t.Errorf("Verdict keys = %v; want %v, the omitted plugin hooks alone", gotVerdict, wantVerdict)
			}
			for _, key := range []string{sandboxModeKey, instructionsSizeKey} {
				settings := manifestItemsOf(got, shuttleengine.ChannelSettings, key)
				if len(settings) != 1 || settings[0].Engine != engineName || settings[0].Detail == "" {
					t.Errorf("manifest items settings/%s = %+v; want one keyed to %s with a detail", key, settings, engineName)
				}
			}
			if sandbox := manifestItemsOf(got, shuttleengine.ChannelSettings, sandboxModeKey); len(sandbox) == 1 && sandbox[0].Detail != sandboxWorkspaceWrite {
				t.Errorf("sandbox settings detail = %q; want %q", sandbox[0].Detail, sandboxWorkspaceWrite)
			}
			if got.Engine != engineName || got.Role != tt.spec.Role || got.PromptEngine != tt.spec.PromptEngine {
				t.Errorf("manifest header = %q, %q, %q; want %q, %q, %q", got.Engine, got.Role, got.PromptEngine, engineName, tt.spec.Role, tt.spec.PromptEngine)
			}
		})
	}
}
