// bolthold.go settles one dobby's runs against the adopt hold a Bolt records.
// It works through the pause record's bolt-hold entries.

package tenderengine

import (
	"errors"
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/pauserecord"
)

// BoltHoldState is what the local Bolt records about the hub's adopt hold.
type BoltHoldState struct {
	// Standing is true while a hold stands.
	Standing bool
	// Machine is the machine that wrote the hold.
	Machine string
	// FixTask is the slug of the fix task an adopt-fix hold stands over, empty when the hold covers every slug.
	FixTask string
}

// BoltHoldRequest names what one dobby settles against the hold.
type BoltHoldRequest struct {
	// Machine is the hold's machine, recorded on each bolt-hold entry written.
	Machine string
	// Standing is true while the hold stands; once false, every bolt-hold entry of Scope is lifted.
	Standing bool
	// Hold lists the slugs the hold holds: those the act rule answers wait with reason held for.
	Hold []string
	// Scope lists every slug the caller drives, the only slugs whose entries are written or lifted.
	Scope []string
	// Grace is the grace a written entry's pause waits for busy sessions and network operations.
	Grace time.Duration
	// HoldsParentLock, when set, reports whether a slug's run holds its parent lock.
	// A slug that does gets no entry and has its bolt-hold entry lifted,
	// so the gate does not hold its step in flight while the act rule still starts no producer for it.
	// A nil seam reads no slug as holding its lock.
	HoldsParentLock func(slug string) (bool, error)
}

// BoltHoldReport lists what SyncBoltHold changed.
type BoltHoldReport struct {
	// Written holds the slugs given a bolt-hold entry.
	Written []string `json:"written"`
	// Dropped holds the slugs whose bolt-hold entry was lifted.
	Dropped []string `json:"dropped"`
}

// SyncBoltHold brings the bolt-hold entries of req.Scope in line with the hold.
// While the hold stands, each slug of req.Hold with no live entry of its own scope gets a bolt-hold entry through Pause,
// so its sessions and network calls are held.
// Each bolt-hold entry of a Scope slug that left Hold, or holds its parent lock, is lifted;
// so is every one once the hold stands no longer.
// A bolt-hold entry that records another machine than req.Machine is replaced by one that records req.Machine.
// Only bolt-hold entries are written or lifted, so an operator's hub entry or slug entry stands.
// A failed read or write for one slug is joined into the returned error and leaves that slug's entries as they are.
func SyncBoltHold(deps Deps, req BoltHoldRequest) (BoltHoldReport, error) {
	report := BoltHoldReport{Written: []string{}, Dropped: []string{}}
	held := make(map[string]bool, len(req.Hold))
	for _, slug := range req.Hold {
		held[slug] = true
	}
	record, err := pauserecord.Read(deps.RecordPath)
	if err != nil {
		return report, err
	}

	var failures []error
	for _, slug := range req.Scope {
		holdsLock := false
		if req.HoldsParentLock != nil {
			if holdsLock, err = req.HoldsParentLock(slug); err != nil {
				failures = append(failures, fmt.Errorf("bolt hold: read whether %s holds its parent lock: %w", slug, err))
				continue
			}
		}
		own, hasOwn := ownEntry(record, slug)
		switch {
		case req.Standing && held[slug] && !holdsLock:
			if hasOwn && !(own.Caller == pauserecord.BoltHoldCaller && own.Machine != req.Machine) {
				continue
			}
			if hasOwn {
				// The hold moved to another machine: replace the entry so the refusals name the current one.
				if _, err := Lift(deps, EntryRef{Scope: slug, Caller: pauserecord.BoltHoldCaller, PausedAt: own.PausedAt}); err != nil {
					failures = append(failures, fmt.Errorf("bolt hold: lift the entry of %s: %w", slug, err))
					continue
				}
			}
			paused, err := Pause(deps, PauseRequest{Slug: slug, Grace: req.Grace, Caller: pauserecord.BoltHoldCaller, HoldMachine: req.Machine})
			if err != nil {
				failures = append(failures, fmt.Errorf("bolt hold: hold %s: %w", slug, err))
				continue
			}
			if !paused.AlreadyPaused {
				report.Written = append(report.Written, slug)
			}
		case hasOwn && own.Caller == pauserecord.BoltHoldCaller:
			lifted, err := Lift(deps, EntryRef{Scope: slug, Caller: pauserecord.BoltHoldCaller, PausedAt: own.PausedAt})
			if err != nil {
				failures = append(failures, fmt.Errorf("bolt hold: lift the entry of %s: %w", slug, err))
				continue
			}
			if len(lifted.Resumed) > 0 {
				report.Dropped = append(report.Dropped, slug)
			}
		}
	}
	return report, errors.Join(failures...)
}

// ownEntry returns the live entry of record whose scope is slug.
func ownEntry(record pauserecord.Record, slug string) (pauserecord.Entry, bool) {
	for _, live := range record.Live {
		if live.Scope == slug {
			return live, true
		}
	}
	return pauserecord.Entry{}, false
}

// readBoltHold returns the hold deps reads, none when the seam is unset.
func readBoltHold(deps Deps) (BoltHoldState, error) {
	if deps.BoltHold == nil {
		return BoltHoldState{}, nil
	}
	return deps.BoltHold()
}

// boltHoldRefusal is the refusal of a resume of slug while the hold stands.
// The slug's only covering entry is a bolt-hold entry.
func boltHoldRefusal(slug, machine string) error {
	if machine == "" {
		machine = "another machine"
	}
	return fmt.Errorf("tender: %s is held by an adopt on machine %s; way forward: wait for that adopt's hold to clear, "+
		"and the worktree's dobby lifts its entry at its next tick (with no dobby running, run lym beam start); "+
		"if machine %s has stopped, ask the operator", slug, machine, machine)
}

// replacementHoldEntries returns the bolt-hold entries that take the place of the slug entries in lifted.
// It returns none unless the hold stands.
// A hub entry and the hold's fix task get none.
func replacementHoldEntries(hold BoltHoldState, lifted []pauserecord.Entry, now time.Time) []pauserecord.Entry {
	if !hold.Standing {
		return nil
	}
	var entries []pauserecord.Entry
	for _, entry := range lifted {
		if entry.Scope == pauserecord.ScopeHub || entry.Scope == hold.FixTask {
			continue
		}
		entries = append(entries, pauserecord.Entry{
			Scope: entry.Scope, PausedAt: now, GraceEnd: now, Caller: pauserecord.BoltHoldCaller,
			Sessions: []pauserecord.Session{}, Machine: hold.Machine,
		})
	}
	return entries
}
