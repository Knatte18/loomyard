// cli_test.go covers the parts of `lym gate test` that spawn nothing: the refusals that precede hub resolution.

package gatecli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestFlagShapeRefusal_RunAgentTmuxTier pins one refusal.
// A run agent's call that lists the tmux tier and names no package target is refused.
// A filter alone and a wildcard target name no package.
// The refusal names the single-test form as its way forward.
// An operator's call, a call naming a package, and a call of another tier pass.
func TestFlagShapeRefusal_RunAgentTmuxTier(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const wantRefusal = "gate test: a run agent's call of tier2, the tmux tier, names no package, so it would start every tmux test of its scope; way forward: run one test with lym gate test --tier 2 <the package that holds the test> -- <a filter selecting one test>"
	tests := []struct {
		name    string
		request testRequest
		want    string
	}{
		{"tier_flag_with_no_target_and_no_filter", testRequest{runAgent: true, tiers: []int{2}}, wantRefusal},
		{"tier_flag_among_others", testRequest{runAgent: true, tiers: []int{0, 2}}, wantRefusal},
		{"upto_reaching_the_tier", testRequest{runAgent: true, upto: 3, uptoGiven: true}, wantRefusal},
		{"all_scope_of_the_tier", testRequest{runAgent: true, tiers: []int{2}, all: true}, wantRefusal},
		{"package_target", testRequest{runAgent: true, tiers: []int{2}, targets: []string{"./internal/reedengine"}}, ""},
		{"wildcard_target", testRequest{runAgent: true, tiers: []int{2}, targets: []string{"./..."}}, wantRefusal},
		{"filter_after_dashes_alone", testRequest{runAgent: true, tiers: []int{2}, args: []string{"-run", "TestAttach"}}, wantRefusal},
		{"package_and_filter", testRequest{runAgent: true, tiers: []int{2}, targets: []string{"./internal/reedengine"}, args: []string{"-run", "TestAttach"}}, ""},
		{"operator_call", testRequest{tiers: []int{2}}, ""},
		{"tier_below_the_tmux_tier", testRequest{runAgent: true, tiers: []int{1}}, ""},
		{"upto_below_the_tmux_tier", testRequest{runAgent: true, upto: 1, uptoGiven: true}, ""},
		{"default_tier", testRequest{runAgent: true}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := flagShapeRefusal(tt.request); got != tt.want {
				t.Errorf("flagShapeRefusal() = %q; want %q", got, tt.want)
			}
		})
	}
}

func TestGateTest_RefusesBeforeResolvingTheHub(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	missingDir := filepath.Join(t.TempDir(), "absent")
	tests := []struct {
		name      string
		args      []string
		wantError string
	}{
		{name: "--tags names --tier", args: []string{"test", "--tags", "integration", "./pkg"}, wantError: "--tier"},
		{name: "-C with --all names explicit targets", args: []string{"test", "-C", "sub", "--all"}, wantError: "explicit targets"},
		{name: "-C with a computed scope names explicit targets", args: []string{"test", "-C", "sub"}, wantError: "explicit targets"},
		{name: "-C with --card names explicit targets", args: []string{"test", "-C", "sub", "--card", "1"}, wantError: "explicit targets"},
		{name: "--all with explicit targets is two scope forms", args: []string{"test", "--all", "./pkg"}, wantError: "one scope form"},
		{name: "--all with --base is two scope forms", args: []string{"test", "--all", "--base", "main"}, wantError: "one scope form"},
		{name: "--gate with --tier names --tier", args: []string{"test", "--gate", "card", "--tier", "1"}, wantError: "drop --tier"},
		{name: "--gate with --upto names --upto", args: []string{"test", "--gate", "card", "--upto", "1"}, wantError: "drop --upto"},
		{name: "--gate with --all names --all", args: []string{"test", "--gate", "card", "--all"}, wantError: "drop --all"},
		{name: "--gate with --base names --base", args: []string{"test", "--gate", "card", "--base", "main"}, wantError: "drop --base"},
		{name: "a negative --upto names no tier", args: []string{"test", "--upto", "-1"}, wantError: "--upto -1 names no tier"},
		{name: "--main-packages without --card names --card", args: []string{"test", "--main-packages"}, wantError: "--card <id>"},
		{name: "-C directory does not exist", args: []string{"test", "-C", missingDir, "./pkg"}, wantError: "does not exist"},
		{name: "a negative --copies names no count", args: []string{"test", "--copies=-1"}, wantError: "--copies -1 names no count"},
		{name: "--copies without --gate names --gate", args: []string{"test", "--copies", "2", "./pkg"}, wantError: "name the gate key with --gate"},
		{name: "--copies without targets names the targets", args: []string{"test", "--copies", "2", "--gate", "live"}, wantError: "name the packages to run as targets"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			code := clihelp.ExecuteIn(Command(), t.TempDir(), &out, tc.args)

			var envelope struct {
				OK    bool   `json:"ok"`
				Error string `json:"error"`
			}
			if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
				t.Fatalf("output %q is not a JSON envelope: %v", out.String(), err)
			}
			if code != 1 || envelope.OK || !strings.Contains(envelope.Error, tc.wantError) || !strings.Contains(envelope.Error, "way forward") {
				t.Errorf("ExecuteIn(%v) = %d, %+v; want exit 1, ok false and an error holding %q and a way forward", tc.args, code, envelope, tc.wantError)
			}
		})
	}
}
