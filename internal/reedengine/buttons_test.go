// buttons_test.go pins the decoding and validation of button entries and the placeholder filling of their run, confirm and ask text.

package reedengine

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shell"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"gopkg.in/yaml.v3"
)

// buttonsNode decodes doc, a YAML document holding a buttons key, into the buttons node.
func buttonsNode(t *testing.T, doc string) yaml.Node {
	t.Helper()
	var holder struct {
		Buttons yaml.Node `yaml:"buttons"`
	}
	if err := yaml.Unmarshal([]byte(doc), &holder); err != nil {
		t.Fatalf("unmarshal %q: %v", doc, err)
	}
	return holder.Buttons
}

// TestPlaceButtons pins which entries each place takes from a buttons node and which invalid shapes are skipped with a Warn.
// It captures the process-global logger output, so it does not run in parallel.
func TestPlaceButtons(t *testing.T) {
	tiergate.Require(t, 0)
	const source = "reed.yaml (test)"
	templateButtons := buttonsNode(t, ConfigTemplate())

	tests := []struct {
		name       string
		buttons    yaml.Node
		place      ButtonPlace
		wantLabels []string
		// wantWarn holds substrings of the captured log; empty means nothing is logged.
		wantWarn []string
	}{
		{name: "TemplateTaskGroupComesBackWhole", buttons: templateButtons, place: ButtonPlaceTask, wantLabels: []string{"HALT", "CANCEL", "code", "terminal", "agent"}},
		{name: "EmptyPrimeTakesBeam", buttons: buttonsNode(t, "buttons: {beam: [{label: b, run: r}], prime: []}"), place: ButtonPlacePrime, wantLabels: []string{"b"}},
		{name: "NonEmptyPrimeTakesOnlyPrime", buttons: buttonsNode(t, "buttons: {beam: [{label: b, run: r}], prime: [{label: p, run: r}]}"), place: ButtonPlacePrime, wantLabels: []string{"p"}},
		{name: "PrimeAsMappingFallsBackToBeam", buttons: buttonsNode(t, "buttons: {beam: [{label: b, run: r}], prime: {label: p}}"), place: ButtonPlacePrime, wantLabels: []string{"b"}, wantWarn: []string{source, "prime"}},
		{name: "ZeroNodeGivesNone", buttons: yaml.Node{}, place: ButtonPlaceTask},
		{name: "AbsentGroupGivesNone", buttons: buttonsNode(t, "buttons: {beam: [{label: b, run: r}]}"), place: ButtonPlaceTask},
		{name: "GroupAsMappingIsSkipped", buttons: buttonsNode(t, "buttons: {task: {label: x}}"), place: ButtonPlaceTask, wantWarn: []string{source, "task"}},
		{name: "ScalarEntryIsSkipped", buttons: buttonsNode(t, "buttons: {task: [oops, {label: ok, run: r}]}"), place: ButtonPlaceTask, wantLabels: []string{"ok"}, wantWarn: []string{source, "task", "index=0"}},
		{name: "LabelAsListIsSkipped", buttons: buttonsNode(t, "buttons: {task: [{label: [a], run: r}, {label: ok, run: r}]}"), place: ButtonPlaceTask, wantLabels: []string{"ok"}, wantWarn: []string{source, "task", "index=0"}},
		{name: "WindowAgentsWithStrandIsKept", buttons: buttonsNode(t, "buttons: {task: [{label: k, run: 'x {strand}', confirm: 'y {strand}', window: agents}]}"), place: ButtonPlaceTask, wantLabels: []string{"k"}},
		{name: "AskWithAnswerAndDefaultIsKept", buttons: buttonsNode(t, "buttons: {task: [{label: k, run: 'x {answer}', confirm: 'y {answer}?', ask: 'Name:', ask_default: d}]}"), place: ButtonPlaceTask, wantLabels: []string{"k"}},
	}
	invalid := []struct{ name, entry string }{
		{"EmptyLabel", "{label: '', run: r}"},
		{"EmptyRun", "{label: l, run: ''}"},
		{"HashInLabel", "{label: 'a#b', run: r}"},
		{"HashInConfirm", "{label: l, run: r, confirm: 'a#b'}"},
		{"PercentInLabel", "{label: 'a%b', run: r}"},
		{"PercentInConfirm", "{label: l, run: r, confirm: 'a%b'}"},
		{"TabInLabel", `{label: "a\tb", run: r}`},
		{"TabInConfirm", `{label: l, run: r, confirm: "a\tb"}`},
		{"WindowAll", "{label: l, run: r, window: all}"},
		{"StrandInRunWithoutAgents", "{label: l, run: 'x {strand}'}"},
		{"StrandInConfirmWithoutAgents", "{label: l, run: r, confirm: 'x {strand}'}"},
		{"StrandInAskWithoutAgents", "{label: l, run: r, ask: 'x {strand}'}"},
		{"HashInAsk", "{label: l, run: r, ask: 'a#b'}"},
		{"PercentInAsk", "{label: l, run: r, ask: 'a%b'}"},
		{"TabInAsk", `{label: l, run: r, ask: "a\tb"}`},
		{"CommaInAsk", "{label: l, run: r, ask: 'a,b'}"},
		{"AnswerInRunWithoutAsk", "{label: l, run: 'x {answer}'}"},
		{"AnswerInConfirmWithoutAsk", "{label: l, run: r, confirm: 'x {answer}'}"},
		{"AskDefaultWithoutAsk", "{label: l, run: r, ask_default: d}"},
	}
	for _, bad := range invalid {
		tests = append(tests, struct {
			name       string
			buttons    yaml.Node
			place      ButtonPlace
			wantLabels []string
			wantWarn   []string
		}{
			name:       bad.name + "IsSkippedBesideAValidNeighbour",
			buttons:    buttonsNode(t, fmt.Sprintf("buttons: {task: [%s, {label: ok, run: r}]}", bad.entry)),
			place:      ButtonPlaceTask,
			wantLabels: []string{"ok"},
			wantWarn:   []string{source, "task", "index=0"},
		})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := logcapture.CaptureVerbose(t)
			var gotLabels []string
			for _, entry := range placeButtons(tt.buttons, tt.place, source) {
				gotLabels = append(gotLabels, entry.Label)
			}
			if !slices.Equal(gotLabels, tt.wantLabels) {
				t.Errorf("placeButtons labels = %v; want %v", gotLabels, tt.wantLabels)
			}
			if len(tt.wantWarn) == 0 && logs.String() != "" {
				t.Errorf("placeButtons logged %q; want nothing", logs.String())
			}
			for _, want := range tt.wantWarn {
				if !strings.Contains(logs.String(), want) {
					t.Errorf("placeButtons log = %q; want it to contain %q", logs.String(), want)
				}
			}
		})
	}
}

// TestFillButtonPlaceholders pins the quoting of run values, the literal brace text, and the doubling of prompt values, in both dialects.
func TestFillButtonPlaceholders(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	for name, sh := range map[string]shell.Shell{"posix": shell.Posix(), "pwsh": shell.Pwsh()} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			tests := []struct {
				name   string
				run    string
				values map[string]string
				want   string
			}{
				{"WorktreeValueIsQuoted", "cd {worktree}", map[string]string{"worktree": "/a b/c"}, "cd " + sh.Quote("/a b/c")},
				{"AwkProgramAndStrandStayLiteral", "awk '{print $1}' {strand}", map[string]string{"slug": "s"}, "awk '{print $1}' {strand}"},
				{"UnknownPlaceholderStaysLiteral", "echo {x}", map[string]string{"slug": "s"}, "echo {x}"},
			}
			for _, tt := range tests {
				if got := fillRunPlaceholders(sh, tt.run, tt.values); got != tt.want {
					t.Errorf("%s fillRunPlaceholders(%q) = %q; want %q", tt.name, tt.run, got, tt.want)
				}
			}
			if got := fillPromptPlaceholders("Cancel {slug}?", map[string]string{"slug": "a#b%c"}); got != "Cancel a##b%%c?" {
				t.Errorf("fillPromptPlaceholders = %q; want %q", got, "Cancel a##b%%c?")
			}
		})
	}
}
