// guards_test.go pins the shell guard rows a Codex launch installs, the hook entries it renders for them and the deny notice announcing them.

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

// TestInstalledGuards_CoverEveryRow pins that a launch with every switch on installs every row of the table in table order, the refuse-raw row of a non-empty list included,
// and that the python and publish rows follow their switches.
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
		{"SwitchesOff", sampleRefuseRaw, guardInputs{}, slices.DeleteFunc(slices.Clone(every), func(name string) bool { return name == "python" || name == "publish" })},
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
// It pins one entry per row, each opening with the literal Bash matcher and carrying the row's Codex steer in its deny.
// It also pins a deny notice of the rows' Codex notices in order.
func TestGuardEntries_RenderEveryRowWithItsCodexText(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	if got, want := denyDecision("x"), `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"x"}}`; got != want {
		t.Errorf("denyDecision(x) = %s; want %s", got, want)
	}
	rows := installedGuards(sampleRefuseRaw, guardInputs{denyPython: true, denyPublish: true})
	entries := guardEntries(rows)
	if len(entries) != len(rows) {
		t.Fatalf("guardEntries() = %d entries; want one per row, %d", len(entries), len(rows))
	}
	var notices []string
	for i, row := range rows {
		if !strings.HasPrefix(entries[i], `{matcher="Bash",hooks=[`) {
			t.Errorf("entry %d = %s; want the %s row's entry to open with the literal Bash matcher", i, entries[i], row.Name)
		}
		if !strings.Contains(entries[i], tomlString(guardCommand(row))) {
			t.Errorf("entry %d = %s; want it to run the %s row's own command", i, entries[i], row.Name)
		}
		if !strings.Contains(guardCommand(row), denyDecision(row.SteerFor(engineName))) {
			t.Errorf("%s command = %s; want it to carry the row's Codex deny", row.Name, guardCommand(row))
		}
		notices = append(notices, row.NoticeFor(engineName))
	}
	if got, want := denyNotice(rows), strings.Join(notices, " "); got != want {
		t.Errorf("denyNotice() = %q; want %q", got, want)
	}
	if got := denyNotice(rows); strings.Contains(got, "with Read") || !strings.Contains(got, "apply_patch") {
		t.Errorf("denyNotice() = %q; want the Codex edit rule in place of the Claude one", got)
	}
	if got := denyNotice(nil); got != "" {
		t.Errorf("denyNotice(nil) = %q; want empty", got)
	}
}
