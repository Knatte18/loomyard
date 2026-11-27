// raddlewrite.go implements the Raddle-Write producer, a decorator around the table-driven session producer.
// It records the run's mode and numbers each session.
// It starts a fresh session after a return or a goto.
// It commits the raddle directory once a session is done.

package loomshed

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// RaddleWriteDeps holds the paths and seams the Raddle-Write producer reads and writes.
type RaddleWriteDeps struct {
	// StatePath and StateLockPath are the run's state record and its lock.
	StatePath     string
	StateLockPath string
	// ProposalPath is the proposal file; ReturnedPath is where a return in auto or perch mode sets it aside.
	ProposalPath string
	ReturnedPath string
	// ColdPath is where the run's first passing proposal is stored once.
	ColdPath string
	// ApprovalPath is the approval record the approve verb writes.
	ApprovalPath string
	// SplitPerch reports whether the recipe's rows hold a split perch, a Bouncer row over the proposal.
	SplitPerch func() (bool, error)
	// AutoApprove reports the raddle_auto_approve loom key.
	// The key is read through the run's setting lookup, so a profile's raddle block may set it.
	AutoApprove func() (bool, error)
	// FreshEntry reports whether the row's current attempt began with a Raddle-File return or an operator goto.
	// It reads the status history and gives the history position of that entry, which is zero when fresh is false.
	FreshEntry func() (fresh bool, position int, err error)
	// StoreCold copies the proposal to ColdPath and sets cold_stored unless already stored.
	// It is the raddle gate's onPass in auto and perch mode.
	// It is a no-op in interactive mode, where the propose verb stores the cold proposal.
	StoreCold func() error
	// Commit commits the raddle directory.
	Commit func() error
}

// raddleWrite is the Raddle-Write producer.
type raddleWrite struct {
	name  string
	inner shedengine.ShedProducer
	deps  RaddleWriteDeps
}

var _ shedengine.ShedProducer = (*raddleWrite)(nil)

// NewRaddleWrite returns the producer of the Raddle-Write row, named name.
// It wraps inner, the table-driven session producer.
func NewRaddleWrite(name string, inner shedengine.ShedProducer, deps RaddleWriteDeps) shedengine.ShedProducer {
	return &raddleWrite{name: name, inner: inner, deps: deps}
}

// Call implements shedengine.ShedProducer.
// Each state update is one step, so a resume after a crash repeats none of them.
// The first call records the run's mode.
// It is perch when SplitPerch reports a split perch, with no read of AutoApprove.
// Otherwise it is auto or interactive from AutoApprove.
// Every later call and the Raddle-File read take the mode from the state instead.
// Every call raises the session count.
// A fresh entry not yet taken up at its history position sets the last attempt's output aside.
// Auto and perch mode move the proposal to ReturnedPath.
// Interactive mode removes the approval record and clears the state's approval.
// Either way the seat starts a new session.
// The inner producer then runs; on Done the return reason is cleared and the raddle directory committed.
// An error or a Stuck verdict of the inner producer is returned as is, with no commit.
func (p *raddleWrite) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, p.name); err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	recorded, err := ReadRaddleState(p.deps.StatePath, p.deps.StateLockPath)
	if err != nil {
		return "", shedengine.OutputPointer{}, p.fail("read the run's state", err)
	}
	mode := recorded.Mode
	if mode == "" {
		first, err := p.firstMode()
		if err != nil {
			return "", shedengine.OutputPointer{}, err
		}
		mode = first
	}
	current, err := UpdateRaddleState(p.deps.StatePath, p.deps.StateLockPath, func(stored *RaddleState) {
		if stored.Mode == "" {
			stored.Mode = mode
		}
		stored.Session++
	})
	if err != nil {
		return "", shedengine.OutputPointer{}, p.fail("record the session", err)
	}

	fresh, position, err := p.deps.FreshEntry()
	if err != nil {
		return "", shedengine.OutputPointer{}, p.fail("read the row's entry from the run's history", err)
	}
	if fresh && position != current.EntryPosition {
		if err := p.clearLastAttempt(current.Mode); err != nil {
			return "", shedengine.OutputPointer{}, p.fail("clear the last attempt's output", err)
		}
		if _, err := UpdateRaddleState(p.deps.StatePath, p.deps.StateLockPath, func(stored *RaddleState) {
			stored.EntryPosition = position
			if current.Mode == loomengine.RaddleModeInteractive {
				stored.Approval = nil
			}
		}); err != nil {
			return "", shedengine.OutputPointer{}, p.fail("record the taken entry", err)
		}
	}

	outcome, pointer, err := p.inner.Call(ctx)
	if err != nil || outcome != shedengine.Done {
		return outcome, pointer, err
	}

	if _, err := UpdateRaddleState(p.deps.StatePath, p.deps.StateLockPath, func(stored *RaddleState) { stored.ReturnReason = "" }); err != nil {
		return "", shedengine.OutputPointer{}, p.fail("clear the return reason", err)
	}
	if err := p.deps.Commit(); err != nil {
		return "", shedengine.OutputPointer{}, p.fail("commit the raddle directory", err)
	}
	return outcome, pointer, nil
}

// firstMode returns the mode the run's first call records.
// It is perch when the recipe's rows hold a split perch, and then the raddle_auto_approve setting is never read;
// otherwise it is auto or interactive by that setting.
func (p *raddleWrite) firstMode() (loomengine.RaddleMode, error) {
	perch, err := p.deps.SplitPerch()
	if err != nil {
		return "", p.fail("read the recipe's rows for a split perch", err)
	}
	if perch {
		return loomengine.RaddleModePerch, nil
	}
	auto, err := p.deps.AutoApprove()
	if err != nil {
		return "", p.fail("read the raddle_auto_approve setting", err)
	}
	if auto {
		return loomengine.RaddleModeAuto, nil
	}
	return loomengine.RaddleModeInteractive, nil
}

// clearLastAttempt removes the output of the last attempt in mode.
// The seat's output file must not exist when its new session starts.
// A file already gone is no error, since a crash may have cleared it before the state recorded the entry.
func (p *raddleWrite) clearLastAttempt(mode loomengine.RaddleMode) error {
	var err error
	if mode == loomengine.RaddleModeInteractive {
		err = os.Remove(p.deps.ApprovalPath)
	} else {
		err = os.Rename(p.deps.ProposalPath, p.deps.ReturnedPath)
	}
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// fail returns the error of a failed step, naming the producer and the re-step that retries it.
func (p *raddleWrite) fail(step string, err error) error {
	return fmt.Errorf("loomshed: %s: %s: %w; way forward: the fault is transient, re-step the %s row", p.name, step, err, p.name)
}
