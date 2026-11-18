// cli_test.go covers the `lym claude` subtree's verbs, their argument counts and the refusal outside a hub.

package claudecli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestCommand pins the subtree's verbs, the argument counts they take and the refusal outside a hub.
// It changes the working directory, which is process-global state, so neither it nor its subtests run in parallel.
func TestCommand(t *testing.T) {
	tiergate.Require(t, 0)
	t.Chdir(t.TempDir())

	for path, use := range map[string]string{
		"account set":  "set <account>",
		"account show": "show",
		"statusline":   "statusline <account>",
	} {
		found, _, err := Command().Find(strings.Fields(path))
		if err != nil || found.Use != use {
			t.Errorf("Find(%q) = %v, %v; want the verb %q", path, found, err, use)
		}
	}

	tests := []struct {
		name string
		args []string
		// wantMessage is the text the refusal holds; empty means only the failure is checked.
		wantMessage string
	}{
		{name: "SetNeedsAnAccount", args: []string{"account", "set"}},
		{name: "SetTakesOneAccount", args: []string{"account", "set", "a", "b"}},
		{name: "ShowTakesNoArguments", args: []string{"account", "show", "a"}},
		{name: "StatuslineNeedsAnAccount", args: []string{"statusline"}},
		{name: "SetRefusesOutsideAHub", args: []string{"account", "set", "a"}, wantMessage: "run the verb from a worktree of a hub"},
		{name: "ShowRefusesOutsideAHub", args: []string{"account", "show"}, wantMessage: "Claude accounts belong to a hub"},
		{name: "StatuslineRefusesOutsideAHub", args: []string{"statusline", "a"}, wantMessage: "run the verb from a worktree of a hub"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			code := clihelp.Execute(Command(), &out, tt.args)
			if code == 0 || !strings.Contains(out.String(), `"ok":false`) {
				t.Errorf("Execute(%v) = %d, %q; want a refusal", tt.args, code, out.String())
			}
			if !strings.Contains(out.String(), tt.wantMessage) {
				t.Errorf("Execute(%v) output = %q; want it to hold %q", tt.args, out.String(), tt.wantMessage)
			}
		})
	}
}
