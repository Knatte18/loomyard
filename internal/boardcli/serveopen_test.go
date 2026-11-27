// serveopen_test.go pins the refusals of the board viewer's verbs, which come before either starts a process.

package boardcli_test

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestServeOpenRefusals is not parallel: it sets the strand-name variable of the process.
// It runs both verbs under --board-path, since a run with no git hub resolves no worktree.
func TestServeOpenRefusals(t *testing.T) {
	tiergate.Require(t, 0)

	tests := []struct {
		name   string
		strand string
		want   string
	}{
		{"BoardPath", "", "run it from a worktree of the hub"},
		{"RunAgent", "ly:some-task:webster", "the board viewer and its server are the operator's tools"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(agentname.StrandNameEnv, tt.strand)
			for _, verb := range []string{"open", "serve"} {
				code, out := runCLI(t, verb, "--board-path", t.TempDir())
				if code == 0 || !strings.Contains(out, tt.want) {
					t.Errorf("board %s = %d %s; want a refusal naming %q", verb, code, out, tt.want)
				}
			}
		})
	}
}
