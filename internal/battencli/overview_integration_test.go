//go:build integration

// overview_integration_test.go drives "lyx batten overview" through RunCLIIn over a hubforge hub: its refusals, which both end before any pane starts.
// It stays a white-box "package battencli" test, like the other integration files here.

package battencli

import (
	"bytes"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
)

func TestOverviewCmd_RefusesOutsideThePrime(t *testing.T) {
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: ".", Pairs: []string{hubforge.TemplatePairSlug}})
	primeName, err := fabricengine.PrimeName(h.Location)
	if err != nil {
		t.Fatalf("prime name: %v", err)
	}

	tests := []struct {
		name    string
		cwd     string
		wantErr string
	}{
		{"the hub's task pair", h.PairCodeWorktree(hubforge.TemplatePairSlug), primeName},
		{"a directory that is no git worktree", t.TempDir(), lyxcwd.ErrNotAGitRepo.Error()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			if code := RunCLIIn(tt.cwd, &out, []string{"overview"}); code != 1 {
				t.Errorf("overview from %s exited %d; want 1", tt.name, code)
			}
			envelope.RequireErr(t, out.String(), tt.wantErr)
		})
	}
}
