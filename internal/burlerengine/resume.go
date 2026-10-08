// resume.go probes a round's two halves for a previous session's work and resumes a round whose halves are both still live, joining them through the same rules a fresh round ends by.

package burlerengine

import (
	"errors"
	"fmt"
	"os"

	"github.com/Knatte18/loomyard/internal/logger"
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
	// Its StrandGUID names the strand a caller stops when it will not resume the round.
	Handle Handle
}

// LiveRound is what ProbeRound found of a round's two halves.
type LiveRound struct {
	Review LiveHalf
	Fix    LiveHalf
}

// ProbeRound reports the state of each half of the round p describes under opts, without waiting on either.
// A half is live when the shuttle holds a run for its one output file, even one whose output file is already on disk, as a reviewer repairing its review in the parse gate is;
// done when it has no live run and its output file is present; and gone otherwise.
// A probe error, and an invalid p, are returned as is.
func (e *Engine) ProbeRound(p Profile, opts RunOpts) (LiveRound, error) {
	if err := p.validate(e.geom.WorktreeRoot, e.cfg); err != nil {
		return LiveRound{}, err
	}

	review, fix := p.roundHalves(opts, "", "")
	var live LiveRound
	var err error
	if live.Review, err = e.probeHalf(review, p.ReviewPath); err != nil {
		return LiveRound{}, err
	}
	if live.Fix, err = e.probeHalf(fix, p.FixerReportPath); err != nil {
		return LiveRound{}, err
	}
	return live, nil
}

// probeHalf classifies one half by its shuttle probe and, failing a live run, by its output file.
func (e *Engine) probeHalf(half halfSpec, outputPath string) (LiveHalf, error) {
	handle, found, err := e.shuttle.ProbeGated(half.spec, half.gate)
	if err != nil {
		return LiveHalf{}, fmt.Errorf("burler: probe the %s half: %w", half.spec.Role, err)
	}
	if found {
		return LiveHalf{State: HalfLive, Handle: handle}, nil
	}
	if _, err := os.Stat(outputPath); err == nil {
		return LiveHalf{State: HalfDone}, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return LiveHalf{}, fmt.Errorf("burler: stat the %s half's output %q: %w", half.spec.Role, outputPath, err)
	}
	return LiveHalf{State: HalfGone}, nil
}

// Resume completes a round whose two halves are both live, attaching to them instead of starting either.
// It removes any ready marker it finds, since a finished reviewer's run is never live and a marker here is stale.
// It then joins the two handles as a fresh round does:
// the two waits run concurrently, the reviewer's completion releases the fixer through the marker, and the review is compared against the bytes it handed off.
// A LiveRound that does not hold both halves live is an error, and nothing is stopped or started.
func (e *Engine) Resume(p Profile, opts RunOpts, live LiveRound) (Result, error) {
	if live.Review.State != HalfLive || live.Fix.State != HalfLive {
		return Result{}, fmt.Errorf("burler: resume needs both halves live (review %d, fix %d)", live.Review.State, live.Fix.State)
	}
	if err := p.validate(e.geom.WorktreeRoot, e.cfg); err != nil {
		return Result{}, err
	}

	logger.Info("burler: resuming a live round", "round", opts.Round, "reviewStrand", live.Review.Handle.StrandGUID(), "fixStrand", live.Fix.Handle.StrandGUID())

	if err := os.Remove(p.ReadyMarkerPath); err != nil && !os.IsNotExist(err) {
		return Result{}, fmt.Errorf("burler: remove the stale ready marker %q: %w", p.ReadyMarkerPath, err)
	}
	return e.join(&p, opts, live.Review.Handle, live.Fix.Handle)
}
