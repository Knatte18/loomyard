// resumereconcile.go runs the run-scope origin reconcile that "lym batten run" performs over an existing run record.
// It turns the verdict on the slug into a resume or a refusal.

package battencli

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/pairteardown"
)

// sideBolt and sideRecords name the repository sides a branch outcome reports.
// A stop on the bolt side, or on the records side of the prime, is shared by every pair.
const (
	sideBolt    = "bolt"
	sideRecords = "records"
)

// reconcileRun runs the run-scope origin reconcile of slug from the prime, with the hub seams the pair teardown builds.
// The run scope tears no pair down.
// With judgeOnly the reconcile only judges: it moves no ref and tears nothing down.
// opts bounds the reconcile's remote calls and its retries of an unreachable origin.
// Each entry of the report's mutation record is logged at Info, since a resume has no envelope to carry it.
// The report is returned with the error, so a partial record is still logged.
func reconcileRun(location *lymcwd.Location, slug string, judgeOnly bool, opts fabricengine.ReconcileOptions) (fabricengine.OriginReport, error) {
	teardown, err := pairteardown.New(location)
	if err != nil {
		return fabricengine.OriginReport{}, err
	}
	report, err := fabricengine.ReconcileOrigin(location, fabricengine.ReconcileScope{Run: slug, JudgeOnly: judgeOnly}, teardown.OriginSeams(pairteardown.HubSeamOptions(location)), opts)
	for _, mutation := range report.Mutations.Entries() {
		logger.Info("battencli: origin reconcile mutation", "slug", slug, "kind", string(mutation.Kind), "target", mutation.Target, "detail", mutation.Detail)
	}
	return report, err
}

// reconcileResume runs the run-scope reconcile for the slug the pre-run is about to resume.
// It returns the refusal its verdict calls for.
// A reconcile error refuses; the way forward is to fix the cause it names, then re-run.
// The pass's catch-up has already read origin, so an unreachable origin here means it was lost since.
// That logs one Warn and resumes, unless the local board shows a task owned by another machine; it writes no owner.
// A reconcile that reached origin claims the prime when no prime owner is recorded;
// it warns when the claim fails.
// It then judges the verdict, and writes no owner.
// A receiver with no reconcile seam resumes unchecked.
func (c *battenCLI) reconcileResume() error {
	if c.reconcile == nil {
		return nil
	}
	report, err := c.reconcile(c.slug)
	if err != nil {
		return fmt.Errorf("battencli: origin reconcile of %q failed: %w; fix the cause it names, then re-run lym batten run %s", c.slug, err, c.slug)
	}
	if report.Unreachable {
		logger.Warn("battencli: origin was unreachable, resuming without judging the run against it", "slug", c.slug)
		return c.refuseOwnedElsewhereOnLocalBoard(fmt.Sprintf("re-run `lym batten run %s` once origin answers, or ", c.slug))
	}
	if c.owner.Machine != nil {
		if err := c.claimPrimeOwner(); err != nil {
			logger.Warn("battencli: the prime owner was not claimed, resuming without it", "slug", c.slug, "cause", err)
		}
	}
	return resumeRefusal(c.slug, report)
}

// resumeRefusal turns the verdict on slug's entry into the refusal a resume gets.
// It returns nil for an active entry, or for one the report does not judge.
// An ended entry refuses, naming the landing or the removal from the board;
// "lym cleanup --apply" from the prime is the way forward.
// An entry owned elsewhere refuses, naming its owner and the adopt of its subject that takes it over;
// the subject is the beam or `root`.
// A stopped entry refuses, naming the branch that stopped it and its state;
// the way forward for that state comes next, then the re-run of the resume.
func resumeRefusal(slug string, report fabricengine.OriginReport) error {
	for _, entry := range report.Entries {
		if entry.Slug != slug {
			continue
		}
		switch entry.State {
		case fabricengine.EntryEnded:
			return fmt.Errorf(
				"battencli: origin ended %q (%s); run \"lym cleanup --apply\" from the prime, whose hub reconcile tears its pair down, instead of resuming it",
				slug, endedReason(entry.How),
			)
		case fabricengine.EntryOwnedElsewhere:
			return ownedElsewhereFor(slug, entry.Owner, entry.AdoptSubject)
		case fabricengine.EntryStopped:
			return stoppedRefusal(slug, "resume", resumeReentry(slug), entry, report.Branches)
		}
	}
	return nil
}

// endedReason words how origin ended an entry: its board entry is done once it landed, and removed otherwise.
func endedReason(how string) string {
	if how == "done" {
		return "it landed and its board entry is done"
	}
	return "its board entry was removed from the board"
}

// stoppedRefusal names the branch that stopped entry, its state, and the way forward for it;
// reentry, the step that retries the act, follows.
// act is the verb the refusal says it cannot do, "resume" or "cancel".
// A branch the entry's own run holds busy stops nothing, as the reconcile itself judges it.
// An entry stopped by a branch every pair shares is named by that branch;
// one with no branch to name falls back to the entry's own way forward.
func stoppedRefusal(slug, act, reentry string, entry fabricengine.EntryOutcome, branches []fabricengine.BranchOutcome) error {
	for _, branch := range branches {
		own := branch.Slug == slug && !(branch.Repo == sideRecords && branch.State == fabricengine.BranchBusy)
		shared := branch.Repo == sideBolt || (branch.Repo == sideRecords && branch.Prime)
		if !own && !(shared && branch.State != fabricengine.BranchBusy) {
			continue
		}
		// Only a stopped branch carries a way forward, so a branch without one is skipped.
		if branch.WayForward == "" {
			continue
		}
		return fmt.Errorf("battencli: cannot %s %q: branch %s is %s against origin; %s; %s", act, slug, branch.Branch, branch.State, branch.WayForward, reentry)
	}
	return fmt.Errorf("battencli: cannot %s %q: origin reconcile stopped it; %s; %s", act, slug, entry.WayForward, reentry)
}

// fabricRemoveCommand is the removal a way forward names for a branch that only the removal clears.
const fabricRemoveCommand = "lym fabric remove"

// resumeReentry is the step that ends a resume refusal: the run, run again.
func resumeReentry(slug string) string {
	return fmt.Sprintf("then re-run lym batten run %s", slug)
}

// rewordedError is an error whose text a follow-up replaced, which still matches whatever the original error matches.
type rewordedError struct {
	error
	text string
}

// Error returns the reworded text.
func (e rewordedError) Error() string { return e.text }

// Unwrap returns the error whose text was replaced.
func (e rewordedError) Unwrap() error { return e.error }

// withFollowUp ends refusal's text in followUp.
// It replaces the trailing "; <reentry>" when the text ends in it;
// otherwise it appends followUp after "; ".
func withFollowUp(refusal error, reentry, followUp string) error {
	text := refusal.Error()
	if head, ok := strings.CutSuffix(text, "; "+reentry); ok {
		text = head
	}
	return rewordedError{error: refusal, text: text + "; " + followUp}
}
