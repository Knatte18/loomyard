// resolve_verbs_test.go covers which resolver each shuttle verb takes in the parent's pre-run, over a real hub.
// It runs from a records-sibling cwd, where await-forks resolves and a strict verb is refused.

package shuttlecli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestRunCLI_AwaitForksResolvesFromTheRecordsSibling runs two verbs from a subdirectory of a pair's records sibling.
// The hub is anchored at a subpath.
// await-forks passes the resolution gate and reaches its own check of the calling strand.
// state takes the strict cwd gate and is refused with the outside-anchor error.
// It is not parallel: it sets the strand-name environment variable.
func TestRunCLI_AwaitForksResolvesFromTheRecordsSibling(t *testing.T) {
	tiergate.Require(t, 1)

	hub := hubforge.NewHub(t, "backend")
	hubforge.AddPair(t, hub, "feat")
	siblingDir := filepath.Join(hub.PairRecordsSibling("feat"), "plan")
	if err := os.MkdirAll(siblingDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", siblingDir, err)
	}
	t.Setenv(agentname.StrandNameEnv, "")

	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"AwaitForksPassesTheResolutionGate", []string{"await-forks"}, "no shuttle strand"},
		{"StateIsRefusedOutsideTheAnchor", []string{"state"}, lymcwd.ErrCwdOutsideAnchor.Error()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			if code := RunCLIIn(siblingDir, &out, tt.args); code != 1 {
				t.Errorf("RunCLIIn(%v) = %d; want 1; output: %s", tt.args, code, out.String())
			}
			envelope := parseSingleEnvelope(t, out.Bytes())
			if got, _ := envelope["error"].(string); envelope["ok"] != false || !strings.Contains(got, tt.wantErr) {
				t.Errorf("envelope = %v; want ok false with an error containing %q", envelope, tt.wantErr)
			}
		})
	}
}
