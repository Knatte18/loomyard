// holds.go keeps the runs that an adopt holds until its subject's check.
// It reads and writes the per-subject holds and fix-tries files.
// It gives the pause-record entries that gate a held run and chooses which slugs to hold.

package adoptcli

import (
	"fmt"
	"path/filepath"
	"slices"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/state"
)

const (
	// holdsFileName lists, in a subject's scratch directory, the slugs adopt holds for that subject's check.
	holdsFileName = "holds.json"
	// triesFileName records, in a subject's scratch directory, how many fix tasks the subject's check has filed.
	triesFileName = "fix-tries.json"
)

// holdsFile is the content of holds.json.
type holdsFile struct {
	Slugs []string `json:"slugs"`
}

// triesFile is the content of fix-tries.json.
type triesFile struct {
	Count int `json:"count"`
}

// readHolds returns the slugs the holds file in dir lists, none when the file is absent.
func readHolds(dir string) ([]string, error) {
	path := filepath.Join(dir, holdsFileName)
	file, _, err := state.ReadJSON[holdsFile](path, path+".lock")
	if err != nil {
		return nil, fmt.Errorf("read the holds of %s: %w", dir, err)
	}
	return file.Slugs, nil
}

// appendHolds lists the slugs not yet listed in the holds file in dir, in one locked update.
func appendHolds(dir string, slugs []string) error {
	path := filepath.Join(dir, holdsFileName)
	err := state.UpdateJSON(path, path+".lock", func(current holdsFile, _ bool) (holdsFile, error) {
		for _, slug := range slugs {
			if !slices.Contains(current.Slugs, slug) {
				current.Slugs = append(current.Slugs, slug)
			}
		}
		return current, nil
	})
	if err != nil {
		return fmt.Errorf("record the holds of %s: %w", dir, err)
	}
	return nil
}

// removeHolds drops the named slugs from the holds file in dir in one locked update, leaving every other slug listed.
func removeHolds(dir string, slugs []string) error {
	path := filepath.Join(dir, holdsFileName)
	err := state.UpdateJSON(path, path+".lock", func(current holdsFile, _ bool) (holdsFile, error) {
		current.Slugs = slices.DeleteFunc(slices.Clone(current.Slugs), func(slug string) bool { return slices.Contains(slugs, slug) })
		return current, nil
	})
	if err != nil {
		return fmt.Errorf("drop holds of %s: %w", dir, err)
	}
	return nil
}

// readTries returns the fix-try count the tries file in dir records, 0 when the file is absent.
func readTries(dir string) (int, error) {
	path := filepath.Join(dir, triesFileName)
	file, _, err := state.ReadJSON[triesFile](path, path+".lock")
	if err != nil {
		return 0, fmt.Errorf("read the fix tries of %s: %w", dir, err)
	}
	return file.Count, nil
}

// writeTries records count as the fix-try count in the tries file in dir.
func writeTries(dir string, count int) error {
	path := filepath.Join(dir, triesFileName)
	err := state.UpdateJSON(path, path+".lock", func(current triesFile, _ bool) (triesFile, error) {
		current.Count = count
		return current, nil
	})
	if err != nil {
		return fmt.Errorf("record the fix tries of %s: %w", dir, err)
	}
	return nil
}

// holdEntries gives each slug of slugs a pause entry of its own scope that gates it until its subject's check.
// It updates record in place, returns the slugs now held, and maps the slugs another caller pauses to that caller.
// A slug holding an owner entry has that entry's caller turned into the hold caller, keeping its pause time.
// A slug holding a hold entry keeps it.
// A slug holding an entry of any other caller keeps it and is returned in paused with that caller.
// Any other slug gets a new hold entry at now.
// A new entry waits no grace, since no session is interrupted by it.
func holdEntries(record *pauserecord.Record, slugs []string, now time.Time) (held []string, paused map[string]string) {
	paused = map[string]string{}
	for _, slug := range slugs {
		index := slices.IndexFunc(record.Live, func(entry pauserecord.Entry) bool { return entry.Scope == slug })
		switch {
		case index < 0:
			record.Live = append(record.Live, pauserecord.Entry{Scope: slug, PausedAt: now, GraceEnd: now, Caller: pauserecord.HoldCaller})
		case record.Live[index].Caller == pauserecord.OwnerCaller:
			record.Live[index].Caller = pauserecord.HoldCaller
		case record.Live[index].Caller != pauserecord.HoldCaller:
			paused[slug] = record.Live[index].Caller
			continue
		}
		held = append(held, slug)
	}
	return held, paused
}

// holdTargets returns, per subject, the slugs adopt holds.
// A moved run is held when its takeover ran with no stop, error or warning.
// Its reconciled owner must read as another machine, and this call's owner write must make it this machine's.
// Its project is a beam in writing or root while the prime is written;
// an adopted beam's own run also needs its parent owned here or written.
// A run whose pair is live here is not held and reports running here.
// A released task not done, not a front task, with an empty status, is held under that subject.
// Its parent must be a beam in writing, or the prime while it is written.
func holdTargets(deps adoptDeps, here string, reconciled boardengine.Owners, tasks []boardengine.Task, runs []string, writing []string, rootGoing bool, outcomes map[string]*slugOutcome) (map[string][]string, error) {
	targets := map[string][]string{}
	add := func(subject, slug string) {
		if !slices.Contains(targets[subject], slug) {
			targets[subject] = append(targets[subject], slug)
		}
	}
	// subjectOf returns the subject that this call's owner write gives the parent's chain to.
	// It returns "" when no write reaches the parent's chain.
	subjectOf := func(parent string) string {
		switch {
		case parent == "" && rootGoing:
			return boardengine.RootSubject
		case parent != "" && slices.Contains(writing, parent):
			return parent
		}
		return ""
	}

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
		subject, err := holdSubject(deps, reconciled, here, slug, writing, subjectOf)
		if err != nil {
			return nil, err
		}
		if subject == "" {
			continue
		}
		if deps.LocalPair(slug) {
			live, err := deps.LivePair(slug)
			if err != nil {
				return nil, fmt.Errorf("read whether the pair of %s is live: %w", slug, err)
			}
			if live {
				out.Run = runningHereRun
				continue
			}
		}
		add(subject, slug)
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
		if subject := subjectOf(parent); subject != "" {
			add(subject, task.Slug)
		}
	}

	for subject := range targets {
		slices.Sort(targets[subject])
	}
	return targets, nil
}

// holdSubject returns the subject whose owner write makes slug's run this machine's.
// It returns "" when no write of this call does.
// A beam being written holds its own run under itself, provided its parent reads as this machine's or is written too.
// Any other run is held under the subject its parent names.
func holdSubject(deps adoptDeps, reconciled boardengine.Owners, here, slug string, writing []string, subjectOf func(parent string) string) (string, error) {
	parent, err := deps.ParentOf(slug)
	if err != nil {
		return "", fmt.Errorf("read the parent of %s: %w", slug, err)
	}
	if !slices.Contains(writing, slug) {
		return subjectOf(parent), nil
	}
	if subjectOf(parent) != "" {
		return slug, nil
	}
	parentOwner := reconciled.Prime
	if parent != "" {
		if parentOwner, err = reconciled.Effective(parent, deps.ParentOf); err != nil {
			return "", fmt.Errorf("read the owner of %s: %w", parent, err)
		}
	}
	if boardengine.OwnedElsewhere(parentOwner, here) {
		return "", nil
	}
	return slug, nil
}
