// prreviewask_integration_test.go proves the PR-Review ask reaches the run's parent orch over a real hub.
// The ask is signed as its caller may sign.
// Creel's daemon runs in process on a fake delivery.

package loomcli

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/pullrequest"
	"github.com/Knatte18/loomyard/internal/testkit/creelfake"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestPRReviewAsker_AskReachesTheParentSignedAsTheCaller pins who signs the PR-Review ask the parent orch receives.
// The verb's ask seam gets the session's strand, which signs the ask from the pane where creel refuses a program claim.
// The row passes no claim, and its ask signs as the loom program.
// Either ask is recorded open with the deadline loom.yaml's pr.review_parent_timeout_min of 60 minutes sets.
func TestPRReviewAsker_AskReachesTheParentSignedAsTheCaller(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	session := hubforge.TestShortname + ":ask-task:pr-reviewer"
	tests := []struct {
		name     string
		claim    string
		inPane   bool
		wantFrom string
	}{
		{"TheSessionInItsOwnPaneSignsAsItsStrand", session, true, session},
		{"TheRowWithNoClaimSignsAsTheLoomProgram", "", false, hubforge.TestShortname + ":LOOM"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
			hubforge.AddPair(t, h, "ask-task")
			sessionStrand := creelfake.LiveStrand(session)
			if tt.inPane {
				sessionStrand.PanePID = os.Getpid()
			}
			delivery, receiver := startNoticeOrch(t, h, sessionStrand)
			pair, err := lymcwd.ResolveWorktree(h.PairCodeWorktree("ask-task"))
			if err != nil {
				t.Fatalf("lymcwd.ResolveWorktree(ask-task) = %v; want nil", err)
			}
			rec := pullrequest.Record{URL: "https://github.com/o/r/pull/7", Head: "ask-task", Base: "main"}
			before := time.Now()

			ask, err := parentReviewPRAskFunc(pair)(rec, "head-1", tt.claim)

			after := time.Now()
			if err != nil {
				t.Fatalf("parentReviewPRAskFunc()()() = %v; want nil", err)
			}
			if ask.State != burlerengine.ParentAskOpen || ask.Strand != receiver ||
				ask.Deadline.Before(before.Add(60*time.Minute)) || ask.Deadline.After(after.Add(60*time.Minute)) {
				t.Errorf("ask = %+v; want an open ask to %s with its deadline 60 minutes after the ask", ask, receiver)
			}
			// Out of the session's pane again, so the log reader's program claim is not refused.
			delivery.SetStrands(creelfake.LiveStrand(receiver), creelfake.LiveStrand(session))
			posted := postedEntries(t, h)
			if len(posted) != 1 {
				t.Fatalf("posted mails = %+v; want exactly one", posted)
			}
			if got := posted[0]; got.From != tt.wantFrom || got.To != receiver || !strings.HasPrefix(got.Subject, "[pr-review request] ask-task ask 1:") {
				t.Errorf("posted mail = from %q to %q subject %q; want from %q to %q, the ask's line", got.From, got.To, got.Subject, tt.wantFrom, receiver)
			}
		})
	}
}
