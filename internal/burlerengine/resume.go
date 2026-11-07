// resume.go classifies the halves of a round for a resumed session.
// It defines the probed half states and the live table that a half's resume reuses.

package burlerengine

import (
	"errors"
	"fmt"
	"os"

	"github.com/Knatte18/loomyard/internal/seatengine"
)

// HalfState is what a probe found of one half of a round.
type HalfState int

const (
	// HalfGone means the half has no live run and its output file is absent.
	HalfGone HalfState = iota
	// HalfDone means the half has no live run and its output file is present.
	HalfDone
	// HalfLive means the half has a live run, even when its output file is already on disk.
	HalfLive
)

// LiveHalf is one half's probed state and, for a live half, the handle of its run.
type LiveHalf struct {
	State HalfState
	// Handle is the live run, set only when State is HalfLive.
	// Its StrandGUID names the strand a caller stops when it will not resume the half.
	Handle Handle
	// table is what the seats engine's probe found.
	// A half's Resume hands it back to the seats engine.
	// It is nil unless ProbeReview or ProbeFix probed the half live.
	table *seatengine.LiveTable
}

// probeHalf reports the state of the one-seat table's chair, without waiting on it:
// live when the seats engine's probe finds a live run.
// done when it finds none and outputPath exists, and gone otherwise.
// A seat that could not be stopped is returned wrapped in ErrHalfNotStopped.
func (e *Engine) probeHalf(table seatengine.Table, outputPath string) (LiveHalf, error) {
	found, err := e.seats.Probe(table)
	if errors.Is(err, seatengine.ErrSeatNotStopped) {
		return LiveHalf{}, fmt.Errorf("%w: %w", ErrHalfNotStopped, err)
	}
	if err != nil {
		return LiveHalf{}, err
	}
	half, err := classifyHalf(found.Chair, outputPath)
	if err != nil {
		return LiveHalf{}, err
	}
	if half.State == HalfLive {
		half.table = &found
	}
	return half, nil
}

// classifyHalf classifies one half from its probed handle: a non-nil handle is a live run.
// Without one, the half is done when outputPath exists and gone otherwise.
// A stat failure other than a missing file is returned.
func classifyHalf(handle seatengine.Handle, outputPath string) (LiveHalf, error) {
	if handle != nil {
		return LiveHalf{State: HalfLive, Handle: handle}, nil
	}
	if _, err := os.Stat(outputPath); err == nil {
		return LiveHalf{State: HalfDone}, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return LiveHalf{}, fmt.Errorf("burler: stat the half's output %q: %w", outputPath, err)
	}
	return LiveHalf{State: HalfGone}, nil
}
