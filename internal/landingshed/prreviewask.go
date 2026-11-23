// prreviewask.go holds the PR-Review ask to the run's parent orch:
// the asker that records and posts it, the line it posts, and the standing the run's ask record gives it at a head.
// The ask lives in burlerengine's never-tracked ask record, beside the review rounds' asks.

package landingshed

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
)

// PRReviewAskLineTag opens every line that asks the parent orch to review the run's pull request.
const PRReviewAskLineTag = "[pr-review request]"

// PRReviewAskSeed is the seed of the PR-Review row's asks in the run's ask record.
const PRReviewAskSeed = "pr-review"

// prReviewDeadlineLayout renders an ask's deadline in the line as a wall-clock time.
const prReviewDeadlineLayout = "2006-01-02 15:04:05 MST"

// PRReviewAsker records the run's ask to the parent orch to review its pull request, and posts it.
type PRReviewAsker struct {
	// Files locates the run's ask record.
	Files burlerengine.ParentReviewFiles
	// Slug, URL, TaskBranch and ParentBranch name the run and its pull request in the ask's line.
	// TaskBranch and ParentBranch are the pull request's two branches.
	Slug         string
	URL          string
	TaskBranch   string
	ParentBranch string
	// Wait is the time from an ask to its deadline: pr_review_parent_timeout_min.
	Wait time.Duration
	// Resolve answers the orch strand an ask goes to and whether that orch's state records one.
	Resolve func() (strand string, recorded bool, err error)
	// Post sends line to strand as creel mail.
	Post func(strand, line string) error
}

// Ask records an ask for the pull request at head as the run's next ask and posts its line, asked at now.
// A parent that resolves gets the ask pending-post with its strand and a deadline Wait after now;
// it is posted and then open.
// A post that fails ends the ask undelivered, with the error as its reason.
// A parent that fails to resolve, or records no orch strand, makes the ask no-parent, and nothing is posted.
// It returns the ask as it then stands.
func (a PRReviewAsker) Ask(head string, now time.Time) (burlerengine.ParentAsk, error) {
	ask := burlerengine.ParentAsk{Seed: PRReviewAskSeed, Tag: PRReviewAskLineTag, Digest: head, AskedAt: now}
	strand, recorded, err := a.Resolve()
	switch {
	case err != nil:
		ask.State, ask.Reason = burlerengine.ParentAskNoParent, fmt.Sprintf("the run's parent orch could not be resolved: %s", err)
	case !recorded:
		ask.State, ask.Reason = burlerengine.ParentAskNoParent, "no orch strand is recorded at the run's parent"
	default:
		ask.State, ask.Strand, ask.Deadline = burlerengine.ParentAskPendingPost, strand, now.Add(a.Wait)
	}
	ask, err = burlerengine.RecordParentAsk(a.Files, ask)
	if err != nil || ask.State != burlerengine.ParentAskPendingPost {
		return ask, err
	}

	if err := a.Post(ask.Strand, PRReviewAskLine(a.Slug, a.URL, a.TaskBranch, a.ParentBranch, ask)); err != nil {
		ended, _, endErr := burlerengine.EndParentAsk(a.Files, ask.Number, burlerengine.ParentAskUndelivered, fmt.Sprintf("the request was not delivered: %s", err))
		return ended, endErr
	}
	if err := burlerengine.MarkParentAskPosted(a.Files, ask.Number); err != nil {
		return burlerengine.ParentAsk{}, err
	}
	ask.State = burlerengine.ParentAskOpen
	return ask, nil
}

// PRReviewAskLine renders the one creel line that asks the parent orch to review the pull request at prURL under ask.
// It names the slug, the ask number, the pull request, its branches and the head the ask was sent at.
// It also names the deadline as a wall-clock time.
// It also names the answer verb with its --ask flag filled in, in both forms.
func PRReviewAskLine(slug, prURL, taskBranch, parentBranch string, ask burlerengine.ParentAsk) string {
	answer := fmt.Sprintf("lym loom parent-review answer %s --ask %d", slug, ask.Number)
	return fmt.Sprintf(`%s %s ask %d: pull request %s, from branch %s into %s, is open for your review at head %s. Answer by %s with "%s --satisfied" to agree, or "%s --findings-file <path>" whose findings each say what to change and where`,
		PRReviewAskLineTag, slug, ask.Number, prURL, taskBranch, parentBranch, ask.Digest, ask.Deadline.Local().Format(prReviewDeadlineLayout), answer, answer)
}

// PRReviewStanding is where the run's PR-Review ask to the parent orch stands at a head.
type PRReviewStanding string

const (
	// PRReviewSatisfied is an ask sent at the head that the parent answered with no findings.
	PRReviewSatisfied PRReviewStanding = "satisfied"
	// PRReviewAwaiting is an ask sent at the head that the parent has not answered, before its deadline.
	PRReviewAwaiting PRReviewStanding = "awaiting"
	// PRReviewAskAgain is any other standing: no PR-Review ask at the head, or one that does not stand.
	PRReviewAskAgain PRReviewStanding = "ask-again"
)

// ReadPRReviewStanding reads the standing at head of the run's latest ask made under PRReviewAskLineTag, at now.
// An ask of another tag is never read as one, a review round's or the drift judge's among them.
// It is satisfied when the ask was sent at head and is done with a report that holds no findings.
// It is awaiting when the ask was sent at head and is pending-post or open before its deadline.
// Otherwise it is ask-again.
// That covers no ask, one sent at another head, one with findings, and one ended timeout, no-parent or undelivered.
// It also covers one cleared, and one open past its deadline.
func ReadPRReviewStanding(files burlerengine.ParentReviewFiles, head string, now time.Time) (PRReviewStanding, error) {
	record, _, err := burlerengine.ReadParentAsks(files)
	if err != nil {
		return "", err
	}
	ask, found := record.LatestTagged(PRReviewAskLineTag)
	if !found || ask.Digest != head || ask.Cleared {
		return PRReviewAskAgain, nil
	}
	switch ask.State {
	case burlerengine.ParentAskDone:
		report, err := burlerengine.ReadParentReport(ask.Report)
		if err != nil {
			return "", err
		}
		if len(report.Findings) == 0 {
			return PRReviewSatisfied, nil
		}
	case burlerengine.ParentAskPendingPost, burlerengine.ParentAskOpen:
		if now.Before(ask.Deadline) {
			return PRReviewAwaiting, nil
		}
	}
	return PRReviewAskAgain, nil
}
