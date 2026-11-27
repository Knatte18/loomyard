// parent.go is the parent step of the pusher, for a target whose branch runs land on.
// It records a landing the ledger lost, settles each landing origin's parent holds and drops one origin holds otherwise.
// It holds back what another machine owns and pushes the rest.

package runpusher

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/pushledger"
)

var (
	// errParentRefused ends a pass whose parent push is held back; the refusal is already recorded.
	errParentRefused = errors.New("runpusher: the parent push is held back")
	// errParentOwed is wrapped by a parent step that could not finish and is retried, such as over an unreachable origin.
	errParentOwed = errors.New("runpusher: the parent step is owed")
)

// ParentDeps holds the seams of the parent step.
// They are the git, board, ledger and failed-push work on one branch runs land on.
// An owner answers "" when no other machine holds it.
type ParentDeps struct {
	// Branch names the parent branch.
	Branch func() (string, error)
	// Refresh brings the owner records up from origin; an error leaves the pass owed.
	Refresh func() error
	// Fetch fetches the parent branch.
	Fetch func() error
	// OriginHolds reports whether origin's parent branch, as last fetched, holds the commit tip.
	OriginHolds func(tip string) (bool, error)
	// PushTip pushes the parent branch up to tip. Nil runs no parent step in a pass.
	PushTip func(tip string) error
	// ParentOwner names the other machine that owns the parent.
	ParentOwner func() (string, error)
	// RunOwner names the other machine that owns the run, and what `lym adopt` takes to move it here.
	RunOwner func(run string) (owner, subject string, err error)
	// WorkUnderOtherCommit reports whether origin's parent holds the run's work under a commit other than its landing.
	WorkUnderOtherCommit func(run string) (bool, error)
	// LandingCommits maps each run to its landing commit on the parent past origin.
	LandingCommits func() (map[string]string, error)
	// RunDone reports whether the run of a landing commit LandingCommits lists is marked done on the board.
	RunDone func(run string) (bool, error)
	// RunPastFinalize reports whether the run is done in its task worktree, or its pair is gone from this machine.
	RunPastFinalize func(run string) (bool, error)
	// Carries reports whether the first commit holds the second; an empty first commit reads the parent's head.
	Carries func(tip, commit string) (bool, error)
	// AheadByOnly reports whether the parent is ahead of origin by none but tips.
	AheadByOnly func(tips []string) (bool, error)
	// UnpushedRecords returns the run's oldest records commit origin lacks, "" when none or its pair is gone.
	UnpushedRecords func(run string) (string, error)
	// ResetToOrigin moves the parent branch back to origin's tip.
	ResetToOrigin func() error
	// HoldPair keeps records, a records commit of the run, held back for as long as its pair stands.
	HoldPair func(run, records string) error
	// HandOver hands the run over to the machine that owns it.
	HandOver func(run, owner string) error
	// DoneWrite marks the run's board entry done and pushes the board.
	DoneWrite func(run string) error
	// WakePair asks the run's pair target to push now.
	WakePair func(run string) error
	// RecordRefusal records the parent branch's held-back push.
	RecordRefusal func(reason, wayForward string) error
	// ClearRefusal removes the parent branch's refused-push record.
	ClearRefusal func() error
	// RefusalReason returns the reason of the standing held-back record, "" when none stands.
	RefusalReason func() (string, error)
	// PushStep names the command that pushes the parent branch, with its directory.
	PushStep string
}

// refusal gathers what holds a parent push back, one reason and one way forward per cause.
type refusal struct {
	reasons, waysForward []string
}

func (r *refusal) add(reason, wayForward string) {
	if !slices.Contains(r.reasons, reason) {
		r.reasons = append(r.reasons, reason)
		r.waysForward = append(r.waysForward, wayForward)
	}
}

func (r *refusal) standing() bool { return len(r.reasons) > 0 }

func (r *refusal) record(p ParentDeps) error {
	return p.RecordRefusal(strings.Join(r.reasons, reasonSeparator), strings.Join(r.waysForward, "; and "))
}

// reasonSeparator joins the reasons of one held-back record.
const reasonSeparator = "; also "

// unrecordedReasonFormat is the reason of a landing commit no landing records; unrecordedReason reads it back.
const unrecordedReasonFormat = "unrecorded landing commit %s of run %s"

var unrecordedReason = regexp.MustCompile(`^unrecorded landing commit ([0-9a-f]+) of run \S+$`)

func owed(err error) error { return fmt.Errorf("%w: %w", errParentOwed, err) }

// parentEntry reads the parent target's ledger entry.
func parentEntry(deps Deps) (pushledger.Entry, error) {
	entry, _, err := pushledger.Read(deps.HubPath, deps.Key)
	return entry, err
}

// orderedLandings returns the standing landings with each one before every landing whose tip carries its tip.
func orderedLandings(deps Deps) ([]pushledger.Landing, error) {
	entry, err := parentEntry(deps)
	if err != nil {
		return nil, err
	}
	var ordered []pushledger.Landing
	for _, landing := range entry.Landings {
		at := len(ordered)
		for i, other := range ordered {
			newer, err := deps.Parent.Carries(other.Tip, landing.Tip)
			if err != nil {
				return nil, err
			}
			if newer && other.Tip != landing.Tip {
				at = i
				break
			}
		}
		ordered = slices.Insert(ordered, at, landing)
	}
	return ordered, nil
}

func removeLanding(t Target, deps Deps, run string) error {
	return updateEntry(t, deps, func(entry *pushledger.Entry) {
		entry.Landings = slices.DeleteFunc(entry.Landings, func(landing pushledger.Landing) bool { return landing.Run == run })
	})
}

// carriesDropped reports whether the parent's head still holds the commit of a dropped landing.
func carriesDropped(deps Deps) (bool, error) {
	entry, err := parentEntry(deps)
	if err != nil {
		return false, err
	}
	for _, commit := range entry.Dropped {
		carried, err := deps.Parent.Carries("", commit)
		if err != nil || carried {
			return carried, err
		}
	}
	return false, nil
}

// clearUnlessDropped clears the parent's refused-push record unless a dropped landing's commit still stands on it.
func clearUnlessDropped(deps Deps) error {
	carried, err := carriesDropped(deps)
	if err != nil || carried {
		return err
	}
	return deps.Parent.ClearRefusal()
}

// prepareParent brings the owners up from origin and records again every landing the ledger lost:
// a landing commit past origin of a run past its Finalize, neither recorded, dropped nor done.
func prepareParent(t Target, deps Deps) error {
	p := deps.Parent
	if p.Refresh != nil {
		if err := p.Refresh(); err != nil {
			return owed(err)
		}
	}
	commits, err := p.LandingCommits()
	if err != nil {
		return err
	}
	entry, err := parentEntry(deps)
	if err != nil {
		return err
	}
	for _, run := range slices.Sorted(maps.Keys(commits)) {
		recorded := slices.ContainsFunc(entry.Landings, func(landing pushledger.Landing) bool { return landing.Run == run })
		if recorded || entry.Dropped[run] != "" {
			continue
		}
		done, err := p.RunDone(run)
		if err != nil {
			return err
		}
		if done {
			continue
		}
		past, err := p.RunPastFinalize(run)
		if err != nil {
			return err
		}
		if !past {
			continue
		}
		records, err := p.UnpushedRecords(run)
		if err != nil {
			return err
		}
		branch, err := p.Branch()
		if err != nil {
			return err
		}
		landing := pushledger.Landing{Run: run, Tip: commits[run], RecordsCommit: records, ParentBranch: branch, At: deps.Now()}
		if err := RecordLanding(t, landing, deps.Now()); err != nil {
			return err
		}
	}
	return nil
}

// refreshTips re-reads each landing's tip after a rebase of the parent.
// It also forgets each dropped landing whose commit is gone from the parent.
func refreshTips(t Target, deps Deps) error {
	commits, err := deps.Parent.LandingCommits()
	if err != nil {
		return err
	}
	if _, found, err := pushledger.Read(deps.HubPath, deps.Key); err != nil || !found {
		return err
	}
	forgot := false
	err = updateEntry(t, deps, func(entry *pushledger.Entry) {
		for i := range entry.Landings {
			if commit := commits[entry.Landings[i].Run]; commit != "" {
				entry.Landings[i].Tip = commit
			}
		}
		for run := range entry.Dropped {
			if commit := commits[run]; commit != "" {
				entry.Dropped[run] = commit
				continue
			}
			delete(entry.Dropped, run)
			forgot = true
		}
	})
	if err != nil || !forgot {
		return err
	}
	return clearUnlessDropped(deps)
}

// settleOnOrigin settles, oldest first, every landing whose tip origin's parent holds:
// a run of this machine is marked done, and any other is left owing its hand-over with its records held.
func settleOnOrigin(t Target, deps Deps) error {
	p := deps.Parent
	landings, err := orderedLandings(deps)
	if err != nil {
		return err
	}
	var pending []error
	for _, landing := range landings {
		holds, err := p.OriginHolds(landing.Tip)
		if err != nil {
			return err
		}
		if !holds {
			continue
		}
		owner, _, err := p.RunOwner(landing.Run)
		if err != nil {
			pending = append(pending, err)
			continue
		}
		if owner != "" {
			if err := p.HoldPair(landing.Run, landing.RecordsCommit); err != nil {
				return err
			}
			err := updateEntry(t, deps, func(entry *pushledger.Entry) {
				entry.Landings = slices.DeleteFunc(entry.Landings, func(l pushledger.Landing) bool { return l.Run == landing.Run })
				oweHandOver(entry, landing.Run, owner)
			})
			if err != nil {
				return err
			}
			// The landing is settled however it reached origin; a later pass records again any cause that still stands.
			if err := clearUnlessDropped(deps); err != nil {
				return err
			}
			continue
		}
		if err := p.DoneWrite(landing.Run); err != nil {
			pending = append(pending, err)
			continue
		}
		if err := removeLanding(t, deps, landing.Run); err != nil {
			return err
		}
		if err := p.WakePair(landing.Run); err != nil {
			return err
		}
		if err := clearUnlessDropped(deps); err != nil {
			return err
		}
	}
	if len(pending) > 0 {
		return owed(errors.Join(pending...))
	}
	return nil
}

func oweHandOver(entry *pushledger.Entry, run, owner string) {
	if entry.HandOvers == nil {
		entry.HandOvers = map[string]string{}
	}
	entry.HandOvers[run] = owner
}

// dropLandedElsewhere drops, newest first, every landing whose work origin's parent holds under another commit.
// A dropped run owned elsewhere is left owing its hand-over.
func dropLandedElsewhere(t Target, deps Deps) error {
	p := deps.Parent
	landings, err := orderedLandings(deps)
	if err != nil {
		return err
	}
	for _, landing := range slices.Backward(landings) {
		under, err := p.WorkUnderOtherCommit(landing.Run)
		if err != nil {
			return err
		}
		if !under {
			continue
		}
		owner, _, err := p.RunOwner(landing.Run)
		if err != nil {
			return owed(err)
		}
		if err := p.HoldPair(landing.Run, landing.RecordsCommit); err != nil {
			return err
		}
		err = updateEntry(t, deps, func(entry *pushledger.Entry) {
			entry.Landings = slices.DeleteFunc(entry.Landings, func(l pushledger.Landing) bool { return l.Run == landing.Run })
			if entry.Dropped == nil {
				entry.Dropped = map[string]string{}
			}
			entry.Dropped[landing.Run] = landing.Tip
			if owner != "" {
				oweHandOver(entry, landing.Run, owner)
			}
		})
		if err != nil {
			return err
		}
		if err := p.ClearRefusal(); err != nil {
			return err
		}
	}
	return nil
}

// payOwedSteps makes the resets and hand-overs that dropped and settled landings still owe.
// It returns what holds the parent back.
// It reads what is owed from the entry and the parent's head, so a step that failed is made again on every call.
func payOwedSteps(t Target, deps Deps) (held refusal, err error) {
	p := deps.Parent
	entry, err := parentEntry(deps)
	if err != nil {
		return held, err
	}
	for _, run := range slices.Sorted(maps.Keys(entry.Dropped)) {
		commit := entry.Dropped[run]
		carried, err := p.Carries("", commit)
		if err != nil {
			return held, err
		}
		if !carried {
			continue
		}
		alone, err := p.AheadByOnly([]string{commit})
		if err != nil {
			return held, err
		}
		if !alone {
			held.add(droppedRefusal(p, run, commit))
			continue
		}
		if err := p.ResetToOrigin(); err != nil {
			held.add(fmt.Sprintf("the parent was not reset past dropped landing commit %s of run %s: %v", commit, run, err),
				fmt.Sprintf("commit or discard the uncommitted changes in the parent's worktree that the reset past %s would overwrite; "+
					"the next pass or dobby tick resets again", commit))
			continue
		}
		if err := clearUnlessDropped(deps); err != nil {
			return held, err
		}
	}
	for _, run := range slices.Sorted(maps.Keys(entry.HandOvers)) {
		owner, _, err := p.RunOwner(run)
		if err != nil {
			return held, owed(err)
		}
		// A run this machine owns again is handed to no one.
		if owner != "" {
			if err := p.HandOver(run, owner); err != nil {
				held.add(fmt.Sprintf("the run %s was not handed over to machine %q: %v", run, owner, err),
					fmt.Sprintf("fix what the hand-over of %s names; the next pass or dobby tick hands it over again", run))
				continue
			}
		}
		if err := updateEntry(t, deps, func(entry *pushledger.Entry) { delete(entry.HandOvers, run) }); err != nil {
			return held, err
		}
		if err := clearUnlessDropped(deps); err != nil {
			return held, err
		}
	}
	return held, nil
}

func droppedRefusal(p ParentDeps, run, commit string) (reason, wayForward string) {
	return fmt.Sprintf("dropped landing commit %s of run %s stands on the parent among other local commits, and origin holds its work already", commit, run),
		fmt.Sprintf("rebase the parent's worktree to drop %s, then run %s", commit, p.PushStep)
}

// settleLandings fetches the parent and then settles and drops the landings origin already answers.
// It also makes the steps those still owe.
func settleLandings(t Target, deps Deps) (refusal, error) {
	if err := deps.Parent.Fetch(); err != nil {
		return refusal{}, owed(err)
	}
	if err := refreshTips(t, deps); err != nil {
		return refusal{}, err
	}
	if err := settleOnOrigin(t, deps); err != nil {
		return refusal{}, err
	}
	if err := dropLandedElsewhere(t, deps); err != nil {
		return refusal{}, err
	}
	return payOwedSteps(t, deps)
}

// removedFromParent reports whether neither the parent's head nor origin holds the landing commit tip.
// A rebase or reset of the parent that dropped the commit leaves it so.
func removedFromParent(p ParentDeps, tip string) (bool, error) {
	carried, err := p.Carries("", tip)
	if err != nil || carried {
		return false, err
	}
	holds, err := p.OriginHolds(tip)
	return !holds && err == nil, err
}

// blocker is a commit no parent push may carry, with why and the way forward.
type blocker struct {
	commit, reason, wayForward string
}

// unrecordedBlockers returns the landing commits of runs still at their Finalize that no landing records.
func unrecordedBlockers(deps Deps, entry pushledger.Entry) ([]blocker, error) {
	p := deps.Parent
	commits, err := p.LandingCommits()
	if err != nil {
		return nil, err
	}
	var blockers []blocker
	for _, run := range slices.Sorted(maps.Keys(commits)) {
		recorded := slices.ContainsFunc(entry.Landings, func(landing pushledger.Landing) bool { return landing.Run == run })
		if recorded || entry.Dropped[run] != "" {
			continue
		}
		done, err := p.RunDone(run)
		if err != nil {
			return nil, err
		}
		if done {
			continue
		}
		past, err := p.RunPastFinalize(run)
		if err != nil {
			return nil, err
		}
		if past {
			continue
		}
		blockers = append(blockers, blocker{
			commit: commits[run],
			reason: fmt.Sprintf(unrecordedReasonFormat, commits[run], run),
			wayForward: fmt.Sprintf("run `lym loom resume` in the task worktree of %s, which records its landing, "+
				"or rebase the parent's worktree to drop %s, then run %s", run, commits[run], p.PushStep),
		})
	}
	return blockers, nil
}

// pushParent pushes the newest landing tip nothing holds back, and returns what holds the others back.
// A beam parent's one push carries every landing, so another machine's run holds it back whole.
func pushParent(t Target, deps Deps) (held refusal, pushed bool, err error) {
	p := deps.Parent
	entry, err := parentEntry(deps)
	if err != nil {
		return held, false, err
	}
	landings, err := orderedLandings(deps)
	if err != nil {
		return held, false, err
	}
	blockers, err := unrecordedBlockers(deps, entry)
	if err != nil || (len(landings) == 0 && len(blockers) == 0) {
		return held, false, err
	}

	parentOwner, err := p.ParentOwner()
	if err != nil {
		return held, false, owed(err)
	}
	owners := map[string][2]string{}
	for _, landing := range landings {
		owner, subject, err := p.RunOwner(landing.Run)
		if err != nil {
			return held, false, owed(err)
		}
		owners[landing.Run] = [2]string{owner, subject}
	}
	if parentOwner != "" {
		subject := t.Slug
		if t.Kind == KindLanding {
			subject = "root"
		}
		held.add(fmt.Sprintf("the parent is owned by machine %q", parentOwner),
			fmt.Sprintf("run `lym adopt %s` on this machine, then run %s", subject, p.PushStep))
		return held, false, nil
	}
	if len(landings) == 0 {
		for _, b := range blockers {
			held.add(b.reason, b.wayForward)
		}
		return held, false, nil
	}

	elsewhere := map[string]bool{}
	for _, landing := range landings {
		owner, subject := owners[landing.Run][0], owners[landing.Run][1]
		if owner == "" {
			continue
		}
		elsewhere[landing.Tip] = true
		blockers = append(blockers, blocker{
			commit:     landing.Tip,
			reason:     fmt.Sprintf("the landed run %s is owned by machine %q", landing.Run, owner),
			wayForward: fmt.Sprintf("run `lym adopt %s` on this machine, then run %s, or leave the run to its owner", subject, p.PushStep),
		})
	}
	for _, commit := range slices.Sorted(maps.Values(entry.Dropped)) {
		reason, wayForward := droppedRefusal(p, droppedRun(entry, commit), commit)
		blockers = append(blockers, blocker{commit: commit, reason: reason, wayForward: wayForward})
	}

	allowed, wholeHeld := "", false
	for _, landing := range landings {
		removed, err := removedFromParent(p, landing.Tip)
		if err != nil {
			return held, false, err
		}
		// A push by SHA would put a commit the parent no longer holds onto origin.
		if removed {
			held.add(fmt.Sprintf("landing commit %s of run %s was removed from the parent", landing.Tip, landing.Run),
				fmt.Sprintf("run `git merge --no-ff %s` in the parent's worktree, then run %s", landing.Tip, p.PushStep))
			continue
		}
		blocked := false
		for _, b := range blockers {
			carries, err := p.Carries(landing.Tip, b.commit)
			if err != nil {
				return held, false, err
			}
			if !carries {
				continue
			}
			blocked = true
			held.add(b.reason, b.wayForward)
			wholeHeld = wholeHeld || (t.Kind != KindLanding && elsewhere[b.commit])
		}
		if !blocked {
			allowed = landing.Tip
		}
	}
	if allowed == "" || wholeHeld {
		return held, false, nil
	}
	if err := p.PushTip(allowed); err != nil {
		return held, false, err
	}
	return held, true, nil
}

func droppedRun(entry pushledger.Entry, commit string) string {
	for run, dropped := range entry.Dropped {
		if dropped == commit {
			return run
		}
	}
	return ""
}

// hasParentWork reports whether the parent target holds a landing, a dropped one or an owed hand-over,
// or its branch carries a landing commit past origin of a run not done that nothing records.
func hasParentWork(deps Deps) (bool, error) {
	entry, err := parentEntry(deps)
	if err != nil || len(entry.Landings) > 0 || len(entry.Dropped) > 0 || len(entry.HandOvers) > 0 {
		return err == nil, err
	}
	commits, err := deps.Parent.LandingCommits()
	if err != nil {
		return false, err
	}
	for run := range commits {
		done, err := deps.Parent.RunDone(run)
		if err != nil || !done {
			return err == nil, err
		}
	}
	return false, nil
}

// settleParent runs one parent pass of t: it records what the ledger lost, settles and drops, pushes and settles again.
// pushed reports whether the parent branch was pushed.
// A held-back push records its refusal and returns errParentRefused;
// a step to retry returns an error wrapping errParentOwed, or the push's own owed failure.
func settleParent(ctx context.Context, t Target, deps Deps) (pushed bool, err error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := prepareParent(t, deps); err != nil {
		return false, err
	}
	held, err := settleLandings(t, deps)
	if err != nil {
		return false, err
	}
	pushHeld, pushed, err := pushParent(t, deps)
	if err != nil {
		return false, err
	}
	if pushed {
		again, err := settleLandings(t, deps)
		if err != nil {
			return true, err
		}
		// The second settle reads every owed step again, so what the first one held is stale.
		held = again
	}
	for i, reason := range pushHeld.reasons {
		held.add(reason, pushHeld.waysForward[i])
	}
	if !held.standing() {
		return pushed, nil
	}
	if err := held.record(deps.Parent); err != nil {
		return pushed, err
	}
	return pushed, errParentRefused
}

// SettleRefused settles and drops the landings of a refused parent target and pushes nothing,
// so a way forward a hand ran clears the refusal.
// It also clears a refusal over unrecorded landing commits once each is recorded or gone.
// A target with no parent seams is left alone.
func SettleRefused(ctx context.Context, t Target, deps Deps) error {
	if deps.Parent.Fetch == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := prepareParent(t, deps); err != nil {
		return err
	}
	held, err := settleLandings(t, deps)
	if err != nil {
		return err
	}
	if held.standing() {
		return held.record(deps.Parent)
	}
	return clearRecordedGap(deps)
}

// clearRecordedGap clears a held-back record whose every reason is a landing commit since recorded or removed.
func clearRecordedGap(deps Deps) error {
	reason, err := deps.Parent.RefusalReason()
	if err != nil || reason == "" {
		return err
	}
	commits, err := deps.Parent.LandingCommits()
	if err != nil {
		return err
	}
	entry, err := parentEntry(deps)
	if err != nil {
		return err
	}
	standing := slices.Collect(maps.Values(commits))
	for _, clause := range strings.Split(reason, reasonSeparator) {
		match := unrecordedReason.FindStringSubmatch(clause)
		if match == nil {
			return nil
		}
		recorded := slices.ContainsFunc(entry.Landings, func(landing pushledger.Landing) bool { return landing.Tip == match[1] })
		if !recorded && slices.Contains(standing, match[1]) {
			return nil
		}
	}
	return deps.Parent.ClearRefusal()
}

// RecordLanding adds landing to the entry of the parent target with the push-now mark, and starts or wakes its pusher.
// A landing of a run already recorded replaces it.
// A pusher that does not start is logged, since the next tick starts it.
func RecordLanding(parent Target, landing pushledger.Landing, now time.Time) error {
	err := pushledger.Update(parent.Location.HubPath, ledgerKey(parent), now, func(entry pushledger.Entry, _ bool) (pushledger.Entry, bool, error) {
		entry.Landings = slices.DeleteFunc(entry.Landings, func(l pushledger.Landing) bool { return l.Run == landing.Run })
		entry.Landings = append(entry.Landings, landing)
		delete(entry.Dropped, landing.Run)
		delete(entry.HandOvers, landing.Run)
		entry.PushNow = true
		entry.Worktree = parent.Location.WorktreePath()
		entry.Slug = parent.Slug
		return entry, true, nil
	})
	if err != nil {
		return fmt.Errorf("runpusher: record the landing of %s: %w", landing.Run, err)
	}
	RestartPusher(func() (bool, error) { return Start(parent) }, parent.Location.WorktreePath()+" parent")
	return nil
}
