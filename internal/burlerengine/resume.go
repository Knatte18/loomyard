// resume.go probes a round's two seats for a previous session's work and resumes a round whose seats are both still live, deciding it through the same rules a fresh round ends by.

package burlerengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
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
// table holds what the seats engine's probe found, which Resume hands back to it; it is nil unless the fixer was live.
type LiveRound struct {
	Review LiveHalf
	Fix    LiveHalf
	table  *seatengine.LiveTable
}

// probeTable returns the seat table a probe or a resume of p under opts addresses.
// A probe composes no prompt and starts nothing, so the instruction files are named under a directory the round never creates.
func (e *Engine) probeTable(p *Profile, opts RunOpts, accept func(shuttleengine.Result) error) seatengine.Table {
	probeDir := filepath.Join(e.geom.AnchorPath, lyxdirs.DotLyxDirName, "burler", "round-probe")
	reviewerValues, fixerValues := roundSeatValues(p, roundFilesIn(probeDir))
	return p.roundTable(opts, roundPrompts{ReviewerValues: reviewerValues, FixerValues: fixerValues}, accept)
}

// ProbeRound reports the state of each half of the round p describes under opts, without waiting on either.
// A half is live when the seats engine's probe finds a live run of its seat, even one whose output file is already on disk, as a reviewer repairing its review in the parse gate is;
// done when it has no live run and its output file is present; and gone otherwise.
// A live reviewer beside a fixer that is not live is stopped by the probe and reported by its output file, so a caller starts a fresh round.
// A probe error, and an invalid p, are returned as is, a seat that could not be stopped wrapped in ErrHalfNotStopped.
func (e *Engine) ProbeRound(p Profile, opts RunOpts) (LiveRound, error) {
	if err := p.validate(e.geom.WorktreeRoot, e.cfg); err != nil {
		return LiveRound{}, err
	}

	found, err := e.seats.Probe(e.probeTable(&p, opts, (&roundAcceptance{profile: &p}).accept))
	if errors.Is(err, seatengine.ErrSeatNotStopped) {
		return LiveRound{}, fmt.Errorf("%w: %w", ErrHalfNotStopped, err)
	}
	if err != nil {
		return LiveRound{}, err
	}

	var live LiveRound
	if found.Chair != nil {
		live.table = &found
	}
	if live.Fix, err = classifyHalf(found.Chair, p.FixerReportPath); err != nil {
		return LiveRound{}, err
	}
	if live.Review, err = classifyHalf(found.Advisors[seatengine.AdvisorName(1)], p.ReviewPath); err != nil {
		return LiveRound{}, err
	}
	return live, nil
}

// classifyHalf classifies one half by its probed handle, nil for a seat the probe did not find, and failing a live run by its output file.
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

// Resume completes a round whose two halves are both live, attaching to them instead of starting either.
// It removes any ready marker it finds, since a finished reviewer's run is never live and a marker here is stale.
// It then resumes the seat table as a fresh round runs it:
// the reviewer's completion releases the fixer through the marker, and the review is compared against the bytes it handed off.
// A LiveRound that does not hold both halves live is an error, and so is one with no probed table, which only a LiveRound not taken from ProbeRound lacks; nothing is stopped or started.
func (e *Engine) Resume(p Profile, opts RunOpts, live LiveRound) (Result, error) {
	if live.Review.State != HalfLive || live.Fix.State != HalfLive {
		return Result{}, fmt.Errorf("burler: resume needs both halves live (review %d, fix %d)", live.Review.State, live.Fix.State)
	}
	if live.table == nil {
		return Result{}, fmt.Errorf("burler: resume needs the table ProbeRound found; way forward: take the LiveRound from ProbeRound, or run the row again so it starts the round fresh")
	}
	if err := p.validate(e.geom.WorktreeRoot, e.cfg); err != nil {
		return Result{}, err
	}

	logger.Info("burler: resuming a live round", "round", opts.Round, "reviewStrand", live.Review.Handle.StrandGUID(), "fixStrand", live.Fix.Handle.StrandGUID())

	if err := os.Remove(p.ReadyMarkerPath); err != nil && !os.IsNotExist(err) {
		return Result{}, fmt.Errorf("burler: remove the stale ready marker %q: %w", p.ReadyMarkerPath, err)
	}
	acceptance := &roundAcceptance{profile: &p}
	seats, err := e.seats.Resume(e.probeTable(&p, opts, acceptance.accept), *live.table)
	return e.concludeRound(&p, opts, seats, acceptance, err)
}
