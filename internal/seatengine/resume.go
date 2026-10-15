// resume.go probes a table's seats for a previous session's runs and resumes a table whose chair is still live.

package seatengine

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
)

// LiveTable is what Probe found of a table's seats.
// Chair is the chair's unwaited run, nil when the chair is not live, and Advisors holds the live advisors' runs keyed by seat name.
// work is the advisor work the probe wired into the chair's hold deferral, nil for a LiveTable the probe did not build.
type LiveTable struct {
	Chair    Handle
	Advisors map[string]Handle
	work     *advisorWork
}

// Probe reports which of table's seats have a live run, without waiting on any.
// The chair is probed with the table's gate and each advisor with its seat's gate, so an advisor whose record is terminal while its strand is still up, such as a done one kept by its pane or a timed-out one, is removed by the probe and answers not found.
// When the chair is live Probe returns its handle with every advisor it found.
// When it is not, Probe stops every live advisor and returns an empty LiveTable, so a caller archives and starts fresh only once no seat is live;
// a stop that fails is an error wrapping ErrSeatNotStopped.
// Probe archives nothing and starts nothing.
func (e *Engine) Probe(table Table) (LiveTable, error) {
	if err := table.Validate(e.geom.StencilsDir); err != nil {
		return LiveTable{}, err
	}

	work := &advisorWork{}
	chairHandle, chairFound, err := e.probeSeat(table, table.Chair(), work.outstanding)
	if err != nil {
		return LiveTable{}, err
	}
	advisors := make(map[string]Handle)
	for _, seat := range table.Seats {
		if seat.Name == leadName {
			continue
		}
		handle, found, err := e.probeSeat(table, seat, nil)
		if err != nil {
			return LiveTable{}, err
		}
		if found {
			advisors[seat.Name] = handle
		}
	}

	if chairFound {
		return LiveTable{Chair: chairHandle, Advisors: advisors, work: work}, nil
	}
	var stopErr error
	for _, seat := range table.Seats {
		if handle, live := advisors[seat.Name]; live {
			stopErr = errors.Join(stopErr, stopSeat(handle))
		}
	}
	return LiveTable{}, stopErr
}

// probeSeat probes the shuttle for a live run of seat, under the gate the seat starts with.
// holdDeferral is the chair's held-turn deferral, which an attached chair needs in its spec as a fresh one has it; an advisor is probed with none.
func (e *Engine) probeSeat(table Table, seat Seat, holdDeferral func() bool) (Handle, bool, error) {
	spec := seatSpec(table, seat, "")
	spec.HoldDeferral = holdDeferral
	handle, found, err := e.shuttle.ProbeGated(spec, seatGate(table, seat))
	if err != nil {
		return nil, false, fmt.Errorf("seatengine: probe seat %q: %w", seat.Name, err)
	}
	return handle, found, nil
}

// Resume completes a table whose chair is live, attaching to the chair and every advisor the probe found instead of starting any.
// It joins them as a fresh run does.
// The chair is told again that each found advisor is up, since the line may not have landed before the interruption.
// An advisor the probe did not find is never restarted, and the chair is told of it once:
// an advisor whose outputs all exist is finished, any other has ended.
// The chair's hold deferral reads the work Probe wired into it, which Resume binds to its sender and adds every found advisor to, so an attached chair defers as a fresh one does;
// a LiveTable built without Probe carries no work, and the attached chair then has nothing new to read.
// A found required advisor is watched with the same Accept rule as in a fresh run, and a required advisor the probe did not find ends the step at once: the chair is stopped, no line is queued, and the advisor's result carries EndedStep with no terminal result.
// A notice sent before the interruption may land again, which costs the chair nothing.
// A LiveTable without a live chair is an error, and nothing is stopped or started.
func (e *Engine) Resume(table Table, live LiveTable) (Result, error) {
	if live.Chair == nil {
		return Result{}, fmt.Errorf("seatengine: resume needs a live chair; way forward: run the row again so it starts the table fresh")
	}
	if err := table.Validate(e.geom.StencilsDir); err != nil {
		return Result{}, err
	}
	names, err := strandNames(e.geom, table)
	if err != nil {
		return Result{}, err
	}
	logger.Info("seatengine: resuming a live table", "chairStrand", live.Chair.StrandGUID(), "liveAdvisors", len(live.Advisors))

	sender := newNoticeSender(e.absolute(table.Chair().Outputs), e.noticeRetry)
	work := live.work
	if work == nil {
		work = &advisorWork{}
	}
	work.bindSender(sender)
	ender := newStepEnder()
	var advisors []*seatRun
	for _, seat := range table.Seats {
		if seat.Name == leadName {
			continue
		}
		outputs := e.absolute(seat.Outputs)
		if handle, found := live.Advisors[seat.Name]; found {
			sender.queue(advisorUpNotice(names[seat.Name]))
			attached := newSeatRun(seat, names[seat.Name], handle, outputs, sender, ender)
			work.add(attached)
			advisors = append(advisors, attached)
			continue
		}
		lost := &seatRun{seat: seat, name: names[seat.Name]}
		advisors = append(advisors, lost)
		if seat.Required {
			ender.end(lost)
			continue
		}
		lost.notice = resumeNotice(names[seat.Name], outputs)
		sender.queue(lost.notice)
	}
	sender.start(live.Chair)
	return e.join(table, live.Chair, advisors, sender, ender)
}

// resumeNotice returns the one-line notice that tells the chair advisor name is not running any more at resume.
// An advisor whose outputs all exist finished, and one that lacks an output ended without finishing.
func resumeNotice(name string, outputs []string) string {
	if !allExist(outputs) {
		return advisorNotice(name, outputs, "")
	}
	return fmt.Sprintf("Advisor %s finished before this step was resumed and no longer answers; its outputs stand: %s", name, strings.Join(outputs, ", "))
}
