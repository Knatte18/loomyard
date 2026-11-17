// active_test.go covers the active-implementer seam over temp anchors with planted state files.

package weaverengine_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/gatefixer"
	"github.com/Knatte18/loomyard/internal/runplan"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/weaverengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// stateKind is how a test plants one implementer's state file.
type stateKind int

const (
	stateAbsent stateKind = iota
	stateEmpty
	stateBegun
	stateCorrupt
)

// plantState writes one implementer's state file under anchor in the given kind.
func plantState(t *testing.T, anchor string, impl weaverengine.Implementer, kind stateKind) {
	t.Helper()
	if kind == stateAbsent {
		return
	}
	runDir, scratchDir := weaverengine.RunDirs(impl, anchor)
	batches := map[int]*runplan.BatchState{}
	if kind == stateBegun {
		batches[1] = &runplan.BatchState{Slug: "one"}
	}
	switch {
	case kind == stateCorrupt:
		if err := os.MkdirAll(runDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(runDir, "state.json"), []byte("{not json"), 0o644); err != nil {
			t.Fatal(err)
		}
	case impl == weaverengine.ImplementerWeaver:
		if err := weaverengine.SaveState(runDir, scratchDir, &weaverengine.State{State: runplan.State{Batches: batches}}); err != nil {
			t.Fatal(err)
		}
	default:
		if err := websterengine.SaveState(runDir, scratchDir, &websterengine.State{State: runplan.State{Batches: batches}}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestActive(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	setting := func(value string) func() (string, error) {
		return func() (string, error) { return value, nil }
	}
	versionAt := func(commit, lastEnd string) weaverengine.StateVersion {
		data, err := json.Marshal(weaverengine.State{LastEnd: lastEnd})
		if err != nil {
			t.Fatal(err)
		}
		return weaverengine.StateVersion{Commit: commit, Data: data}
	}
	strand := func(role string) string {
		name, err := agentname.Format("sn", "slug", role)
		if err != nil {
			t.Fatal(err)
		}
		return name
	}
	history := func(versions ...weaverengine.StateVersion) func() ([]weaverengine.StateVersion, error) {
		return func() ([]weaverengine.StateVersion, error) { return versions, nil }
	}
	head := func(sha string) func() (string, error) { return func() (string, error) { return sha, nil } }
	live := func(names ...string) func() ([]string, error) { return func() ([]string, error) { return names, nil } }

	tests := []struct {
		name        string
		webster     stateKind
		weaver      stateKind
		deps        weaverengine.ActiveDeps
		want        weaverengine.Implementer
		wantState   bool
		wantRefused bool
		wantInText  []string
		wantNotText []string
	}{
		{name: "neither file with setting webster", deps: weaverengine.ActiveDeps{Setting: setting("webster")}, want: weaverengine.ImplementerWebster},
		{name: "neither file with setting weaver", deps: weaverengine.ActiveDeps{Setting: setting("weaver")}, want: weaverengine.ImplementerWeaver},
		{name: "neither file with a nil setting", want: weaverengine.ImplementerWebster},
		{
			name: "setting error", wantRefused: true, wantInText: []string{"lym config tapestry"},
			deps: weaverengine.ActiveDeps{Setting: func() (string, error) { return "", errors.New("unreadable") }},
		},
		{name: "unknown setting value", deps: weaverengine.ActiveDeps{Setting: setting("other")}, wantRefused: true, wantInText: []string{"lym config tapestry"}},
		{name: "begun Webster wins over setting weaver", webster: stateBegun, deps: weaverengine.ActiveDeps{Setting: setting("weaver")}, want: weaverengine.ImplementerWebster, wantState: true},
		{name: "begun Weaver wins over setting webster", weaver: stateBegun, deps: weaverengine.ActiveDeps{Setting: setting("webster")}, want: weaverengine.ImplementerWeaver, wantState: true},
		{name: "unbegun state follows the setting", webster: stateEmpty, weaver: stateEmpty, deps: weaverengine.ActiveDeps{Setting: setting("weaver")}, want: weaverengine.ImplementerWeaver, wantState: true},
		{
			name: "both begun", webster: stateBegun, weaver: stateBegun, deps: weaverengine.ActiveDeps{Setting: setting("weaver")},
			wantRefused: true, wantInText: []string{"lym webster reset --to start", "lym weaver reset"}, wantNotText: []string{"lym config tapestry"},
		},
		{name: "corrupt Webster file", webster: stateCorrupt, wantRefused: true, wantInText: []string{"_lym/webster/state.json", "repair"}},
		{
			name: "corrupt Webster file with Weaver begun", webster: stateCorrupt, weaver: stateBegun,
			wantRefused: true, wantInText: []string{"_lym/webster/state.json", "repair"},
		},
		{
			name: "corrupt Weaver file restores the newest parsing version", weaver: stateCorrupt, wantRefused: true,
			deps: weaverengine.ActiveDeps{
				StateHistory: history(versionAt("c-new", "h1"), versionAt("c-old", "h0")), TaskHead: head("h1"), LiveStrands: live(strand("reviewer")),
			},
			wantInText: []string{"_lym/weaver/state.json", "c-new", "restore"}, wantNotText: []string{"c-old"},
		},
		{
			name: "corrupt Weaver file with HEAD moved", weaver: stateCorrupt, wantRefused: true,
			deps:       weaverengine.ActiveDeps{StateHistory: history(versionAt("c-new", "h1")), TaskHead: head("h2"), LiveStrands: live()},
			wantInText: []string{"lym weaver reset"}, wantNotText: []string{"c-new"},
		},
		{
			name: "corrupt Weaver file with a live batch strand", weaver: stateCorrupt, wantRefused: true,
			deps:       weaverengine.ActiveDeps{StateHistory: history(versionAt("c-new", "h1")), TaskHead: head("h1"), LiveStrands: live(strand("weaver-2"))},
			wantInText: []string{"lym weaver reset"}, wantNotText: []string{"c-new"},
		},
		{
			name: "corrupt Weaver file with a live verify fixer", weaver: stateCorrupt, wantRefused: true,
			deps:       weaverengine.ActiveDeps{StateHistory: history(versionAt("c-new", "h1")), TaskHead: head("h1"), LiveStrands: live(strand(gatefixer.RoleVerifyFixer))},
			wantInText: []string{"lym weaver reset"}, wantNotText: []string{"c-new"},
		},
		{
			name: "corrupt Weaver file with no parsing version", weaver: stateCorrupt, wantRefused: true,
			deps: weaverengine.ActiveDeps{
				StateHistory: history(weaverengine.StateVersion{Commit: "c-bad", Data: []byte("{")}), TaskHead: head("h1"), LiveStrands: live(),
			},
			wantInText: []string{"lym weaver reset"}, wantNotText: []string{"c-bad"},
		},
		{
			name: "corrupt Weaver file with a failing history read", weaver: stateCorrupt, wantRefused: true,
			deps: weaverengine.ActiveDeps{
				StateHistory: func() ([]weaverengine.StateVersion, error) { return nil, errors.New("no history") }, TaskHead: head("h1"), LiveStrands: live(),
			},
			wantInText: []string{"lym weaver reset"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			anchor := t.TempDir()
			plantState(t, anchor, weaverengine.ImplementerWebster, tt.webster)
			plantState(t, anchor, weaverengine.ImplementerWeaver, tt.weaver)
			deps := tt.deps
			deps.AnchorRoot = anchor

			got, st, err := weaverengine.Active(deps)
			if tt.wantRefused {
				if !errors.Is(err, weaverengine.ErrActiveRefused) {
					t.Fatalf("Active() error = %v; want one wrapping ErrActiveRefused", err)
				}
				for _, text := range tt.wantInText {
					if !strings.Contains(err.Error(), text) {
						t.Errorf("Active() error = %q; want it to contain %q", err, text)
					}
				}
				for _, text := range tt.wantNotText {
					if strings.Contains(err.Error(), text) {
						t.Errorf("Active() error = %q; want it free of %q", err, text)
					}
				}
				if conflict := tt.webster == stateBegun && tt.weaver == stateBegun; conflict != errors.Is(err, weaverengine.ErrImplementerConflict) {
					t.Errorf("Active() error = %v; ErrImplementerConflict match want %v", err, conflict)
				}
			} else {
				if err != nil {
					t.Fatalf("Active() error = %v; want none", err)
				}
				if got != tt.want || (st != nil) != tt.wantState {
					t.Errorf("Active() = (%q, state %v); want (%q, state %v)", got, st != nil, tt.want, tt.wantState)
				}
			}

			advisory, advisoryState, reason := weaverengine.ActiveOrSetting(deps)
			if !tt.wantRefused {
				if advisory != tt.want || (advisoryState != nil) != tt.wantState || reason != "" {
					t.Errorf("ActiveOrSetting() = (%q, state %v, %q); want (%q, state %v, no reason)", advisory, advisoryState != nil, reason, tt.want, tt.wantState)
				}
				return
			}
			if reason != err.Error() {
				t.Errorf("ActiveOrSetting() reason = %q; want the refusal %q", reason, err)
			}
			wantAdvisory := weaverengine.ImplementerWebster
			if value, settingErr := callSetting(deps); settingErr == nil && value == "weaver" {
				wantAdvisory = weaverengine.ImplementerWeaver
			}
			if advisory != wantAdvisory {
				t.Errorf("ActiveOrSetting() implementer = %q; want the setting's %q", advisory, wantAdvisory)
			}
		})
	}
}

// callSetting reads deps' setting, answering webster for a nil one.
func callSetting(deps weaverengine.ActiveDeps) (string, error) {
	if deps.Setting == nil {
		return "webster", nil
	}
	return deps.Setting()
}
