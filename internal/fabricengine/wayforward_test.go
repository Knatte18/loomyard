// wayforward_test.go covers the way forward of a held-back push and the push step each branch names.

package fabricengine

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestFailedPushWayForward(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name     string
		prime    bool
		wantStep string
	}{
		{"ThePrimesCodeBranchIsPushedByGit", true, "`git push` in /hub/parent"},
		{"ABeamParentIsPushedByThePairPush", false, "`lym fabric push` in "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			record := fabriclayout.FailedPush{
				Repo:       fabriclayout.RepoCode,
				RepoPath:   "/hub/parent",
				Branch:     "parent",
				Prime:      tt.prime,
				Class:      fabriclayout.PushFailureHeldBack,
				WayForward: "run `lym adopt root`, then push",
			}

			if got := FailedPushWayForward(record); got != record.WayForward {
				t.Errorf("FailedPushWayForward() = %q; want the record's own %q", got, record.WayForward)
			}
			if got := pushStep(record); !strings.HasPrefix(got, tt.wantStep) {
				t.Errorf("pushStep() = %q; want it to open with %q", got, tt.wantStep)
			}
		})
	}
}
