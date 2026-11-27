// serveopen_test.go holds the refusals of `lym board serve` and `lym board open`, none of which starts a process.

package boardcli_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestServeOpenRefusals is not parallel: it sets the strand-name variable of the process.
func TestServeOpenRefusals(t *testing.T) {
	tiergate.Require(t, 0)
	const noHub = "only a run from a worktree of the hub resolves"
	tests := []struct {
		name, strand, want string
	}{
		{"Operator", "", noHub},
		{"Orch", "ly:some-task:orch", noHub},
		{"RunAgent", "ly:some-task:webster", "the board viewer and its server are the operator's tools"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(agentname.StrandNameEnv, tt.strand)
			for _, verb := range []string{"open", "serve"} {
				code, out := runCLI(t, "--board-path", t.TempDir(), verb)
				envelope.RequireErr(t, out, tt.want)
				envelope.RequireErr(t, out, "way forward:")
				if code == 0 {
					t.Errorf("lym board %s exit = 0; want a refusal", verb)
				}
			}
		})
	}
}
