// prepush.go judges a loom run against origin and its owners right before the run commits or pushes its work.
// It stops a run whose owner or branches moved since the run last looked.

package loomcli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/tendercli"
)

// errPushStopped marks a push the pre-push check stopped: the run is paused and the orch was noticed.
var errPushStopped = errors.New("loomcli: the push was stopped before it ran")

// prePushCheck is the check a loom run's commits and pushes pass first, its seams injected so a test drives it from stubs.
type prePushCheck struct {
	// slug names the run in its notices.
	slug string
	// skip reports whether the push needs no check: pushes are off, the pair is the prime, or a pause covers the run.
	skip func() (bool, error)
	// reconcile runs the run-scope origin reconcile and returns the report narrowed to the run's own entry.
	reconcile func() (fabricengine.OriginReport, error)
	// parentBranch names the branch the run lands on, which the run-scope reconcile judges beside the run's own branches.
	parentBranch func() (string, error)
	// parentNoticed records key as posted and reports whether it already was, so each origin tip is announced once.
	parentNoticed func(key string) bool
	// landed reports whether the run's parent already holds the run's work.
	landed func() (bool, error)
	// pauseOwned pauses the run under the owner caller.
	pauseOwned func() error
	// pause pauses the run with a plain pair entry.
	pause func() error
	// notify posts one line to the nearest live orch.
	notify func(line string) error
}

// newPrePushCheck builds the pre-push check of the run runID in the pair at location over the hub's own seams.
// It performs no I/O: every seam reads at call time.
func newPrePushCheck(location *lymcwd.Location, runID string) prePushCheck {
	slug := seedSlug(location.WorktreeName)
	return prePushCheck{
		slug: slug,
		skip: func() (bool, error) {
			return skipsCheck(
				fabricengine.EnvSyncOptions(),
				func() (bool, error) { return fabricengine.IsPrimeWorktree(location) },
				func() bool { return pauseCovered(location)(slug) },
				func() (string, error) {
					status, _, err := state.ReadJSON[shedengine.Status](shedrun.StatusFile(location, runID), shedrun.StatusLock(location, runID))
					return status.CurrentProducer, err
				},
			)
		},
		reconcile:    runReconciler(location, slug),
		parentBranch: func() (string, error) { return readRecordedParentBranch(location) },
		parentNoticed: func(key string) bool {
			return announcedBefore(shedrun.ParentNoticeFile(location, runID), key)
		},
		landed: func() (bool, error) {
			prime, err := primeOfHub(location)
			if err != nil {
				return false, err
			}
			return fabricengine.ParentHoldsTaskWork(prime, location, slug)
		},
		pauseOwned: func() error { return tendercli.PauseOwnedElsewhere(location, slug) },
		pause:      func() error { return tendercli.PausePair(location, slug) },
		notify: func(line string) error {
			_, err := orchcli.NotifyParent(location, loomProgram, line)
			return err
		},
	}
}

// skipsCheck reports whether a run's commit or push needs no pre-push check.
// It needs none when pushes or git are off, the pair is the prime, or a pause covers the run.
// It also needs none when the run stands at Finalize, which judges origin and its owners itself.
// Each read runs only when the ones before it left the answer open, and a failed read is returned with false.
// isPrime reads whether the pair is the prime, pauseCovers whether a pause covers the run;
// currentProducer returns the producer the run's status file names, empty when it has no status.
func skipsCheck(opts fabricengine.SyncOptions, isPrime func() (bool, error), pauseCovers func() bool, currentProducer func() (string, error)) (bool, error) {
	if opts.SkipGit || opts.SkipPush {
		return true, nil
	}
	prime, err := isPrime()
	if err != nil || prime {
		return prime, err
	}
	if pauseCovers() {
		return true, nil
	}
	producer, err := currentProducer()
	if err != nil {
		return false, err
	}
	return producer == loomshed.NameFinalize, nil
}

// guard runs the check and returns nil when the work may go.
// It returns an error wrapping errPushStopped, naming why, when it stops the work.
// The work goes when the check is skipped or cannot tell whether it applies;
// when origin is unreachable or the origin reconcile fails, so the push's own failure is recorded;
// when a read of whether the parent holds the run's work fails; and when the run's entry reads active.
// An active entry whose own branches the reconcile moved posts one notice naming them.
// One whose parent branch origin moved past this machine's tip posts another naming it, once per origin tip.
// An entry another machine owns pauses the run under the owner;
// it names `lym adopt` with the verdict's subject, or the slug when it brought none.
// An entry origin ended pauses the run unless its parent already holds the run's work, as after its own landing.
// An entry a branch stops pauses the run and names the stopped branches with their state;
// they are the run's own branches and its parent branch, or a branch every run shares when none did.
// The notice then gives the way forward and `lym tender resume`.
// Every stop posts one notice, which names a failed pause.
func (c prePushCheck) guard() error {
	skip, err := c.skip()
	if err != nil {
		logger.Warn("loomcli: the pre-push check could not tell whether it applies; pushing unchecked", "slug", c.slug, "error", err)
		return nil
	}
	if skip {
		return nil
	}
	report, err := c.reconcile()
	if err != nil {
		logger.Warn("loomcli: the pre-push origin reconcile failed; pushing unchecked", "slug", c.slug, "error", err)
		return nil
	}
	if report.Unreachable {
		logger.Warn("loomcli: origin is unreachable before a push; pushing unchecked, so the push's own failure is recorded", "slug", c.slug)
		return nil
	}

	reason, owned := c.stopReason(report)
	if reason == "" {
		if moved := c.movedBranches(report); len(moved) > 0 {
			c.post(fmt.Sprintf("%s: origin moved %s; the run took the new tips before its push", c.slug, strings.Join(moved, ", ")))
		}
		parent, found := c.parentOutcome(report)
		if found && (parent.State == fabricengine.BranchFastForward || parent.State == fabricengine.BranchDiverged) &&
			!c.parentNoticed(fmt.Sprintf("%s %s %s", parent.Branch, parent.State, parent.OriginHead)) {
			c.post(fmt.Sprintf("%s: origin moved the parent branch %s (%s) past this machine's tip", c.slug, parent.Branch, parent.State))
		}
		return nil
	}

	pause := c.pause
	if owned {
		pause = c.pauseOwned
	}
	line := fmt.Sprintf("%s: paused before its push: %s", c.slug, reason)
	if err := pause(); err != nil {
		line += fmt.Sprintf("; the pause failed (%v), so the run's next push stops again", err)
	}
	c.post(line)
	return fmt.Errorf("%w: %s", errPushStopped, reason)
}

// guardedCommit returns commit behind the check.
// A commit the check stops runs nothing and returns the check's error, which wraps errPushStopped,
// so the records origin has moved stay as they are.
// A caller that pushes after its commit reads that error as the stop of the push too.
func (c prePushCheck) guardedCommit(commit func(msg string) (bool, error)) func(msg string) (bool, error) {
	return func(msg string) (bool, error) {
		if err := c.guard(); err != nil {
			return false, err
		}
		return commit(msg)
	}
}

// guardedSpawn returns spawn behind the check.
// A push the check stops is not started; it returns nil and leaves its commits local.
func (c prePushCheck) guardedSpawn(spawn func() error) func() error {
	return func() error {
		if err := c.guard(); err != nil {
			logger.Info("loomcli: the status push was stopped; its commits stay local", "error", err)
			return nil
		}
		return spawn()
	}
}

// guardedPush returns push behind the check.
// A push the check stops runs nothing and returns the check's error, which wraps errPushStopped.
func (c prePushCheck) guardedPush(push func() error) func() error {
	return func() error {
		if err := c.guard(); err != nil {
			return err
		}
		return push()
	}
}

// stopReason returns why the run's entry stops its push, empty when it does not;
// owned reports whether the stop is another machine's ownership.
// A failed read of whether the parent holds the run's work stops nothing;
// the push goes and its own failure is recorded.
func (c prePushCheck) stopReason(report fabricengine.OriginReport) (reason string, owned bool) {
	for _, entry := range report.Entries {
		if entry.Slug != c.slug {
			continue
		}
		switch entry.State {
		case fabricengine.EntryOwnedElsewhere:
			subject := entry.AdoptSubject
			if subject == "" {
				subject = c.slug
			}
			return fmt.Sprintf("the run is owned by machine %q; take it over with `lym adopt %s` from this machine's prime", entry.Owner, subject), true
		case fabricengine.EntryEnded:
			landed, err := c.landed()
			if err != nil {
				logger.Warn("loomcli: could not tell whether the parent holds the run's work; pushing unchecked, so the push's own failure is recorded", "slug", c.slug, "error", err)
				return "", false
			}
			if landed {
				return "", false
			}
			return fmt.Sprintf("origin shows the run %s; `lym cleanup --apply` from the prime removes its pair", entry.How), false
		case fabricengine.EntryStopped:
			what := "a branch every run shares"
			if stopped := c.stoppedBranches(report); len(stopped) > 0 {
				what = strings.Join(stopped, ", ")
			}
			return fmt.Sprintf("origin moved %s: %s; then `lym tender resume %s`", what, entry.WayForward, c.slug), false
		}
	}
	return "", false
}

// stoppedBranches returns each of the run's own branches whose state stops its entry, as the branch name with its state.
// It also returns the parent branch when origin's move of it stops the entry.
// A parent branch held back by this machine's own live pair, dirty path or lock is not named.
// Origin moved nothing there.
// The run's own busy records branch is not named either: its driver holds it, and the reconcile reads that as no stop.
func (c prePushCheck) stoppedBranches(report fabricengine.OriginReport) []string {
	var stopped []string
	for _, branch := range c.ownBranches(report) {
		if branch.Repo == fabricengine.RepoRecords && branch.State == fabricengine.BranchBusy {
			continue
		}
		if branch.State.Stops() {
			stopped = append(stopped, fmt.Sprintf("%s (%s)", branch.Branch, branch.State))
		}
	}
	if parent, found := c.parentOutcome(report); found && stoppedByOrigin(parent.State) {
		stopped = append(stopped, fmt.Sprintf("%s (%s)", parent.Branch, parent.State))
	}
	return stopped
}

// stoppedByOrigin reports whether a branch in state stops its entry because of what origin did to the branch.
func stoppedByOrigin(state fabricengine.BranchState) bool {
	switch state {
	case fabricengine.BranchRewound, fabricengine.BranchRewritten, fabricengine.BranchDiverged, fabricengine.BranchGone:
		return true
	}
	return false
}

// announcedBefore reports whether the file at path already holds key, and records key there when it does not.
// A file that cannot be read or written reads as not announced, so a notice is repeated rather than lost.
func announcedBefore(path, key string) bool {
	held, err := os.ReadFile(path)
	if err == nil && string(held) == key {
		return true
	}
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		logger.Warn("loomcli: the pre-push notice record is unreadable, so its notice is posted again", "path", path, "error", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		logger.Warn("loomcli: the pre-push notice record cannot be written, so its notice may repeat", "path", path, "error", err)
		return false
	}
	if err := os.WriteFile(path, []byte(key), 0o644); err != nil {
		logger.Warn("loomcli: the pre-push notice record cannot be written, so its notice may repeat", "path", path, "error", err)
	}
	return false
}

// movedBranches returns the name of each of the run's own branches the reconcile moved.
func (c prePushCheck) movedBranches(report fabricengine.OriginReport) []string {
	var names []string
	for _, branch := range c.ownBranches(report) {
		if branch.Moved {
			names = append(names, branch.Branch)
		}
	}
	return names
}

// ownBranches returns the verdicts on the run's own branches.
func (c prePushCheck) ownBranches(report fabricengine.OriginReport) []fabricengine.BranchOutcome {
	var own []fabricengine.BranchOutcome
	for _, branch := range report.Branches {
		if branch.Slug == c.slug {
			own = append(own, branch)
		}
	}
	return own
}

// parentOutcome returns the reconcile's verdict on the run's parent branch.
// found is false when the reconcile judged none, or when the parent branch cannot be read.
// Only a branch of the code repository counts, since the hub's board branch may carry the same name.
func (c prePushCheck) parentOutcome(report fabricengine.OriginReport) (fabricengine.BranchOutcome, bool) {
	name, err := c.parentBranch()
	if err != nil {
		logger.Warn("loomcli: the run's parent branch is unreadable, so the pre-push check judges none", "slug", c.slug, "error", err)
		return fabricengine.BranchOutcome{}, false
	}
	for _, branch := range report.Branches {
		if branch.Repo == fabricengine.RepoCode && branch.Branch == name {
			return branch, true
		}
	}
	return fabricengine.BranchOutcome{}, false
}

// post sends line to the orch, and a failed post only warns.
func (c prePushCheck) post(line string) {
	if err := c.notify(line); err != nil {
		logger.Warn("loomcli: the pre-push notice was not posted", "slug", c.slug, "line", line, "error", err)
	}
}
