// settings_test.go drives the claude-settings target:
// folding its entries, merging them into the file and taking out what no entry sets any more.

package heddleengine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/agentconfig"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// The members the derived agent-defaults entry adds to the settings object while no hub agents.yaml sets an effort.
// derivedBesideEnvA is the same pair with the env object also holding the key "A".
const (
	derivedEffort     = `"effortLevel": "medium"`
	derivedEnv        = `"` + agentconfig.SubagentEffortEnv + `": "medium"`
	derivedSettings   = derivedEffort + `, "env": {` + derivedEnv + `}`
	derivedBesideEnvA = derivedEffort + `, "env": {"A": "1", ` + derivedEnv + `}`
)

// invalidAgentsConfig is an agents.yaml whose efforts table has no default, so it fails to load.
const invalidAgentsConfig = "efforts:\n  haiku: low\n"

// writeAgentsConfig writes content as the hub's agents.yaml in the Bolt checkout at bolt.
func writeAgentsConfig(t *testing.T, bolt, content string) {
	t.Helper()
	path := configengine.ConfigFile(bolt, agentconfig.ModuleName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// settingsStore returns a repository store holding one claude-settings entry per text, in order, each set through Set.
func settingsStore(t *testing.T, texts ...string) Store {
	t.Helper()
	var store Store
	for i, text := range texts {
		slug := fmt.Sprintf("settings-%d", i)
		if err := store.Set(slug, Patch{Text: textOf(text), Order: orderOf(i), Targets: []string{claudeSettingsTarget}}); err != nil {
			t.Fatalf("Set(%s) = %v", slug, err)
		}
	}
	return store
}

// requireJSON fails unless got and want decode to the same JSON value.
func requireJSON(t *testing.T, label, got, want string) {
	t.Helper()
	var gotValue, wantValue any
	if err := json.Unmarshal([]byte(got), &gotValue); err != nil {
		t.Fatalf("%s = %q does not decode: %v", label, got, err)
	}
	if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
		t.Fatalf("want %q does not decode: %v", want, err)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Errorf("%s = %s; want %s", label, got, want)
	}
}

// TestRenderClaudeSettings is not parallel: it replaces shippedStore, which every render reads.
func TestRenderClaudeSettings(t *testing.T) {
	tiergate.Require(t, 0)
	const shipped = `{"entries": [{"slug": "ship-settings", "order": 1, "targets": ["claude-settings"],
  "text": "{\"permissions\": {\"deny\": [\"Workflow\"]}, \"attribution\": {\"commit\": \"\"}}"}]}`
	const noShipped = `{"entries": []}`

	tests := []struct {
		name    string
		shipped string
		// agents is the hub's agents.yaml; empty leaves none, so the template's efforts apply.
		agents string
		// file is the settings file present before the first render; empty leaves none.
		file string
		// earlier holds the entry texts of a render before the checked one; nil renders once.
		earlier []string
		// editBetween replaces the settings file after the earlier render; empty leaves it.
		editBetween string
		// recordOverwrite replaces the record after the earlier render; empty leaves it.
		recordOverwrite string
		// invalidText replaces the repository's entries with one whose text holds no JSON object at the checked render.
		invalidText string
		// texts are the repository's claude-settings entry texts at the checked render.
		texts []string
		// excluded replaces the fixture's exclude list when not nil.
		excluded []string
		// want is the file after the checked render; empty wants no file.
		want                                    string
		wantWritten, wantRemoved, wantUnwritten bool
		// wantWarning is a text one warning holds; empty wants no warning.
		wantWarning string
	}{
		{
			name: "NoFileTakesTheEntriesFoldedInOrder", shipped: shipped,
			texts: []string{`{"permissions": {"deny": ["Artifact", "Workflow"]}}`, `{"attribution": {"commit": "Made by us"}, "permissions": {"deny": ["Artifact"]}}`},
			want:  `{"permissions": {"deny": ["Workflow", "Artifact"]}, "attribution": {"commit": "Made by us"}, ` + derivedSettings + `}`, wantWritten: true,
		},
		{
			name: "RepositoryEntryWinsOverTheDerivedEffort", shipped: noShipped,
			texts: []string{`{"effortLevel": "high"}`},
			want:  `{"effortLevel": "high", "env": {` + derivedEnv + `}}`, wantWritten: true,
		},
		{
			name: "FileKeysStayAndHeddleScalarsWin", shipped: shipped,
			file: `{"model": "opus", "permissions": {"allow": ["Bash"], "deny": ["Mine"]}, "attribution": {"commit": "old", "pr": "old"}}`,
			want: `{"model": "opus", "permissions": {"allow": ["Bash"], "deny": ["Mine", "Workflow"]}, "attribution": {"commit": "", "pr": "old"}, ` + derivedSettings + `}`, wantWritten: true,
		},
		{
			name: "WhatNoEntrySetsAnyMoreIsTakenOut", shipped: shipped,
			file:    `{"permissions": {"allow": ["Bash"]}}`,
			earlier: []string{`{"permissions": {"deny": ["Artifact"]}, "env": {"A": "1"}}`},
			want:    `{"permissions": {"allow": ["Bash"], "deny": ["Workflow"]}, "attribution": {"commit": ""}, ` + derivedSettings + `}`, wantWritten: true,
		},
		{
			name: "NoEntryLeftRemovesTheFileTheTakingOutEmpties", shipped: noShipped, agents: invalidAgentsConfig,
			earlier: []string{`{"env": {"A": "1"}}`}, wantRemoved: true, wantWarning: "effort defaults",
		},
		{
			name: "NoEntryLeftKeepsTheKeysHeddleNeverSet", shipped: noShipped,
			file:    `{"model": "opus"}`,
			earlier: []string{`{"env": {"A": "1"}}`},
			want:    `{"model": "opus", ` + derivedSettings + `}`, wantWritten: true,
		},
		{
			name: "ScalarChangedSinceStaysWhenNoEntrySetsIt", shipped: noShipped,
			earlier:     []string{`{"attribution": {"commit": ""}}`},
			editBetween: `{"attribution": {"commit": "mine"}}`,
			want:        `{"attribution": {"commit": "mine"}, ` + derivedSettings + `}`, wantWritten: true,
		},
		{
			name: "UserItemInAListHeddleOwnsStaysWhenTheEntryGoes", shipped: noShipped,
			file:    `{"permissions": {"deny": ["Mine"]}}`,
			earlier: []string{`{"permissions": {"deny": ["Artifact"]}}`},
			want:    `{"permissions": {"deny": ["Mine"]}, ` + derivedSettings + `}`, wantWritten: true,
		},
		{
			name: "NoEntryAndNoRecordTouchesNothing", shipped: noShipped, agents: invalidAgentsConfig,
			file: `{"model": "opus"}`, want: `{"model": "opus"}`, wantWarning: "effort defaults",
		},
		{
			name: "BlankFileTakesTheEntries", shipped: shipped,
			file: "  \n",
			want: `{"permissions": {"deny": ["Workflow"]}, "attribution": {"commit": ""}, ` + derivedSettings + `}`, wantWritten: true,
		},
		{
			name: "RecordHoldingNoObjectTakesNothingOut", shipped: shipped,
			earlier:         []string{`{"env": {"A": "1"}}`},
			recordOverwrite: `["not", "an", "object"]`,
			want:            `{"permissions": {"deny": ["Workflow"]}, "attribution": {"commit": ""}, ` + derivedBesideEnvA + `}`, wantWritten: true,
		},
		{
			name: "EntryHoldingNoObjectKeepsWhatTheLastRenderSet", shipped: noShipped,
			earlier:     []string{`{"env": {"A": "1"}}`},
			invalidText: "not json",
			want:        `{` + derivedBesideEnvA + `}`, wantWritten: true, wantWarning: "holds no JSON object",
		},
		{
			name: "FileHoldingNoObjectIsLeftAlone", shipped: shipped,
			file: `["not", "an", "object"]`, want: `["not", "an", "object"]`,
			wantUnwritten: true, wantWarning: "holds no JSON object",
		},
		{
			name: "OffExcludedFileIsNotWritten", shipped: shipped,
			excluded:      []string{"CLAUDE.local.md", ".heddle"},
			wantUnwritten: true, wantWarning: fabricengine.ExcludeWayForward(ClaudeSettingsFile),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			useShippedLayer(t, tt.shipped)
			f := newRenderFixture(t, settingsStore(t, tt.earlier...))
			if tt.agents != "" {
				writeAgentsConfig(t, f.bolt, tt.agents)
			}
			if tt.excluded != nil {
				f.excluded = tt.excluded
			}
			if tt.file != "" {
				f.writeAnchor(t, ClaudeSettingsFile, tt.file)
			}
			if tt.earlier != nil {
				mustRender(t, f)
			}
			if tt.editBetween != "" {
				f.writeAnchor(t, ClaudeSettingsFile, tt.editBetween)
			}
			if tt.recordOverwrite != "" {
				recordPath := filepath.Join(f.scratch, lymdirs.HeddleDirName, claudeSettingsTarget+recordSuffix)
				if err := os.WriteFile(recordPath, []byte(tt.recordOverwrite), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			store := settingsStore(t, tt.texts...)
			if tt.invalidText != "" {
				store = Store{Entries: []Entry{entryFor("settings-bad", tt.invalidText, 0, claudeSettingsTarget)}}
			}
			saveStore(t, f.bolt, store)

			report := mustRender(t, f)

			if tt.want == "" {
				if f.anchorHas(ClaudeSettingsFile) {
					t.Errorf("%s = %q; want no file", ClaudeSettingsFile, f.readAnchor(t, ClaudeSettingsFile))
				}
			} else {
				requireJSON(t, ClaudeSettingsFile, f.readAnchor(t, ClaudeSettingsFile), tt.want)
			}
			if got := slices.Contains(report.Written, ClaudeSettingsFile); got != tt.wantWritten {
				t.Errorf("written %v; want the settings file written: %v", report.Written, tt.wantWritten)
			}
			if got := slices.Contains(report.Removed, ClaudeSettingsFile); got != tt.wantRemoved {
				t.Errorf("removed %v; want the settings file removed: %v", report.Removed, tt.wantRemoved)
			}
			if got := slices.Contains(report.Unwritten, ClaudeSettingsFile); got != tt.wantUnwritten {
				t.Errorf("unwritten %v; want the settings file unwritten: %v", report.Unwritten, tt.wantUnwritten)
			}
			if tt.wantWarning == "" {
				if len(report.Warnings) != 0 {
					t.Errorf("warnings = %v; want none", report.Warnings)
				}
			} else if !slices.ContainsFunc(report.Warnings, func(warning string) bool { return strings.Contains(warning, tt.wantWarning) }) {
				t.Errorf("warnings = %v; want one holding %q", report.Warnings, tt.wantWarning)
			}
		})
	}
}

// TestRenderClaudeSettingsDropsTheRecordWithTheLastEntry is not parallel:
// it replaces shippedStore, which every render reads.
func TestRenderClaudeSettingsDropsTheRecordWithTheLastEntry(t *testing.T) {
	tiergate.Require(t, 0)
	useShippedLayer(t, `{"entries": []}`)
	f := newRenderFixture(t, settingsStore(t, `{"env": {"A": "1"}}`))
	f.writeAnchor(t, ClaudeSettingsFile, `{"model": "opus"}`)
	mustRender(t, f)
	saveStore(t, f.bolt, Store{})
	mustRender(t, f)
	requireJSON(t, ClaudeSettingsFile, f.readAnchor(t, ClaudeSettingsFile), `{"model": "opus", `+derivedSettings+`}`)

	f.writeAnchor(t, ClaudeSettingsFile, `{"model": "opus", "env": {"A": "1"}}`)
	saveStore(t, f.bolt, settingsStore(t, `{"attribution": {"commit": "x"}}`))
	mustRender(t, f)

	requireJSON(t, ClaudeSettingsFile, f.readAnchor(t, ClaudeSettingsFile),
		`{"model": "opus", "attribution": {"commit": "x"}, `+derivedBesideEnvA+`}`)
}

// TestShippedClaudeSettings pins the Claude settings the shipped layer sets in every worktree.
// It also pins the effort defaults derived from the template's agents.yaml.
// It pins that a first render writes them as Show prints them.
func TestShippedClaudeSettings(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	f := newRenderFixture(t, Store{})

	shown, err := Renderer{}.Show(f.bolt, claudeSettingsTarget, f.anchor)
	if err != nil {
		t.Fatalf("Show(%s) = %v", claudeSettingsTarget, err)
	}
	requireJSON(t, "Show("+claudeSettingsTarget+")", shown, `{
  "attribution": {"commit": "", "pr": ""},
  "enabledPlugins": {"ly@loomyard": true, "scribe@scribe": true},
  "extraKnownMarketplaces": {"scribe": {"source": {"source": "github", "repo": "Knatte18/scribe"}}},
  "permissions": {"deny": ["Artifact", "Workflow", "ReportFindings", "ScheduleWakeup", "SendFeedback",
    "CronCreate", "CronDelete", "CronList", "RemoteTrigger", "DesignSync", "NotebookEdit",
    "EnterWorktree", "ExitWorktree", "Skill(dataviz)"]},
  "disableClaudeAiConnectors": true,
  "disableBundledSkills": true,
  "disableWorkflows": true,
  "promptSuggestionEnabled": false,
  `+derivedSettings+`
}`)

	mustRender(t, f)
	if got := f.readAnchor(t, ClaudeSettingsFile); got != shown {
		t.Errorf("%s = %q; want the text Show printed, %q", ClaudeSettingsFile, got, shown)
	}
}

// TestClaudeSettingsCarryTheAgentDefaults pins the effortLevel and subagent effort variable of Show and Render.
// Both hold the efforts table's default of the hub's agents.yaml.
// An agents.yaml which fails to load leaves both out and makes Render warn.
func TestClaudeSettingsCarryTheAgentDefaults(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name   string
		agents string
		// wantEffort is the effort both keys hold; empty wants both left out and a warning.
		wantEffort string
	}{
		{"NoAgentsFileTakesTheTemplateDefault", "", "medium"},
		{"HubAgentsFileDefaultWins", "efforts:\n  default: high\n", "high"},
		{"UnloadableAgentsFileLeavesTheDefaultsOut", invalidAgentsConfig, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newRenderFixture(t, Store{})
			if tt.agents != "" {
				writeAgentsConfig(t, f.bolt, tt.agents)
			}
			shown, err := Renderer{}.Show(f.bolt, claudeSettingsTarget, f.anchor)
			if err != nil {
				t.Fatalf("Show(%s) = %v", claudeSettingsTarget, err)
			}
			report := mustRender(t, f)

			for label, text := range map[string]string{"Show": shown, ClaudeSettingsFile: f.readAnchor(t, ClaudeSettingsFile)} {
				var object struct {
					EffortLevel string            `json:"effortLevel"`
					Env         map[string]string `json:"env"`
				}
				if err := json.Unmarshal([]byte(text), &object); err != nil {
					t.Fatalf("%s = %q does not decode: %v", label, text, err)
				}
				if object.EffortLevel != tt.wantEffort || object.Env[agentconfig.SubagentEffortEnv] != tt.wantEffort {
					t.Errorf("%s holds effortLevel %q and %s %q; want both %q",
						label, object.EffortLevel, agentconfig.SubagentEffortEnv, object.Env[agentconfig.SubagentEffortEnv], tt.wantEffort)
				}
			}
			warned := slices.ContainsFunc(report.Warnings, func(warning string) bool { return strings.Contains(warning, "effort defaults") })
			if warned != (tt.wantEffort == "") {
				t.Errorf("warnings = %v; want a warning naming the effort defaults: %v", report.Warnings, tt.wantEffort == "")
			}
		})
	}
}
