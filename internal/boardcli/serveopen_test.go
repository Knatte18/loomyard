// serveopen_test.go pins the refusals of `board serve` and `board open`, which start no process.

package boardcli_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/envelope"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// Not parallel: it sets the strand name and the board's skip variable in the process environment.
func TestServeOpenRefusals(t *testing.T) {
	tiergate.Require(t, 0)
	t.Setenv("BOARD_SKIP_GIT", "1")
	const runAgent = "ly:some-task:webster"

	for _, verb := range []string{"serve", "open"} {
		tests := []struct {
			name   string
			strand string
			want   string
		}{
			{"BoardPath", "", "lym board " + verb + " needs the hub's board config, which only a run from a worktree of the hub resolves: way forward: run it from a worktree of the hub, without --board-path"},
			{"RunAgent", runAgent, "the run session " + runAgent + " cannot run lym board " + verb + ": the board viewer and its server are the operator's tools, and a server an agent started would carry its session into the operator's page: way forward: ask your orch or the operator to run lym board open"},
		}
		for _, tt := range tests {
			t.Run(verb+"_"+tt.name, func(t *testing.T) {
				t.Setenv("LYM_STRAND_NAME", tt.strand)
				code, out := runCLI(t, verb, "--board-path", t.TempDir())
				if got := envelope.Decode(t, out); code == 0 || got.OK || got.Error != tt.want {
					t.Errorf("%s = %d %q; want the refusal %q", verb, code, got.Error, tt.want)
				}
			})
		}
	}
}
