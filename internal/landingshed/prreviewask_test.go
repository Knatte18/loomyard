// prreviewask_test.go — tier 0 tests for the PR-Review ask to the parent orch over a real ask record in a temp directory:
// the asker's records and posts, and the standing the record gives each ask.

package landingshed_test

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// prReviewNow is the clock every ask and standing read of these tests uses.
var prReviewNow = time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)

const prReviewWait = 60 * time.Minute

// TestPRReviewAsker_Ask pins the record the asker writes and the line it posts for each parent it meets.
func TestPRReviewAsker_Ask(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name string
		// earlier records a Code-Review ask first, so the PR-Review ask is the run's second.
		earlier    bool
		unrecorded bool
		resolveErr error
		postErr    error
		wantNumber int
		wantState  burlerengine.ParentAskState
		wantReason string
		wantPosts  int
	}{
		{name: "a resolving parent gets one post and an open ask", wantNumber: 1, wantState: burlerengine.ParentAskOpen, wantPosts: 1},
		{name: "an earlier Code-Review ask numbers the new ask 2", earlier: true, wantNumber: 2, wantState: burlerengine.ParentAskOpen, wantPosts: 1},
		{name: "a parent with no strand records no-parent and posts nothing", unrecorded: true, wantNumber: 1, wantState: burlerengine.ParentAskNoParent, wantReason: "no orch strand"},
		{name: "a resolve error records no-parent and posts nothing", resolveErr: errors.New("boom"), wantNumber: 1, wantState: burlerengine.ParentAskNoParent, wantReason: "could not be resolved: boom"},
		{name: "a failing post records no-parent with its reason", postErr: errors.New("boom"), wantNumber: 1, wantState: burlerengine.ParentAskNoParent, wantReason: "boom", wantPosts: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			files := burlerengine.ParentReviewFilesOf(t.TempDir())
			if tt.earlier {
				earlier := burlerengine.ParentAsk{Seed: "code-review", Tag: "[code-review request]", State: burlerengine.ParentAskTimeout}
				if _, err := burlerengine.RecordParentAsk(files, earlier); err != nil {
					t.Fatalf("RecordParentAsk() error = %v; want nil", err)
				}
			}
			var lines []string
			asker := landingshed.PRReviewAsker{
				Files: files, Slug: "the-slug", URL: "https://github.com/o/r/pull/7", TaskBranch: "task", ParentBranch: "main", Wait: prReviewWait,
				Resolve: func() (string, bool, error) { return "orch-strand", !tt.unrecorded, tt.resolveErr },
				Post: func(strand, line string) error {
					if strand != "orch-strand" {
						t.Errorf("Post strand = %q; want orch-strand", strand)
					}
					lines = append(lines, line)
					return tt.postErr
				},
			}

			ask, err := asker.Ask("head-1", prReviewNow)

			if err != nil {
				t.Fatalf("Ask() error = %v; want nil", err)
			}
			if ask.Number != tt.wantNumber || ask.State != tt.wantState || !strings.Contains(ask.Reason, tt.wantReason) {
				t.Errorf("Ask() = %+v; want ask %d in state %s with a reason naming %q", ask, tt.wantNumber, tt.wantState, tt.wantReason)
			}
			record, _, err := burlerengine.ReadParentAsks(files)
			if err != nil {
				t.Fatalf("ReadParentAsks() error = %v; want nil", err)
			}
			recorded, found := record.LatestTagged(landingshed.PRReviewAskLineTag)
			if !found || recorded.Seed != landingshed.PRReviewAskSeed || recorded.Digest != "head-1" || recorded.State != tt.wantState || !recorded.AskedAt.Equal(prReviewNow) {
				t.Errorf("recorded ask = %+v, found %v; want the tag, seed, head 1 digest, state %s and asked-at time", recorded, found, tt.wantState)
			}
			if len(lines) != tt.wantPosts {
				t.Fatalf("posts = %q; want %d", lines, tt.wantPosts)
			}
			if tt.wantPosts == 0 {
				return
			}
			if wantDeadline := prReviewNow.Add(prReviewWait); recorded.Strand != "orch-strand" || !recorded.Deadline.Equal(wantDeadline) {
				t.Errorf("recorded ask strand %q, deadline %v; want orch-strand and %v", recorded.Strand, recorded.Deadline, wantDeadline)
			}
			number := strconv.Itoa(tt.wantNumber)
			for _, want := range []string{landingshed.PRReviewAskLineTag + " the-slug ask " + number + ":", "head-1", "https://github.com/o/r/pull/7", "lym loom parent-review answer the-slug --ask " + number + " --satisfied"} {
				if !strings.Contains(lines[0], want) {
					t.Errorf("posted line %q lacks %q", lines[0], want)
				}
			}
		})
	}
}

// standingAsk is one ask a TestReadPRReviewStanding row records, oldest first.
type standingAsk struct {
	tag    string
	digest string
	state  burlerengine.ParentAskState
	// deadline is the ask's deadline, which an answered ask must not have reached.
	deadline time.Time
	// answer answers the ask after it is recorded open: "satisfied" or "findings".
	answer  string
	cleared bool
}

// TestReadPRReviewStanding gives each standing a row.
// It also checks that a newer ask of another tag does not hide the PR-Review ask.
func TestReadPRReviewStanding(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	later, earlier := prReviewNow.Add(time.Minute), prReviewNow.Add(-time.Minute)
	tag := landingshed.PRReviewAskLineTag
	tests := []struct {
		name string
		asks []standingAsk
		want landingshed.PRReviewStanding
	}{
		{"NoAskAsksAgain", nil, landingshed.PRReviewAskAgain},
		{"AnOpenAskAtTheHeadAwaits", []standingAsk{{tag: tag, digest: "h", state: burlerengine.ParentAskOpen, deadline: later}}, landingshed.PRReviewAwaiting},
		{"APendingPostAskAtTheHeadAwaits", []standingAsk{{tag: tag, digest: "h", state: burlerengine.ParentAskPendingPost, deadline: later}}, landingshed.PRReviewAwaiting},
		{"AnOpenAskPastItsDeadlineAsksAgain", []standingAsk{{tag: tag, digest: "h", state: burlerengine.ParentAskOpen, deadline: earlier}}, landingshed.PRReviewAskAgain},
		{"ASatisfiedAskAtTheHeadIsSatisfied", []standingAsk{{tag: tag, digest: "h", state: burlerengine.ParentAskOpen, deadline: later, answer: "satisfied"}}, landingshed.PRReviewSatisfied},
		{"ASatisfiedAskAtAnOlderHeadAsksAgain", []standingAsk{{tag: tag, digest: "old", state: burlerengine.ParentAskOpen, deadline: later, answer: "satisfied"}}, landingshed.PRReviewAskAgain},
		{"AnAnswerWithFindingsAsksAgain", []standingAsk{{tag: tag, digest: "h", state: burlerengine.ParentAskOpen, deadline: later, answer: "findings"}}, landingshed.PRReviewAskAgain},
		{"ATimedOutAskAsksAgain", []standingAsk{{tag: tag, digest: "h", state: burlerengine.ParentAskTimeout, deadline: earlier}}, landingshed.PRReviewAskAgain},
		{"ANoParentAskAsksAgain", []standingAsk{{tag: tag, digest: "h", state: burlerengine.ParentAskNoParent}}, landingshed.PRReviewAskAgain},
		{"AClearedAskAsksAgain", []standingAsk{{tag: tag, digest: "h", state: burlerengine.ParentAskTimeout, deadline: earlier, cleared: true}}, landingshed.PRReviewAskAgain},
		{"ACodeReviewAskIsNeverReadAsAPRReviewAsk", []standingAsk{{tag: "[code-review request]", digest: "h", state: burlerengine.ParentAskOpen, deadline: later, answer: "satisfied"}}, landingshed.PRReviewAskAgain},
		{
			name: "ANewerCodeReviewAskAfterASatisfiedAskAtTheHeadLeavesItSatisfied",
			asks: []standingAsk{
				{tag: tag, digest: "h", state: burlerengine.ParentAskOpen, deadline: later, answer: "satisfied"},
				{tag: "[code-review request]", digest: "other", state: burlerengine.ParentAskOpen, deadline: later},
			},
			want: landingshed.PRReviewSatisfied,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			files := burlerengine.ParentReviewFilesOf(t.TempDir())
			for _, spec := range tt.asks {
				recorded, err := burlerengine.RecordParentAsk(files, burlerengine.ParentAsk{Seed: "seed", Tag: spec.tag, Digest: spec.digest, State: spec.state, Deadline: spec.deadline, AskedAt: earlier})
				if err != nil {
					t.Fatalf("RecordParentAsk() error = %v; want nil", err)
				}
				switch spec.answer {
				case "satisfied":
					_, err = burlerengine.AnswerParentAsk(files, recorded.Number, prReviewNow, nil)
				case "findings":
					_, err = burlerengine.AnswerParentAsk(files, recorded.Number, prReviewNow, []burlerengine.ParentFinding{{Change: "rename the helper"}})
				}
				if err == nil && spec.cleared {
					err = burlerengine.ClearParentAsk(files, recorded.Number)
				}
				if err != nil {
					t.Fatalf("answer or clear the recorded ask: %v", err)
				}
			}

			got, err := landingshed.ReadPRReviewStanding(files, "h", prReviewNow)

			if err != nil || got != tt.want {
				t.Errorf("ReadPRReviewStanding() = %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}
