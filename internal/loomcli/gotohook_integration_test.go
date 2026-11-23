// gotohook_integration_test.go drives the PR-Review reset hook the goto spec carries, over a real pair.

package loomcli

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/pullrequest"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestGotoSpec_AdmittedHookClearsThePRReviewRecordOnlyForThatRow pins what the goto spec's hook does.
// It acts on the pair's record, through a receiver wired as the goto verb wires it, with no shuttle runner.
// A goto onto PR-Review clears the recorded autonomy and ask mark; a goto onto any other row leaves them.
// A record that names a strand has the hook build its own runner to stop that strand's session;
// with no reed session up, the runner's stop fails probing reed, and the hook returns that error and keeps the record.
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
	withStrand := pullrequest.Record{Number: 7, Autonomy: &on, ParentReviewAsked: true, StrandGUID: "guid-1", StrandName: "pr-review", SessionID: "session-1", Engine: "claude"}

	tests := []struct {
		name    string
		held    pullrequest.Record
		target  string
		wantErr string
		want    pullrequest.Record
	}{
		{"AnotherRowKeepsTheRecord", held, loomshed.NameFinalize, "", held},
		{"PRReviewClearsTheAutonomyAndTheAskMark", held, loomshed.NamePRReview, "", pullrequest.Record{Number: 7}},
		{"PRReviewWithARecordedStrandOnAnUnwiredReceiverReturnsTheStopError", withStrand, loomshed.NamePRReview, "could not stop the recorded PR-Review session: shuttle: stop strand guid-1: probe reed before removing it", withStrand},
	}
	for _, tt := range tests {
		if err := pullrequest.WriteRecord(recordPath, lockPath, tt.held); err != nil {
			t.Fatalf("%s: WriteRecord() = %v; want nil", tt.name, err)
		}
		err := hook(tt.target)
		if tt.wantErr == "" && err != nil {
			t.Errorf("%s: hook(%q) = %v; want nil", tt.name, tt.target, err)
		}
		if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
			t.Errorf("%s: hook(%q) = %v; want an error containing %q", tt.name, tt.target, err, tt.wantErr)
		}
		got, found, err := pullrequest.ReadRecord(recordPath, lockPath)
		if err != nil || !found || !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: record after hook(%q) = %+v, %v, %v; want %+v", tt.name, tt.target, got, found, err, tt.want)
		}
	}
}
