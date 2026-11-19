// serveopen_test.go tests what `lym board serve` and `lym board open` refuse before they start anything.

package boardcli_test

import (
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestServeOpenRefusals runs both verbs with --board-path, which resolves no hub worktree.
// It is not parallel: it sets LYM_STRAND_NAME.
func TestServeOpenRefusals(t *testing.T) {
	tiergate.Require(t, 0)

	tests := []struct {
		name   string
		strand string
		want   []string
		absent string
	}{
		{"NoHubWorktreeResolved", "", []string{"a worktree of the hub", "without --board-path"}, ""},
		{"ARunAgentIsRefusedBeforeTheBoardPathCheck", "ly:my-task:webster", []string{"the operator's tools", "lym board open"}, "needs the hub's board config"},
	}
	for _, verb := range []string{"serve", "open"} {
		for _, tt := range tests {
			t.Run(verb+"_"+tt.name, func(t *testing.T) {
				t.Setenv(agentname.StrandNameEnv, tt.strand)
				boardPath := t.TempDir()

				code, out := runCLI(t, "--board-path", boardPath, verb)
				env := envelope.Decode(t, out)
				if code == 0 || env.OK {
					t.Fatalf("lym board %s = exit %d, ok %v; want a refusal", verb, code, env.OK)
				}
				for _, fragment := range tt.want {
					if !strings.Contains(env.Error, fragment) {
						t.Errorf("lym board %s refused with %q; want it to hold %q", verb, env.Error, fragment)
					}
				}
				if tt.absent != "" && strings.Contains(env.Error, tt.absent) {
					t.Errorf("lym board %s refused with %q; want the run-agent refusal alone", verb, env.Error)
				}
				if entries, err := os.ReadDir(boardPath); err != nil || len(entries) != 0 {
					t.Errorf("board dir holds %v, %v after a refusal; want it untouched", entries, err)
				}
			})
		}
	}
}
