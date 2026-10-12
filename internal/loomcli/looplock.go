// looplock.go builds the loop's parent lock hooks: the exit hook that releases or hands off the lock a loop owns, and the wake hook that releases it when the run's agents have all waited past the idle bound.

package loomcli

import (
	"sync"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shedverbs"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
)

// runLockIdentity returns the run part of a parent lock owner for the run runID of the worktree loc: the slug, the pair path and the resolved run id.
// The process fields are left zero for the caller to fill.
func runLockIdentity(loc *lyxcwd.Location, runID string) parentlock.Owner {
	return parentlock.Owner{
		Slug:  seedSlug(loc.WorktreeName),
		Pair:  loc.WorktreePath(),
		RunID: shedrun.ResolveRunID(loc, runID),
	}
}

// selfLockOwner returns the seam naming the calling process as owner of the run runID of the worktree loc, read at call time.
// The loop's hooks run in the loop process, so the owner they name is the loop.
func selfLockOwner(loc *lyxcwd.Location, runID string) func() parentlock.Owner {
	return func() parentlock.Owner {
		owner := runLockIdentity(loc, runID)
		process := proc.SelfRecord()
		owner.PID = process.PID
		owner.StartTime = process.StartTime
		return owner
	}
}

// battenWatcher returns the reading of the run's batten-watched marker: the pid of the live batten watching the run, and false when none does.
func battenWatcher(loc *lyxcwd.Location, runID string) func() (int, bool) {
	return func() (int, bool) {
		return liveBattenPID(shedrun.BattenWatchedMarker(loc, runID), proc.IsAlive)
	}
}

// readAgentStates returns the reading of every live agent run of the worktree loc as the parent lock's idle rule folds it.
func readAgentStates(loc *lyxcwd.Location, cfg shuttleengine.Config) func() ([]parentlock.AgentState, error) {
	return func() ([]parentlock.AgentState, error) {
		waits, err := shuttleengine.ReadAgentWaits(cfg, loc.AnchorPath(), claudeengine.NewFromConfig(cfg), time.Now())
		if err != nil {
			return nil, err
		}
		states := make([]parentlock.AgentState, 0, len(waits))
		for _, wait := range waits {
			states = append(states, parentlock.AgentState{Name: wait.StrandName, Busy: wait.Busy, Waiting: wait.Waiting})
		}
		return states, nil
	}
}

// builtOnFirstCall returns a hook that calls build on its first call and runs what build returned from then on.
// The wake hook reads landing's idle bound when it first wakes, so the verbs that never run a loop read no landing config.
func builtOnFirstCall(build func() func()) func() {
	var once sync.Once
	var hook func()
	return func() {
		once.Do(func() { hook = build() })
		hook()
	}
}

// ownedLockBranches returns the branches whose lock record names the process self() names, by run and by process.
// A failed list is logged and reads as owning none.
func ownedLockBranches(locks *parentlock.Store, self parentlock.Owner) []string {
	entries, err := locks.List()
	if err != nil {
		logger.Warn("loomcli: could not list the parent lock records", "cause", err)
		return nil
	}
	var branches []string
	for _, entry := range entries {
		if entry.Err == "" && entry.Record.Owner.SameProcess(self) {
			branches = append(branches, entry.Record.Branch)
		}
	}
	return branches
}

// loopLockExit returns the loop's exit hook: it settles every parent lock record this loop owns, by run and by process, once the stop is final.
// An awaiting stop at PR-Gate with a live batten watching the run hands each record to that batten's process, so the lock is kept while the orch reviews.
// Every other stop releases the records.
// Both are compare-and-swap on the loop's own process, so a record batten or a newer loop owns is left alone, and a store error is logged and changes no stop.
func loopLockExit(locks *parentlock.Store, self func() parentlock.Owner, batten func() (int, bool)) func(stop shedverbs.LoopStop, st shedengine.Status) {
	return func(stop shedverbs.LoopStop, st shedengine.Status) {
		owner := self()
		branches := ownedLockBranches(locks, owner)
		if len(branches) == 0 {
			return
		}
		successor, handOff := parentlock.Owner{}, false
		if st.State == shedengine.StateAwaiting && st.CurrentProducer == loomshed.NamePRGate {
			if pid, watched := batten(); watched {
				successor = owner
				successor.PID = pid
				successor.StartTime, _ = proc.StartTime(pid)
				handOff = true
			}
		}
		for _, branch := range branches {
			var err error
			if handOff {
				_, err = locks.HandOff(branch, owner, successor)
			} else {
				_, err = locks.Release(branch, owner)
			}
			if err != nil {
				logger.Warn("loomcli: could not settle the parent lock at the loop's exit", "branch", branch, "stop", string(stop), "handOff", handOff, "cause", err)
			}
		}
	}
}

// loopIdleRelease returns the loop's wake hook: it releases every parent lock record the loop owns once no agent of the run is busy and one has waited for bound.
// The release frees the lock only; the run keeps running, and its next lock-taking row reacquires.
// The watch starts counting when the loop first owns a record, so a wait before the lock was taken does not count.
// A failed agent read counts as not waiting and releases nothing.
func loopIdleRelease(locks *parentlock.Store, self func() parentlock.Owner, agents func() ([]parentlock.AgentState, error), bound time.Duration) func() {
	created := time.Now()
	return idleReleaseOn(locks, self, agents, bound, func() time.Duration { return time.Since(created) })
}

// idleReleaseOn is loopIdleRelease over an awake clock, which must not advance while the host is suspended.
func idleReleaseOn(locks *parentlock.Store, self func() parentlock.Owner, agents func() ([]parentlock.AgentState, error), bound time.Duration, awake func() time.Duration) func() {
	watch := &parentlock.IdleWatch{Bound: bound}
	return func() {
		owner := self()
		branches := ownedLockBranches(locks, owner)
		if len(branches) == 0 {
			watch = &parentlock.IdleWatch{Bound: bound}
			return
		}
		states, err := agents()
		if err != nil {
			logger.Warn("loomcli: could not read the agents' states for the parent lock's idle rule", "cause", err)
		}
		if !watch.Observe(states, err == nil, awake()) {
			return
		}
		for _, branch := range branches {
			released, err := locks.Release(branch, owner)
			if err != nil {
				logger.Warn("loomcli: could not release the idle parent lock", "branch", branch, "cause", err)
				continue
			}
			if released {
				logger.Warn("loomcli: released the parent lock after every agent waited past the idle bound", "branch", branch, "bound", bound.String())
			}
		}
		watch = &parentlock.IdleWatch{Bound: bound}
	}
}
