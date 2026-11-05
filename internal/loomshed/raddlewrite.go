// raddlewrite.go implements the Raddle-Write producer: a decorator that records the run's mode, numbers each session of the row, gives a return or a goto a fresh start, and commits the raddle directory once the session is done.

package loomshed

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// RaddleWriteDeps are the told paths and seams of the Raddle-Write producer.
type RaddleWriteDeps struct {
	// StatePath and StateLockPath are the run's state record and its lock.
	StatePath     string
	StateLockPath string
	// ProposalPath is the proposal file and ReturnedPath where a return in auto mode sets it aside.
	ProposalPath string
	ReturnedPath string
	// ColdPath is where the run's first passing proposal is stored once.
	ColdPath string
	// ApprovalPath is the approval record the approve verb writes.
	ApprovalPath string
	// AutoApprove answers the loom key raddle_auto_approve through the run's setting lookup, so a profile's raddle block may set it.
	AutoApprove func() (bool, error)
	// FreshEntry reports whether the row's current attempt began with a Raddle-File return or an operator goto into the row, read from the run's status history, with the history position of that entry.
	// Position is zero when fresh is false.
	FreshEntry func() (fresh bool, position int, err error)
	// StoreCold copies the proposal to ColdPath and sets cold_stored unless already stored.
	// It is the raddle gate's onPass in auto mode and a no-op in interactive mode, where the propose verb stores the cold proposal.
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

// NewRaddleWrite returns the producer of the Raddle-Write row, identified as name, around inner, the table-driven session producer.
func NewRaddleWrite(name string, inner shedengine.ShedProducer, deps RaddleWriteDeps) shedengine.ShedProducer {
	return &raddleWrite{name: name, inner: inner, deps: deps}
}

// Call implements shedengine.ShedProducer.
// Each state update below is one step, so a resume after a crash repeats none of them:
// the first call records the run's mode from AutoApprove, which every later call and Raddle-File read from the state instead of the key, and every call raises the session count.
// A fresh entry not yet taken up at its history position sets the last attempt's output aside, so the seat starts a new session: auto mode moves the proposal to ReturnedPath, and interactive mode removes the approval record and clears the state's approval.
// The inner producer then runs, and on Done the return reason is cleared and the raddle directory committed.
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
		auto, err := p.deps.AutoApprove()
		if err != nil {
			return "", shedengine.OutputPointer{}, p.fail("read the raddle_auto_approve setting", err)
		}
		mode = RaddleModeInteractive
		if auto {
			mode = RaddleModeAuto
		}
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
			if current.Mode == RaddleModeInteractive {
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

// clearLastAttempt removes the output of the last attempt in mode, so the seat's output file does not exist when its new session starts.
// A file already gone is no error, since a crash may have cleared it before the state recorded the entry.
func (p *raddleWrite) clearLastAttempt(mode RaddleMode) error {
	var err error
	if mode == RaddleModeAuto {
		err = os.Rename(p.deps.ProposalPath, p.deps.ReturnedPath)
	} else {
		err = os.Remove(p.deps.ApprovalPath)
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
