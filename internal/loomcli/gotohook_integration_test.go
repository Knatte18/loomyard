// gotohook_integration_test.go drives the PR-Review reset hook the goto spec carries, over a real pair.

package loomcli

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/pullrequest"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestGotoSpec_AdmittedHookClearsThePRReviewRecordOnlyForThatRow pins what the goto spec's hook does.
// It acts on the pair's record.
// A goto onto PR-Review clears the recorded autonomy and ask mark; a goto onto any other row leaves them.
// It runs without t.Parallel: it sets environment variables that skip the record's commit and push.
func TestGotoSpec_AdmittedHookClearsThePRReviewRecordOnlyForThatRow(t *testing.T) {
	tiergate.Require(t, 1)
	t.Setenv("FABRIC_SKIP_GIT", "1")
	t.Setenv("FABRIC_SKIP_PUSH", "1")

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPair(t, h, "goto-task")
	pair, err := lymcwd.ResolveWorktree(h.PairCodeWorktree("goto-task"))
	if err != nil {
		t.Fatalf("lymcwd.ResolveWorktree(goto-task) = %v; want nil", err)
	}
	c := &loomCLI{location: pair, runID: shedrun.SelfRunID}
	hook := c.specFor("goto").GotoAdmitted
	if hook == nil {
		t.Fatal("the goto spec carries no Admitted hook; want the PR-Review reset")
	}
	recordPath, lockPath := shedrun.PullRequestRecord(pair, shedrun.SelfRunID), shedrun.PullRequestRecordLock(pair, shedrun.SelfRunID)
	on := true
	held := pullrequest.Record{Number: 7, Autonomy: &on, ParentReviewAsked: true}

	tests := []struct {
		name   string
		target string
		want   pullrequest.Record
	}{
		{"AnotherRowKeepsTheRecord", loomshed.NameFinalize, held},
		{"PRReviewClearsTheAutonomyAndTheAskMark", loomshed.NamePRReview, pullrequest.Record{Number: 7}},
	}
	for _, tt := range tests {
		if err := pullrequest.WriteRecord(recordPath, lockPath, held); err != nil {
			t.Fatalf("%s: WriteRecord() = %v; want nil", tt.name, err)
		}
		if err := hook(tt.target); err != nil {
			t.Fatalf("%s: hook(%q) = %v; want nil", tt.name, tt.target, err)
		}
		got, found, err := pullrequest.ReadRecord(recordPath, lockPath)
		if err != nil || !found || got.Number != tt.want.Number || (got.Autonomy != nil) != (tt.want.Autonomy != nil) || got.ParentReviewAsked != tt.want.ParentReviewAsked {
			t.Errorf("%s: record after hook(%q) = %+v, %v, %v; want %+v", tt.name, tt.target, got, found, err, tt.want)
		}
	}
}
