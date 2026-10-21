// resumereconcile.go runs the run-scope origin reconcile that "lyx batten run" performs over an existing run record, and turns the slug's verdict into a resume or a refusal.

package battencli

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/pairteardown"
)

// The repository sides a branch outcome names that a stop on is shared by every pair.
const (
	sideBolt    = "bolt"
	sideRecords = "records"
)

// reconcileRun runs the run-scope origin reconcile of slug from the prime, with the seams pairteardown builds for a hub.
// The run scope tears no pair down.
// Each entry of the report's mutation record is logged at Info, since a resume has no envelope to carry it.
// The report is returned with the error, so a partial record is still logged.
func reconcileRun(location *lyxcwd.Location, slug string) (fabricengine.OriginReport, error) {
	teardown, err := pairteardown.New(location)
	if err != nil {
		return fabricengine.OriginReport{}, err
	}
	report, err := fabricengine.ReconcileOrigin(location, fabricengine.ReconcileScope{Run: slug}, teardown.OriginSeams(pairteardown.HubSeamOptions(location)), fabricengine.ReconcileOptions{})
	for _, mutation := range report.Mutations.Entries() {
		logger.Info("battencli: origin reconcile mutation", "slug", slug, "kind", string(mutation.Kind), "target", mutation.Target, "detail", mutation.Detail)
	}
	return report, err
}

// reconcileResume runs the run-scope reconcile for the slug the pre-run is about to resume and returns the refusal its verdict calls for.
// An unreachable origin logs one Warn and resumes, and a reconcile error refuses with fixing its cause, then the re-run, as the way forward.
// A receiver with no reconcile seam resumes unchecked.
func (c *battenCLI) reconcileResume() error {
	if c.reconcile == nil {
		return nil
	}
	report, err := c.reconcile(c.slug)
	if err != nil {
		return fmt.Errorf("battencli: origin reconcile of %q failed: %w; fix the cause it names, then re-run lyx batten run %s", c.slug, err, c.slug)
	}
	if report.Unreachable {
		logger.Warn("battencli: origin was unreachable, resuming without judging the run against it", "slug", c.slug)
		return nil
	}
	return resumeRefusal(c.slug, report)
}

// resumeRefusal turns the verdict on slug's entry into the refusal a resume gets, and returns nil for an active entry or one the report does not judge.
// An ended entry refuses naming the landing or the removal from the board, with "lyx cleanup --apply" from the prime as the way forward.
// A stopped entry refuses naming the branch that stopped it, its state and the way forward for that state, followed by the re-run of the resume.
func resumeRefusal(slug string, report fabricengine.OriginReport) error {
	for _, entry := range report.Entries {
		if entry.Slug != slug {
			continue
		}
		switch entry.State {
		case fabricengine.EntryEnded:
			return fmt.Errorf(
				"battencli: origin ended %q (%s); run \"lyx cleanup --apply\" from the prime, whose hub reconcile tears its pair down, instead of resuming it",
				slug, endedReason(entry.How),
			)
		case fabricengine.EntryStopped:
			return stoppedRefusal(slug, entry, report.Branches)
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

// stoppedRefusal names the branch that stopped entry, its state and the way forward for it, then the resume's own re-entry.
// A branch the entry's own run holds busy stops nothing, as the reconcile itself judges it.
// An entry stopped by a branch every pair shares is named by that branch; one with no branch to name falls back to the entry's own way forward.
func stoppedRefusal(slug string, entry fabricengine.EntryOutcome, branches []fabricengine.BranchOutcome) error {
	reentry := fmt.Sprintf("then re-run lyx batten run %s", slug)
	for _, branch := range branches {
		own := branch.Slug == slug && !(branch.Repo == sideRecords && branch.State == fabricengine.BranchBusy)
		shared := branch.Repo == sideBolt || (branch.Repo == sideRecords && branch.Prime)
		if !own && !(shared && branch.State != fabricengine.BranchBusy) {
			continue
		}
		// Only a stopped branch carries a way forward.
		if branch.WayForward == "" {
			continue
		}
		return fmt.Errorf("battencli: cannot resume %q: branch %s is %s against origin; %s; %s", slug, branch.Branch, branch.State, branch.WayForward, reentry)
	}
	return fmt.Errorf("battencli: cannot resume %q: origin reconcile stopped it; %s; %s", slug, entry.WayForward, reentry)
}
