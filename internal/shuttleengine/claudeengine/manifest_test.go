// manifest_test.go covers the launch manifest Prepare writes: which items a launch holds, read back from its artifacts.

package claudeengine

import (
	"os"
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

// manifestFixture is an engine over a fixture machine.
// The machine has a global CLAUDE.md, an installed plugin with a session-start hook and a skill, and a user skill.
// The anchor is its own repository root holding a CLAUDE.md.
type manifestFixture struct {
	engine *Claude
	anchor string
	// globalFile is the user's global CLAUDE.md.
	globalFile string
	// hooksFile is the installed plugin's hooks file.
	hooksFile string
}

// newManifestFixture builds the fixture; claudeYAML, when set, is the claude.yaml layer of the anchor.
// markedLocal chooses whether the anchor's CLAUDE.local.md opens with heddle's marker, and localText is what it holds.
func newManifestFixture(t *testing.T, claudeYAML string, markedLocal bool, localText string) manifestFixture {
	t.Helper()
	engine, anchor := engineWithAnchor(t)
	if claudeYAML != "" {
		writeClaudeLayer(t, configengine.ConfigFile(anchor, "claude"), claudeYAML)
	}
	configDir := engine.claudeDir
	globalFile := filepath.Join(configDir, instructionFileName)
	writeUserSetupFile(t, globalFile, "global rules\n")
	writeSkillFile(t, configDir, "mine", "A user skill.")
	pluginRoot := filepath.Join(t.TempDir(), "scribe")
	writePluginInstalls(t, configDir, []pluginInstall{{ID: "scribe@market", Scope: pluginScopeUser, Root: pluginRoot}})
	writeSkillFile(t, pluginRoot, "prose", "Write tersely.")
	writeUserSetupFile(t, filepath.Join(pluginRoot, pluginHooksDirName, pluginHooksFileName), sessionStartHooksJSON("", 0, "emit"))

	for _, dir := range []string{"objects", "refs"} {
		if err := os.MkdirAll(filepath.Join(anchor, ".git", dir), 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
	}
	writeUserSetupFile(t, filepath.Join(anchor, ".git", "HEAD"), "ref: refs/heads/main\n")
	writeUserSetupFile(t, filepath.Join(anchor, instructionFileName), "repository rules\n")
	local := localText
	if markedLocal {
		local = heddleengine.Marker + "\n\n" + localText
	}
	writeUserSetupFile(t, filepath.Join(anchor, heddleengine.ClaudeLocalInstructionsFile), local)
	return manifestFixture{engine: engine, anchor: anchor, globalFile: globalFile, hooksFile: filepath.Join(pluginRoot, pluginHooksDirName, pluginHooksFileName)}
}

// shellRowNames returns the names of the shell guard rows hanging on sw, for a hub whose refuse list is empty.
func shellRowNames(sw shellguard.Switch) []string {
	var names []string
	for _, row := range shellguard.Rows(bashForegroundPhrase, nil) {
		if row.Switch == sw {
			names = append(names, row.Name)
		}
	}
	return names
}

// manifestItems returns a record's items of the given channel and key, matched on channel and key alone.
func manifestItems(m shuttleengine.Manifest, channel shuttleengine.ManifestChannel, key string) []shuttleengine.ManifestItem {
	var found []shuttleengine.ManifestItem
	for _, item := range m.Items {
		if item.Channel == channel && item.Key == key {
			found = append(found, item)
		}
	}
	return found
}

// TestPrepare_WritesTheManifest reads back the manifest Prepare writes for each kind of spec.
// Every row holds the items its spec and config deliver and none of the items they do not.
// It installs the process-wide passage reader, so it does not run in parallel.
func TestPrepare_WritesTheManifest(t *testing.T) {
	tiergate.Require(t, 0)

	house := shuttleengine.Standing{
		Passages:    []shuttleengine.Passage{{Slug: "house", Text: "house rule", Engine: engineName}},
		FileEntries: []shuttleengine.Passage{{Slug: "house-file", Text: "house file text"}},
	}
	item := func(channel shuttleengine.ManifestChannel, key string, delivery shuttleengine.ManifestDelivery) shuttleengine.ManifestItem {
		return shuttleengine.ManifestItem{Channel: channel, Key: key, Delivery: delivery}
	}
	guard := func(key string) shuttleengine.ManifestItem {
		return item(shuttleengine.ChannelGuard, key, shuttleengine.DeliveryDelivered)
	}
	guards := func(keys []string) []shuttleengine.ManifestItem {
		var items []shuttleengine.ManifestItem
		for _, key := range keys {
			items = append(items, guard(key))
		}
		return items
	}
	switched := append(guards(shellRowNames(shellguard.SwitchDenyPython)), guards(shellRowNames(shellguard.SwitchDenyPublish))...)

	base := shuttleengine.Spec{Prompt: "do it", PromptEngine: engineName, Role: "writer", ConfigRole: agentconfig.RoleDarn, Skills: []string{"scribe:prose"}}
	tests := []struct {
		name string
		spec shuttleengine.Spec
		// denyPython is the shuttle config's python switch.
		denyPython bool
		claudeYAML string
		// standing is the passages the installed reader returns; nil installs no reader.
		standing *shuttleengine.Standing
		unmarked bool
		// brokenHooksFile makes the plugin's hooks file unparseable.
		brokenHooksFile bool
		// unreadableGlobalFile makes the user's global CLAUDE.md a directory, so the setup resolution fails.
		unreadableGlobalFile bool
		// noRepository removes the anchor's repository.
		noRepository bool
		// blockedManifest plants a directory where manifest.json is written, so the write fails.
		blockedManifest bool
		// want and wantAbsent key "<global>" and "<hooks>" stand for the global file and the hooks file paths.
		want       []shuttleengine.ManifestItem
		wantAbsent []shuttleengine.ManifestItem
	}{
		{
			name: "AutonomousSpecWithSkills", spec: base,
			want: append(guards(slices.Concat(shellRowNames(shellguard.SwitchNone), shellRowNames(shellguard.SwitchDenyPython), shellRowNames(shellguard.SwitchDenyRawTmux))),
				item(shuttleengine.ChannelPromptEngine, "prompt-engine", shuttleengine.DeliveryDelivered),
				guard("question-deny"),
				item(shuttleengine.ChannelSkillLoad, "scribe:prose", shuttleengine.DeliveryDelivered),
				item(shuttleengine.ChannelNotice, "deny-notice", shuttleengine.DeliveryDelivered),
				item(shuttleengine.ChannelNotice, "creel-trust", shuttleengine.DeliveryDelivered),
				item(shuttleengine.ChannelNotice, "subagent-passage", shuttleengine.DeliveryDelivered),
				item(shuttleengine.ChannelBatchingNudge, "batching-nudge", shuttleengine.DeliveryDelivered),
				item(shuttleengine.ChannelInstructionSource, "<global>", shuttleengine.DeliveryNative),
				item(shuttleengine.ChannelInstructionSource, "CLAUDE.md", shuttleengine.DeliveryNative),
				item(shuttleengine.ChannelSkillListing, "scribe:prose", shuttleengine.DeliveryNative),
				item(shuttleengine.ChannelSkillListing, "mine", shuttleengine.DeliveryNative),
				item(shuttleengine.ChannelPluginContext, "scribe@market/emit", shuttleengine.DeliveryNative),
				shuttleengine.ManifestItem{Channel: shuttleengine.ChannelSettings, Key: "settings", Engine: engineName, Delivery: shuttleengine.DeliveryDelivered},
			),
			wantAbsent: append(guards(shellRowNames(shellguard.SwitchDenyPublish)), guard(shuttleengine.WebsterForkDenyKey)),
		},
		{
			name: "HandWrittenPrompt", spec: func() shuttleengine.Spec {
				spec := base
				spec.PromptEngine, spec.HandWrittenPrompt = "", true
				return spec
			}(),
			want:       []shuttleengine.ManifestItem{item(shuttleengine.ChannelPromptEngine, "hand-written", shuttleengine.DeliveryDelivered)},
			wantAbsent: []shuttleengine.ManifestItem{item(shuttleengine.ChannelPromptEngine, "prompt-engine", shuttleengine.DeliveryDelivered)},
		},
		{
			name: "InteractiveSpecHoldsNoQuestionDenyAndTheTmuxScriptRow", spec: func() shuttleengine.Spec {
				spec := base
				spec.Interactive = true
				return spec
			}(),
			want:       guards(shellRowNames(shellguard.SwitchDenyRawTmux)),
			wantAbsent: []shuttleengine.ManifestItem{guard("question-deny")},
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
			name: "ForkSubagentsHoldsTheWebsterForkDeny", spec: func() shuttleengine.Spec {
				spec := base
				spec.ForkSubagents = true
				return spec
			}(),
			want: []shuttleengine.ManifestItem{guard(shuttleengine.WebsterForkDenyKey)},
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
			name: "BatchingNudgeOffHoldsNoNudge", spec: base, claudeYAML: "batching_nudge:\n  enabled: false\n",
			wantAbsent: []shuttleengine.ManifestItem{item(shuttleengine.ChannelBatchingNudge, "batching-nudge", shuttleengine.DeliveryDelivered)},
		},
		{
			name: "InstalledReaderPassageAndFileEntry", spec: base, standing: &house,
			want: []shuttleengine.ManifestItem{
				{Channel: shuttleengine.ChannelStandingPassage, Key: "house", Engine: engineName, Delivery: shuttleengine.DeliveryDelivered},
				item(shuttleengine.ChannelInstructionSource, "heddle-file/house-file", shuttleengine.DeliveryDelivered),
			},
		},
		{
			name: "UnmarkedLocalFileHoldsNoFileEntry", spec: base, standing: &house, unmarked: true,
			want: []shuttleengine.ManifestItem{
				{Channel: shuttleengine.ChannelStandingPassage, Key: "house", Engine: engineName, Delivery: shuttleengine.DeliveryDelivered},
			},
			wantAbsent: []shuttleengine.ManifestItem{item(shuttleengine.ChannelInstructionSource, "heddle-file/house-file", shuttleengine.DeliveryDelivered)},
		},
		{
			name: "UnparseableHooksFileIsOmittedByItsPath", spec: base, brokenHooksFile: true,
			want:       []shuttleengine.ManifestItem{item(shuttleengine.ChannelPluginContext, "<hooks>", shuttleengine.DeliveryOmitted)},
			wantAbsent: []shuttleengine.ManifestItem{item(shuttleengine.ChannelPluginContext, "scribe@market/emit", shuttleengine.DeliveryNative)},
		},
		{
			name: "FailedSetupResolutionListsNoNativeItems", spec: base, unreadableGlobalFile: true,
			want: []shuttleengine.ManifestItem{item(shuttleengine.ChannelInstructionSource, "CLAUDE.md", shuttleengine.DeliveryNative)},
			wantAbsent: []shuttleengine.ManifestItem{
				item(shuttleengine.ChannelInstructionSource, "<global>", shuttleengine.DeliveryNative),
				item(shuttleengine.ChannelSkillListing, "mine", shuttleengine.DeliveryNative),
				item(shuttleengine.ChannelPluginContext, "scribe@market/emit", shuttleengine.DeliveryNative),
			},
		},
		{
			name: "AnchorWithNoRepositoryKeysItsInstructionFileAsCLAUDEmd", spec: base, noRepository: true,
			want: []shuttleengine.ManifestItem{item(shuttleengine.ChannelInstructionSource, "CLAUDE.md", shuttleengine.DeliveryNative)},
		},
		{name: "UnwritableManifestRefusesTheLaunch", spec: base, blockedManifest: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newManifestFixture(t, tt.claudeYAML, !tt.unmarked, "house file text\n")
			if tt.brokenHooksFile {
				writeUserSetupFile(t, f.hooksFile, "not json")
			}
			if tt.unreadableGlobalFile {
				if err := os.Remove(f.globalFile); err != nil {
					t.Fatalf("Remove: %v", err)
				}
				if err := os.Mkdir(f.globalFile, 0o755); err != nil {
					t.Fatalf("Mkdir: %v", err)
				}
			}
			if tt.noRepository {
				if err := os.RemoveAll(filepath.Join(f.anchor, ".git")); err != nil {
					t.Fatalf("RemoveAll: %v", err)
				}
			}
			resolveKey := func(key string) string {
				switch key {
				case "<global>":
					return f.globalFile
				case "<hooks>":
					return f.hooksFile
				}
				return key
			}
			if tt.standing != nil {
				f.engine.dirs.Board = t.TempDir()
				standing := *tt.standing
				t.Cleanup(shuttleengine.InstallPassageReader(func(string, string, string) (shuttleengine.Standing, error) { return standing, nil }))
			}
			cfg := templateAgentsConfig(t)
			cfg.DenyPython = tt.denyPython
			runDir := t.TempDir()
			if tt.blockedManifest {
				if err := os.Mkdir(filepath.Join(runDir, shuttleengine.ManifestFileName), 0o755); err != nil {
					t.Fatalf("Mkdir: %v", err)
				}
				if _, err := f.engine.Prepare(runDir, tt.spec, cfg); err == nil || !strings.Contains(err.Error(), "way forward") {
					t.Errorf("Prepare() error = %v; want a refusal naming the way forward", err)
				}
				return
			}
			if _, err := f.engine.Prepare(runDir, tt.spec, cfg); err != nil {
				t.Fatalf("Prepare() error: %v", err)
			}
			got, found, err := shuttleengine.ReadManifest(filepath.Join(runDir, shuttleengine.ManifestFileName))
			if err != nil || !found {
				t.Fatalf("ReadManifest() = found %v, error %v; want the manifest Prepare wrote", found, err)
			}

			for _, want := range tt.want {
				want.Key = resolveKey(want.Key)
				if items := manifestItems(got, want.Channel, want.Key); len(items) != 1 || items[0].Engine != want.Engine || items[0].Delivery != want.Delivery {
					t.Errorf("manifest items %s/%s = %+v; want one with engine %q and delivery %q", want.Channel, want.Key, items, want.Engine, want.Delivery)
				}
			}
			for _, absent := range tt.wantAbsent {
				if items := manifestItems(got, absent.Channel, resolveKey(absent.Key)); len(items) != 0 {
					t.Errorf("manifest items %s/%s = %+v; want none", absent.Channel, absent.Key, items)
				}
			}
			var wantVerdict []string
			if tt.brokenHooksFile {
				wantVerdict = []string{f.hooksFile}
			}
			var gotVerdict []string
			for _, omitted := range got.Verdict {
				gotVerdict = append(gotVerdict, omitted.Key)
			}
			if !slices.Equal(gotVerdict, wantVerdict) {
				t.Errorf("Verdict keys = %v; want %v, the omitted items alone", gotVerdict, wantVerdict)
			}
			if got.Engine != engineName || got.Role != tt.spec.Role || got.PromptEngine != tt.spec.PromptEngine {
				t.Errorf("manifest header = %q, %q, %q; want %q, %q, %q", got.Engine, got.Role, got.PromptEngine, engineName, tt.spec.Role, tt.spec.PromptEngine)
			}
		})
	}
}
