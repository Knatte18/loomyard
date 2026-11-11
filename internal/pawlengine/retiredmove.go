// retiredmove.go holds the side effects of moving a pawl run off a row its recipe retired: the pull request carried into the landing record, the standing requests and ready record removed, the hold lifted and the beam's orch told.

package pawlengine

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/logger"
)

// RetiredMoveDeps is every path and seam MoveOffRetired reads, told by the recipe wiring.
type RetiredMoveDeps struct {
	// Slug is the beam's slug, which names its landing record and its notice.
	Slug string
	// Paths holds the run's land request, reopen request and dobby-stop marker.
	Paths Paths
	// LandingPath and LandingLockPath name the beam's landing record and its lock.
	LandingPath     string
	LandingLockPath string
	// ReadyPath and ReadyLockPath name the beam's ready record and its lock.
	ReadyPath     string
	ReadyLockPath string
	// EndSession ends a live PR-Review session of the run.
	EndSession func() error
	// ScratchPullRequest reads the run's scratch pull-request record as it stands.
	ScratchPullRequest func() (number int, url string, found bool, err error)
	// PullRequestOpen reports whether GitHub reports the pull request number open.
	PullRequestOpen func(number int) (bool, error)
	// BeamHead reads the beam worktree's HEAD.
	BeamHead func() (string, error)
	// RemoveScratch removes the run's scratch pull-request record.
	RemoveScratch func() error
	// Commit commits the beam worktree's landing directory.
	Commit func() error
	// SetStatus writes the beam entry's run status naming producer as the row the run is at.
	SetStatus func(producer string) error
	// Notify posts one line to the beam's own orch.
	Notify func(line string) error
}

// MoveOffRetired gives the pawl run of the beam moved from the retired row `from` to the row `to` the side effects an in-flight landing under the old rows needs.
// It ends the PR-Review session.
// It carries the run's scratch pull request into the landing record unless GitHub reports it closed or merged.
// The carried pull request takes the beam's head as its pushed head.
// A pull request whose state does not read is carried too, so a transient failure never drops it.
// A move repeated after the scratch record was removed finds no scratch pull request, and names the one the landing record already holds unless GitHub reports it closed or merged.
// It then removes the scratch record, the ready record, the land request, the dobby-stop marker and the reopen request, commits the landing directory, writes the status naming `to`, which lifts the landing hold, and posts one notice telling the orch how to go on.
// A resumed move, one made by a halted run stepped again, tells the orch to resume a second time, since the resume that made the move ends at the pause.
// Every step is idempotent, so a retried move repeats safely; a failed step is returned naming it, and a failed notice is only logged.
// A nil seam is refused naming the field.
func MoveOffRetired(deps RetiredMoveDeps, from, to string, resumed bool) error {
	if err := deps.requireSeams(); err != nil {
		return err
	}
	if err := deps.EndSession(); err != nil {
		return retiredMoveStep(from, "end the PR-Review session", err)
	}
	carried, err := deps.carryPullRequest()
	if err != nil {
		return retiredMoveStep(from, "carry the pull request into the landing record", err)
	}
	if err := deps.RemoveScratch(); err != nil {
		return retiredMoveStep(from, "remove the scratch pull request record", err)
	}
	if _, err := RemoveReady(deps.ReadyPath, deps.ReadyLockPath, deps.Slug); err != nil {
		return retiredMoveStep(from, "remove the ready record", err)
	}
	if err := RemoveLandRequest(deps.Paths); err != nil {
		return retiredMoveStep(from, "remove the land request", err)
	}
	if err := removeFile(deps.Paths.DobbyStopMarker); err != nil {
		return retiredMoveStep(from, "remove the dobby-stop marker", err)
	}
	if err := removeFile(deps.Paths.ReopenRequest); err != nil {
		return retiredMoveStep(from, "remove the reopen request", err)
	}
	if err := deps.Commit(); err != nil {
		return retiredMoveStep(from, "commit the landing directory", err)
	}
	if err := deps.SetStatus(to); err != nil {
		return retiredMoveStep(from, "write the entry's status", err)
	}
	if err := deps.Notify(retiredMoveNotice(deps.Slug, from, to, resumed, carried)); err != nil {
		logger.Warn("pawlengine: the notice of a move off a retired row was not posted", "slug", deps.Slug, "from", from, "to", to, "cause", err)
	}
	return nil
}

// requireSeams returns an error naming the first seam of deps that is nil.
func (d RetiredMoveDeps) requireSeams() error {
	for _, s := range []seam{
		{"EndSession", d.EndSession != nil},
		{"ScratchPullRequest", d.ScratchPullRequest != nil},
		{"PullRequestOpen", d.PullRequestOpen != nil},
		{"BeamHead", d.BeamHead != nil},
		{"RemoveScratch", d.RemoveScratch != nil},
		{"Commit", d.Commit != nil},
		{"SetStatus", d.SetStatus != nil},
		{"Notify", d.Notify != nil},
	} {
		if !s.set {
			return fmt.Errorf("pawlengine: MoveOffRetired: RetiredMoveDeps.%s is unset", s.field)
		}
	}
	return nil
}

// carryPullRequest writes the scratch pull request into the landing record when GitHub does not report it closed, and returns what it wrote.
// With no scratch record it writes nothing and returns the pull request the landing record already holds, unless GitHub reports that one closed or merged.
// It returns nil when GitHub reports the scratch pull request closed, and when there is no scratch record and the landing record holds no open pull request.
func (d RetiredMoveDeps) carryPullRequest() (*LandingPullRequest, error) {
	number, url, found, err := d.ScratchPullRequest()
	if err != nil {
		return nil, fmt.Errorf("read the scratch record: %w", err)
	}
	if !found {
		return d.recordedPullRequest()
	}
	open, err := d.PullRequestOpen(number)
	if err != nil {
		logger.Warn("pawlengine: GitHub's state of the scratch pull request did not read, so it is carried", "slug", d.Slug, "number", number, "cause", err)
		open = true
	}
	if !open {
		return nil, nil
	}
	head, err := d.BeamHead()
	if err != nil {
		return nil, fmt.Errorf("read the beam head: %w", err)
	}
	carried := LandingPullRequest{Number: number, URL: url, PushedHead: head}
	err = UpdateLanding(d.LandingPath, d.LandingLockPath, d.Slug, func(rec *LandingRecord) error {
		rec.PullRequest = &carried
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &carried, nil
}

// recordedPullRequest returns the pull request the beam's landing record holds, nil when the record is absent, holds none, or holds one GitHub reports closed or merged.
// A state that does not read counts as open, as it does for the scratch pull request.
func (d RetiredMoveDeps) recordedPullRequest() (*LandingPullRequest, error) {
	record, found, err := ReadLanding(d.LandingPath, d.LandingLockPath, d.Slug)
	if err != nil || !found || record.PullRequest == nil {
		return nil, err
	}
	open, err := d.PullRequestOpen(record.PullRequest.Number)
	if err != nil {
		logger.Warn("pawlengine: GitHub's state of the landing record's pull request did not read, so it is named as open", "slug", d.Slug, "number", record.PullRequest.Number, "cause", err)
		open = true
	}
	if !open {
		return nil, nil
	}
	return record.PullRequest, nil
}

// retiredMoveStep words the failure of one step of a move off the retired row from.
func retiredMoveStep(from, step string, err error) error {
	return fmt.Errorf("pawlengine: move off %s: %s: %w", from, step, err)
}

// retiredMoveNotice words the notice of the move of the beam slug from the row `from` to the row `to`.
// It names the pull request when one was carried, the commands that bring the beam's orch and run back, a second resume when resumed, and the verbs that land the beam.
func retiredMoveNotice(slug, from, to string, resumed bool, carried *LandingPullRequest) string {
	line := fmt.Sprintf("the pawl run of the beam %s was moved from %s to %s and paused", slug, from, to)
	if carried != nil {
		line += fmt.Sprintf("; its pull request %s stays open", carried.URL)
	}
	line += fmt.Sprintf("; in the beam's worktree run `lym beam start %s` and `lym loom resume`", slug)
	if resumed {
		line += ", then `lym loom resume` again, since the resume that made this move ends at the pause"
	}
	return line + "; then land the beam with `lym beam describe`, `lym beam pr open|update|status|close` and `lym beam ready`"
}
