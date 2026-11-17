// bar_test.go pins the status bar's format strings against shapes a live tmux of this host cannot catch.

package reedengine

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// sessionLoopTwoFormats returns the offset of the first top-level comma in the body of the #{S:} loop at loopStart.
// It returns -1 when the body holds one format.
// A comma inside a nested #{…} or #[…] group does not count.
func sessionLoopTwoFormats(format string, loopStart int) int {
	closers := []byte{'}'}
	for i := loopStart + len("#{S:"); i < len(format); i++ {
		switch {
		case strings.HasPrefix(format[i:], "#{"):
			closers = append(closers, '}')
			i++
		case strings.HasPrefix(format[i:], "#["):
			closers = append(closers, ']')
			i++
		case format[i] == closers[len(closers)-1]:
			closers = closers[:len(closers)-1]
			if len(closers) == 0 {
				return -1
			}
		case format[i] == ',' && len(closers) == 1:
			return i
		}
	}
	return -1
}

// TestStatusFormatActions_SlackAndAwayTogglesFollowTheFollowButton pins line 3's two toggles.
// They come after the follow button and before the action row.
// Each is a user range with its own label, and each lights green only on its own word of the state job's output.
// The slack toggle is drawn only while the state line holds slack-on or slack-off.
// A line with neither word, such as a bare away-on, draws no slack toggle.
// The away toggle is always drawn.
func TestStatusFormatActions_SlackAndAwayTogglesFollowTheFollowButton(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const stateJob = "#(#{E:@lym_slack_state_command})"
	markers := []string{"#[range=user|follow ", "#[range=user|slack ", "#[range=user|away ", "#{?#{@strand},"}
	at := make([]int, len(markers))
	for i, marker := range markers {
		at[i] = strings.Index(statusFormatActions, marker)
		if at[i] < 0 {
			t.Fatalf("statusFormatActions = %q, want it to hold %q", statusFormatActions, marker)
		}
		if i > 0 && at[i] <= at[i-1] {
			t.Fatalf("%q at %d, %q at %d, want line 3 in the order follow, slack, away, action row", markers[i-1], at[i-1], marker, at[i])
		}
	}

	tests := []struct {
		name      string
		segment   string
		label     string
		litWord   string
		unlitWord string
		shownWhen string
		tail      string
	}{
		{
			"Slack", statusFormatActions[at[1]:at[2]], "slack", "slack-on", "away-on",
			"#{?#{||:#{m:*slack-on*," + stateJob + "},#{m:*slack-off*," + stateJob + "}},", ",}",
		},
		{"Away", statusFormatActions[at[2]:at[3]], "away", "away-on", "slack-on", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if !strings.Contains(statusFormatActions, tt.shownWhen+tt.segment) {
				t.Errorf("%s toggle = %q, want it directly after the condition %q", tt.name, tt.segment, tt.shownWhen)
			}
			wantLit := "#{?#{m:*" + tt.litWord + "*," + stateJob + "},bg=green fg=black bold,"
			if !strings.Contains(tt.segment, wantLit) {
				t.Errorf("%s toggle = %q, want it lit by %q", tt.name, tt.segment, wantLit)
			}
			if strings.Contains(tt.segment, tt.unlitWord) {
				t.Errorf("%s toggle = %q, want it free of the other toggle's word %q", tt.name, tt.segment, tt.unlitWord)
			}
			if wantLabel := "] " + tt.label + " #[norange list=on default] " + tt.tail; !strings.HasSuffix(tt.segment, wantLabel) {
				t.Errorf("%s toggle = %q, want it to end with the label %q", tt.name, tt.segment, wantLabel)
			}
		})
	}
}

// TestStatusFormats_SessionLoopsTakeOneFormat fails on a #{S:} loop with a second, current-item format.
// tmux 3.4 does not accept such a loop.
// Rendering on a newer tmux cannot catch one, so this untagged check scans the bar's format strings.
// It also scans every Go string literal of the package outside bar.go,
// so a session loop declared elsewhere cannot escape it.
func TestStatusFormats_SessionLoopsTakeOneFormat(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	loops := 0
	for name, format := range map[string]string{"statusFormatButtons": statusFormatButtons, "statusFormatSessions": statusFormatSessions, "statusFormatActions": statusFormatActions} {
		for from := 0; ; {
			at := strings.Index(format[from:], "#{S:")
			if at < 0 {
				break
			}
			at += from
			loops++
			if comma := sessionLoopTwoFormats(format, at); comma >= 0 {
				t.Errorf("%s: the #{S:} loop at %d has a second format after the comma at %d", name, at, comma)
			}
			from = at + len("#{S:")
		}
	}
	if loops == 0 {
		t.Error("found no #{S:} loop in the bar's formats, want the scan to see statusFormatSessions' loops")
	}

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || name == "bar.go" {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING && strings.Contains(lit.Value, "#{S:") {
				t.Errorf("%s declares a #{S:} loop in a string literal; session loops belong in bar.go", fset.Position(lit.Pos()))
			}
			return true
		})
	}
}
