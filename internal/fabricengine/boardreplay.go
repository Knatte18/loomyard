// boardreplay.go declares the seam fabricengine reaches the board's entry-level merge through.
// boardengine imports fabricengine, so the seam is declared here and boardengine's implementation is installed process-wide.

package fabricengine

import (
	"errors"
	"sync/atomic"
	"time"
)

// BoardEntry is one board entry as the ended-set rule reads it.
// Status is empty when the entry carries none, and Created is the zero time when its creation is unknown.
type BoardEntry struct {
	Slug    string
	Status  string
	Created time.Time
}

// BoardDesignFile is one design file a local board commit touches, with its bytes at the merge base, before the commit and after it.
// A nil slice means the file is absent at that point.
type BoardDesignFile struct {
	Name   string
	Base   []byte
	Before []byte
	After  []byte
}

// BoardReplayRequest is one local board commit to re-apply onto origin's tip.
// BoardDir is the board directory checked out at origin's tip;
// BaseBoardJSON, ParentBoardJSON and CommitBoardJSON are the `board.json` bytes at the merge base, at the commit's parent and at the commit, each empty when the file is absent there.
type BoardReplayRequest struct {
	BoardDir        string
	BaseBoardJSON   []byte
	ParentBoardJSON []byte
	CommitBoardJSON []byte
	DesignFiles     []BoardDesignFile
}

// BoardReplayFinding names one entry-level or design-file change a replay did not apply and why.
// Field is empty when the whole entry is concerned, and Local is the local side's value of the field when it was lost to origin's.
type BoardReplayFinding struct {
	Entry  string
	Field  string
	Reason string
	Local  string
}

// BoardReplayResult reports what a replay applied.
// Empty is true when no entry-level or design-file change survived, so nothing was written.
// Lost holds the local values origin's side won, and Refused the changes a board check refused.
type BoardReplayResult struct {
	Empty   bool
	Lost    []BoardReplayFinding
	Refused []BoardReplayFinding
}

// BoardReplay merges a local board commit into origin's board by entry.
// Entries decodes a `board.json` into the entries the ended-set rule reads.
// Apply re-applies the request's commit onto the board directory it names, under its caller's board and push locks.
type BoardReplay interface {
	Entries(boardJSON []byte) ([]BoardEntry, error)
	Apply(req BoardReplayRequest) (BoardReplayResult, error)
}

// ErrBoardReplayMissing is returned by a read of the board replay seam while none is installed.
// It is a wiring guard: the lym root pre-run installs boardengine.FabricReplay through InstallBoardReplay, and a test binary calls boardkit.InstallReplay.
var ErrBoardReplayMissing = errors.New("fabricengine: no board replay installed: the lym root pre-run installs one through InstallBoardReplay, and a test binary through boardkit.InstallReplay")

var installedBoardReplay atomic.Pointer[BoardReplay]

// InstallBoardReplay stores r as the process's board replay and returns a function that reinstates the previous one.
func InstallBoardReplay(r BoardReplay) (restore func()) {
	previous := installedBoardReplay.Swap(&r)
	return func() { installedBoardReplay.Store(previous) }
}

// InstalledBoardReplay returns the process's board replay, or ErrBoardReplayMissing when none is installed.
func InstalledBoardReplay() (BoardReplay, error) {
	installed := installedBoardReplay.Load()
	if installed == nil {
		return nil, ErrBoardReplayMissing
	}
	return *installed, nil
}
