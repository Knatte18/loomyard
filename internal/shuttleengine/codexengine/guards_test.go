// guards_test.go pins the shell guard rows a Codex launch installs.
// It also pins the hook entries rendered for them and the deny notice announcing them.

package codexengine

import (
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shellguard"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// sampleRefuseRaw is the non-empty refuse list the guard tests render the rows with.
var sampleRefuseRaw = []string{"go test", "dotnet test"}

// rowNames returns the names of rows, in order.
func rowNames(rows []shellguard.Row) []string {
	names := make([]string, len(rows))
	for i, row := range rows {
		names[i] = row.Name
	}
	return names
}

// TestInstalledGuards_CoverEveryRow pins that a launch with every switch on installs every row, in table order.
// The refuse-raw row is included for a non-empty refuse list.
// The python row is installed under every input.
// The publish row follows its switch.
// The tmux-script row is installed unless the launch exempts itself.
//
//testtiming:keep pins the literal installed row names per input, which TestPrepare_InstallsGuardsFromItsInputs derives from installedGuards itself
func TestInstalledGuards_CoverEveryRow(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	every := rowNames(shellguard.Rows(foregroundPhrase, sampleRefuseRaw))
	tests := []struct {
		name      string
		refuseRaw []string
		in        guardInputs
		want      []string
	}{
		{"EverySwitchOn", sampleRefuseRaw, guardInputs{denyPython: true, denyPublish: true}, every},
		{"SwitchesOff", sampleRefuseRaw, guardInputs{}, slices.DeleteFunc(slices.Clone(every), func(name string) bool { return name == "publish" })},
		{"RawTmuxAllowed", sampleRefuseRaw, guardInputs{denyPython: true, denyPublish: true, allowRawTmux: true}, slices.DeleteFunc(slices.Clone(every), func(name string) bool { return name == "tmux-script" })},
		{"NoRefuseList", nil, guardInputs{denyPython: true, denyPublish: true}, slices.DeleteFunc(slices.Clone(every), func(name string) bool { return name == "refuse-raw" })},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := rowNames(installedGuards(tt.refuseRaw, tt.in)); !slices.Equal(got, tt.want) {
				t.Errorf("installedGuards() = %v; want %v", got, tt.want)
			}
		})
	}
}

// TestGuardEntries_RenderEveryRowWithItsCodexText pins the literal deny output Codex honours.
// It pins one entry per row: each opens with the literal Bash matcher and carries the row's Codex steer in its deny.
// It also pins a deny notice of the rows' Codex notices in order.
// That notice names the python row only while the launch's config denies Python.
//
//testtiming:keep pins the literal deny JSON and the deny notice order, question notice first, which TestPrepare_CreelHooksInAHub does not read
func TestGuardEntries_RenderEveryRowWithItsCodexText(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	if got, want := denyDecision("x"), `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"x"}}`; got != want {
		t.Errorf("denyDecision(x) = %s; want %s", got, want)
	}
	every := guardInputs{denyPython: true, denyPublish: true}
	rows := installedGuards(sampleRefuseRaw, every)
	entries := guardEntries(rows, testLymPath)
	if len(entries) != len(rows) {
		t.Fatalf("guardEntries() = %d entries; want one per row, %d", len(entries), len(rows))
	}
	var notices, noticesWithoutPython []string
	for i, row := range rows {
		if row.Name != "python" {
			noticesWithoutPython = append(noticesWithoutPython, row.NoticeFor(engineName))
		}
		if !strings.HasPrefix(entries[i], `{matcher="Bash",hooks=[`) {
			t.Errorf("entry %d = %s; want the %s row's entry to open with the literal Bash matcher", i, entries[i], row.Name)
		}
		if !strings.Contains(entries[i], tomlString(guardCommand(row, testLymPath))) {
			t.Errorf("entry %d = %s; want it to run the %s row's own command", i, entries[i], row.Name)
		}
		if !strings.Contains(guardCommand(row, testLymPath), denyDecision(row.SteerFor(engineName))) {
			t.Errorf("%s command = %s; want it to carry the row's Codex deny", row.Name, guardCommand(row, testLymPath))
		}
		notices = append(notices, row.NoticeFor(engineName))
	}
	if got, want := denyNotice(false, rows, every), strings.Join(notices, " "); got != want {
		t.Errorf("denyNotice(false, rows) = %q; want %q", got, want)
	}
	if got, want := denyNotice(true, rows, every), noticeQuestionDeny+" "+strings.Join(notices, " "); got != want {
		t.Errorf("denyNotice(true, rows) = %q; want %q", got, want)
	}
	if got, want := denyNotice(false, rows, guardInputs{denyPublish: true}), strings.Join(noticesWithoutPython, " "); got != want {
		t.Errorf("denyNotice(false, rows) with python allowed = %q; want %q", got, want)
	}
	if got := denyNotice(false, rows, every); strings.Contains(got, "with Read") || !strings.Contains(got, "apply_patch") {
		t.Errorf("denyNotice() = %q; want the Codex edit rule in place of the Claude one", got)
	}
	if got := denyNotice(false, nil, every); got != "" {
		t.Errorf("denyNotice(false, nil) = %q; want empty", got)
	}
	if !strings.Contains(questionDenyCommand(), denyDecision(steerQuestionDeny)) || strings.ContainsAny(steerQuestionDeny, `'"\`) {
		t.Errorf("questionDenyCommand() = %s; want it to carry the question deny, whose steer holds no quote or backslash", questionDenyCommand())
	}
}
