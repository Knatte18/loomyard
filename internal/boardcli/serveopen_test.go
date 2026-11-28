// serveopen_test.go covers the refusals of the board page's verbs that start nothing.

package boardcli_test

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestServeOpenRefusals is not parallel: it sets the strand name variable.
func TestServeOpenRefusals(t *testing.T) {
	tiergate.Require(t, 0)
	boardPath := t.TempDir()
	tests := []struct {
		name, strand, want string
	}{
		{"NoStrandUnderBoardPath", "", "way forward: run it from a worktree of the hub"},
		// The run-agent refusal comes before the board path's.
		{"RunAgent", "ly:task-a:loom", "the operator's tools"},
	}
	for _, tt := range tests {
		for _, verb := range []string{"serve", "open"} {
			t.Run(tt.name+"_"+verb, func(t *testing.T) {
				t.Setenv(agentname.StrandNameEnv, tt.strand)
				code, out := runCLI(t, "--board-path", boardPath, verb)
				if code == 0 || !strings.Contains(out, "lym board "+verb) || !strings.Contains(out, tt.want) {
					t.Errorf("board %s = %d %s; want a refusal holding %q", verb, code, out, tt.want)
				}
			})
		}
	}
}
