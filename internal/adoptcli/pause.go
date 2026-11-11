// pause.go chooses the slugs an adopt pauses.
// It gives each the pause-record entry that holds it until an operator resumes it.

package adoptcli

import (
	"fmt"
	"slices"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/pauserecord"
)

// pauseTargets returns, sorted, the slugs adopt pauses.
// A moved run is paused when its takeover ran with no stop, error or warning.
// Its reconciled owner must read as another machine, and this call's owner write must make it this machine's.
// A run whose pair reads live here is paused too.
// A released task is paused when this call's owner write reaches its parent.
// That holds for a task not done, not a front task, and with an empty status.
func pauseTargets(deps adoptDeps, here string, reconciled boardengine.Owners, tasks []boardengine.Task, runs, writing []string, rootGoing bool, outcomes map[string]*slugOutcome) ([]string, error) {
	var targets []string
	for _, slug := range runs {
		out := outcomes[slug]
		if out.Stopped || out.Error != "" || out.Warning != "" {
			continue
		}
		owner, err := reconciled.Effective(slug, deps.ParentOf)
		if err != nil {
			return nil, fmt.Errorf("read the owner of %s: %w", slug, err)
		}
		if !boardengine.OwnedElsewhere(owner, here) {
			continue
		}
		reached, err := pauseSubject(deps, reconciled, here, slug, writing, rootGoing)
		if err != nil {
			return nil, err
		}
		if reached {
			targets = append(targets, slug)
		}
	}

	for _, task := range tasks {
		entry := reconciled.Entries[task.Slug]
		if task.Kind != boardengine.KindTask || entry.Done || task.Front || !boardengine.IsEmptyStatus(task.Status) {
			continue
		}
		parent, err := deps.ParentOf(task.Slug)
		if err != nil {
			return nil, fmt.Errorf("read the parent of %s: %w", task.Slug, err)
		}
		if writeReaches(parent, writing, rootGoing) {
			targets = append(targets, task.Slug)
		}
	}

	slices.Sort(targets)
	return slices.Compact(targets), nil
}

// pauseSubject reports whether an owner write of this call makes slug's run this machine's.
// The write covers the beams in writing, and the prime when rootGoing.
// A beam being written reaches its own run, provided its parent reads as this machine's or is written too.
// Any other run is reached through its parent.
func pauseSubject(deps adoptDeps, reconciled boardengine.Owners, here, slug string, writing []string, rootGoing bool) (bool, error) {
	parent, err := deps.ParentOf(slug)
	if err != nil {
		return false, fmt.Errorf("read the parent of %s: %w", slug, err)
	}
	if !slices.Contains(writing, slug) {
		return writeReaches(parent, writing, rootGoing), nil
	}
	if writeReaches(parent, writing, rootGoing) {
		return true, nil
	}
	parentOwner := reconciled.Prime
	if parent != "" {
		if parentOwner, err = reconciled.Effective(parent, deps.ParentOf); err != nil {
			return false, fmt.Errorf("read the owner of %s: %w", parent, err)
		}
	}
	return !boardengine.OwnedElsewhere(parentOwner, here), nil
}

// writeReaches reports whether an owner write gives the chain below parent to this machine.
// The write covers the beams in writing, and the prime when rootGoing.
// The empty parent is the prime.
func writeReaches(parent string, writing []string, rootGoing bool) bool {
	if parent == "" {
		return rootGoing
	}
	return slices.Contains(writing, parent)
}

// pauseEntries gives each slug of slugs a pause entry of its own scope that holds it until an operator resumes it.
// It updates record in place, returns the slugs now paused, and maps the slugs another caller pauses to that caller.
// A slug in listed was paused by an earlier run of the same adopt, so it is skipped and a resumed slug stays resumed.
// A slug holding an owner entry has that entry's caller turned into the adopt caller, keeping its pause time.
// A slug holding an adopt entry keeps it.
// A slug holding an entry of any other caller, the legacy adopt-hold caller included, keeps it and is returned in other.
// Any other slug gets a new adopt entry at now, whose grace ends at once since no session is interrupted by it.
// A beam in beams that slugs does not name gets only the conversion of its own owner entry.
// A beam with no owner entry gets no entry.
func pauseEntries(record *pauserecord.Record, slugs, beams, listed []string, now time.Time) (paused []string, other map[string]string) {
	other = map[string]string{}
	for _, slug := range slugs {
		if slices.Contains(listed, slug) {
			continue
		}
		index := slices.IndexFunc(record.Live, func(entry pauserecord.Entry) bool { return entry.Scope == slug })
		switch {
		case index < 0:
			record.Live = append(record.Live, pauserecord.Entry{Scope: slug, PausedAt: now, GraceEnd: now, Caller: pauserecord.AdoptCaller})
		case record.Live[index].Caller == pauserecord.OwnerCaller:
			record.Live[index].Caller = pauserecord.AdoptCaller
		case record.Live[index].Caller != pauserecord.AdoptCaller:
			other[slug] = record.Live[index].Caller
			continue
		}
		paused = append(paused, slug)
	}

	for _, slug := range beams {
		if slices.Contains(slugs, slug) || slices.Contains(listed, slug) {
			continue
		}
		index := slices.IndexFunc(record.Live, func(entry pauserecord.Entry) bool {
			return entry.Scope == slug && entry.Caller == pauserecord.OwnerCaller
		})
		if index < 0 {
			continue
		}
		record.Live[index].Caller = pauserecord.AdoptCaller
		paused = append(paused, slug)
	}

	slices.Sort(paused)
	return paused, other
}
