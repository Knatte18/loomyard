// boltreplay.go declares the registry of Bolt replayers: the seam a local commit to a Bolt file is re-applied by entry through, and the run ledger the ended-set and owner reads go through.
// A replayer lives in the package that owns its files, which imports fabricengine, so the seam is declared here and each replayer is registered process-wide.

package fabricengine

import (
	"errors"
	"fmt"
	"slices"
	"sync/atomic"
	"time"
)

// ReplayFile is one Bolt file a local commit touches, with its bytes at the merge base, before the commit and after it.
// Path is relative to the Bolt root and slash-separated, and a nil slice means the file is absent at that point.
type ReplayFile struct {
	Path   string
	Base   []byte
	Before []byte
	After  []byte
}

// ReplayRequest is one local commit's changes to a replayer's files, to re-apply onto origin's tip.
// Dir is the Bolt checkout at origin's tip, and Files holds the commit's files that replayer claims.
type ReplayRequest struct {
	Dir   string
	Files []ReplayFile
}

// ReplayFinding names one entry-level or file-level change a replay did not apply and why.
// Field is empty when the whole entry is concerned, and Local is the local side's value of the field when it was lost to origin's.
type ReplayFinding struct {
	Entry  string
	Field  string
	Reason string
	Local  string
}

// ReplayResult reports what a replayer applied.
// Empty is true when no change survived, so nothing was written.
// Lost holds the local values origin's side won, and Refused the changes the replayer's own checks refused.
type ReplayResult struct {
	Empty   bool
	Lost    []ReplayFinding
	Refused []ReplayFinding
}

// BoltReplayer re-applies, by entry, a local commit's changes to its own files onto the Bolt checkout at origin's tip.
// Name identifies the replayer in the registry and in a set-aside tag.
// Claims reports whether a Bolt-root-relative slash path is one of its files.
// Apply re-applies the request's files onto the directory it names, under its caller's locks.
type BoltReplayer interface {
	Name() string
	Claims(path string) bool
	Apply(req ReplayRequest) (ReplayResult, error)
}

// LedgerRun is one run as the run ledger records it.
// Created is the zero time when the run's creation is unknown.
type LedgerRun struct {
	Slug    string
	Ended   bool
	Created time.Time
}

// RunLedger is the part of a Bolt file that records the hub's runs, which one registered replayer may also implement.
// File is the Bolt file holding the runs.
// Runs decodes that file's bytes into its runs, an empty slice holding none.
// CommitRuns returns the slugs of the runs a commit of paths touches, given the ledger file's bytes before and after the commit, each nil when the commit leaves the file alone.
type RunLedger interface {
	File() string
	Runs(data []byte) ([]LedgerRun, error)
	CommitRuns(before, after []byte, paths []string) ([]string, error)
}

// ErrReplayerMissing is returned by a read that needs the run ledger while none is registered.
// It is a wiring guard: the lym root pre-run registers the replayers, and a test binary registers what it needs.
var ErrReplayerMissing = errors.New("fabricengine: no Bolt replayer registered")

var boltReplayerSet atomic.Pointer[[]BoltReplayer]

// updateReplayers atomically replaces the registered set with what update returns for it.
// update may run more than once and never changes the slice it is given.
func updateReplayers(update func(registered []BoltReplayer) []BoltReplayer) {
	for {
		current := boltReplayerSet.Load()
		var registered []BoltReplayer
		if current != nil {
			registered = *current
		}
		next := update(registered)
		if boltReplayerSet.CompareAndSwap(current, &next) {
			return
		}
	}
}

// RegisterBoltReplayer adds r to the registered replayers, replacing the one of the same name, and returns a function that puts back only what that name held before.
// Putting back leaves every other name's registration as it then stands, so parallel tests may share one registration.
func RegisterBoltReplayer(r BoltReplayer) (restore func()) {
	name := r.Name()
	var previous BoltReplayer
	updateReplayers(func(registered []BoltReplayer) []BoltReplayer {
		previous = nil
		next := slices.Clone(registered)
		if index := slices.IndexFunc(next, func(held BoltReplayer) bool { return held.Name() == name }); index >= 0 {
			previous = next[index]
			next[index] = r
			return next
		}
		return append(next, r)
	})
	return func() {
		updateReplayers(func(registered []BoltReplayer) []BoltReplayer {
			next := slices.Clone(registered)
			index := slices.IndexFunc(next, func(held BoltReplayer) bool { return held.Name() == name })
			switch {
			case previous != nil && index >= 0:
				next[index] = previous
			case previous != nil:
				next = append(next, previous)
			case index >= 0:
				next = slices.Delete(next, index, index+1)
			}
			return next
		})
	}
}

// registeredReplayers returns the registered replayers in registration order.
func registeredReplayers() []BoltReplayer {
	current := boltReplayerSet.Load()
	if current == nil {
		return nil
	}
	return *current
}

// claimantOf returns the first registered replayer that claims path, a Bolt-root-relative slash path.
func claimantOf(path string) (BoltReplayer, bool) {
	for _, replayer := range registeredReplayers() {
		if replayer.Claims(path) {
			return replayer, true
		}
	}
	return nil, false
}

// runLedger returns the registered run ledger and the name of the replayer that implements it.
// It returns ErrReplayerMissing, wrapped with the way forward, when no registered replayer is a run ledger.
func runLedger() (RunLedger, string, error) {
	for _, replayer := range registeredReplayers() {
		if ledger, isLedger := replayer.(RunLedger); isLedger {
			return ledger, replayer.Name(), nil
		}
	}
	return nil, "", fmt.Errorf("%w: the lym root pre-run registers the replayers through RegisterBoltReplayer, and a test binary registers what it needs, boardkit.InstallReplay registering the board's", ErrReplayerMissing)
}
