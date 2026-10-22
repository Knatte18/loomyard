// originseams.go builds the seams the origin reconcile takes by injection: whether a pair is live, whether the prime branch's parent lock is free, how an ended entry's pair is torn down, and who owns an entry.
// Every call site's CLI package builds them here, so the reconcile sees one reading of a live pair and one local removal.

package pairteardown

import (
	"context"
	"errors"
	"fmt"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/pauserecord"
)

// SeamOptions carries what the origin reconcile's call site knows and Teardown does not.
// HubPaused reports whether a live hub pause entry covers slug, whose driver is then interrupted; LivePair calls it each time it is called, so a pause lifted meanwhile reads lifted.
// PrimeParentLockFree reads once whether the prime branch's parent lock record is free; nil reads as free.
// WakeHeld reports whether a live wake-written pair pause entry of slug exists, and LiftWakeHold lifts it; both nil read as no wake hold.
// Machine reads this machine's name, and a nil Machine builds no owner seam, so every entry reads as owned here.
// ParentOf names the slug of the pair slug was forked from, or "" for the prime; nil reads every entry's parent as the prime.
type SeamOptions struct {
	HubPaused           func(slug string) (bool, error)
	PrimeParentLockFree func() (bool, error)
	WakeHeld            func(slug string) (bool, error)
	LiftWakeHold        func(slug string) error
	Machine             func() (string, error)
	ParentOf            func(slug string) (string, error)
}

// HubSeamOptions returns the options a call site with only the prime's location needs.
// HubPaused reads the hub's pause record at each call, and PrimeParentLockFree lists the parent lock store once for the prime's branch.
// Machine reads the machine's name, and ParentOf resolves a pair's parent slug from its origin record over the prime's location.
// WakeHeld and LiftWakeHold are left nil, which read as no wake hold, for the call site that knows one to fill.
func HubSeamOptions(prime *lyxcwd.Location) SeamOptions {
	return SeamOptions{
		Machine: machinename.Name,
		ParentOf: func(slug string) (string, error) {
			return fabricengine.ParentSlug(prime, slug)
		},
		HubPaused: func(slug string) (bool, error) {
			geometry, err := hubgeom.PauseGeometry(prime)
			if err != nil {
				return false, err
			}
			record, err := pauserecord.Read(geometry.RecordPath)
			if err != nil {
				return false, err
			}
			_, covered := record.Covering(slug)
			return covered, nil
		},
		PrimeParentLockFree: func() (bool, error) {
			branch, err := gitrepo.New(prime.WorktreePath()).CurrentBranch()
			if err != nil {
				return false, fmt.Errorf("read the prime's branch: %w", err)
			}
			store := &parentlock.Store{Dir: parentlock.Dir(fabricengine.BoardDir(prime.HubPath))}
			entries, err := store.List()
			if err != nil {
				return false, err
			}
			for _, entry := range entries {
				if entry.Record.Branch == branch && (entry.Alive || entry.Err != "") {
					return false, nil
				}
			}
			return true, nil
		},
	}
}

// OwnerReader builds the owner question the origin reconcile asks over one `board.json`: the effective owner of a slug and whether that owner is another machine.
// A machine error, an empty hostname included, a parentOf error and undecodable board bytes are returned, so no owner is ever compared with an empty name.
// parentOf names the slug of a pair's parent in the hub at hubPath, or "" for the prime.
func OwnerReader(machine func() (string, error), parentOf func(hubPath, slug string) (string, error)) fabricengine.OwnerReader {
	return func(hubPath string, boardJSON []byte, slug string) (string, bool, error) {
		here, err := machine()
		if err != nil {
			return "", false, fmt.Errorf("read this machine's name: %w", err)
		}
		owners, err := boardengine.ParseOwners(boardJSON)
		if err != nil {
			return "", false, fmt.Errorf("read the board's owners: %w", err)
		}
		owner, err := owners.Effective(slug, func(entry string) (string, error) { return parentOf(hubPath, entry) })
		if err != nil {
			return "", false, fmt.Errorf("read the owner of %s: %w", slug, err)
		}
		return owner, boardengine.OwnedElsewhere(owner, here), nil
	}
}

// OriginSeams returns the seams the origin reconcile reaches by injection.
// LivePair is the negation of the quiet rule's probe, except that a slug a live hub pause entry covers reads not live.
// OwnerOf is the owner reader over the options' Machine and ParentOf, and nil when Machine is nil.
// Teardown removes an ended entry's pair in local mode with a zero quiet wait, refusing a busy driver, and reports a busy driver or a busy lock as the report's busy flag.
// The force case, a pair with a wake-written pause entry or an earlier snapshot tag, instead ends the driver, snapshots the pair's uncommitted paths and, once they are kept, lifts the wake hold.
func (t *Teardown) OriginSeams(opts SeamOptions) fabricengine.OriginSeams {
	var ownerOf fabricengine.OwnerReader
	if opts.Machine != nil {
		ownerOf = OwnerReader(opts.Machine, func(_, slug string) (string, error) {
			if opts.ParentOf == nil {
				return "", nil
			}
			return opts.ParentOf(slug)
		})
	}
	return fabricengine.OriginSeams{
		OwnerOf: ownerOf,
		LivePair: func(slug string) (bool, error) {
			if opts.HubPaused != nil {
				paused, err := opts.HubPaused(slug)
				if err != nil {
					return false, err
				}
				if paused {
					return false, nil
				}
			}
			state, err := t.quiet(slug)
			if err != nil {
				return false, err
			}
			return !state.quiet, nil
		},
		PrimeParentLockFree: opts.PrimeParentLockFree,
		Teardown: func(request fabricengine.EndedTeardown) (fabricengine.TeardownReport, error) {
			return t.teardownEnded(opts, request)
		},
	}
}

// teardownEnded runs the local removal of one ended entry's pair.
func (t *Teardown) teardownEnded(opts SeamOptions, ended fabricengine.EndedTeardown) (fabricengine.TeardownReport, error) {
	wakeHeld := false
	if opts.WakeHeld != nil {
		var err error
		if wakeHeld, err = opts.WakeHeld(ended.Slug); err != nil {
			return fabricengine.TeardownReport{}, err
		}
	}

	request := Request{Slug: ended.Slug, Local: true, RefuseWhenBusy: true}
	if wakeHeld || ended.EarlierSnapshot {
		request.RefuseWhenBusy = false
		request.Force = true
		request.Snapshot = true
	}
	if wakeHeld && opts.LiftWakeHold != nil {
		request.AfterSnapshot = func() error { return opts.LiftWakeHold(ended.Slug) }
	}

	result, err := t.Run(context.Background(), request)
	report := fabricengine.TeardownReport{MutationRecord: result.Removal.MutationRecord}
	if result.Removal.SnapshotTag != "" {
		report.Tags = append(report.Tags, result.Removal.SnapshotTag)
	}
	if result.Removal.SetAsideTag != "" {
		report.Tags = append(report.Tags, result.Removal.SetAsideTag)
	}
	if reason := result.Removal.CodeBranchKeptReason; reason != "" {
		report.Kept = append(report.Kept, reason)
	}

	switch {
	case errors.Is(err, ErrDriverBusy):
		report.Busy, report.BusyOn = true, fabricengine.BusyOnDriver
		return report, nil
	case errors.Is(err, fabricengine.ErrPairBusy):
		report.Busy, report.BusyOn = true, fabricengine.BusyOnLock
		return report, nil
	}
	return report, err
}
