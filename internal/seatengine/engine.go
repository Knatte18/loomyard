// engine.go declares Engine, which starts a table's seats fresh, joins them and decides the step from the chair's result, and the result types it returns.

package seatengine

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// defaultNoticeRetry is how long the notice sender waits before retrying a line the chair's session could not take.
const defaultNoticeRetry = 5 * time.Second

// ErrSeatNotStopped marks a seat whose strand could not be stopped; its message ends with the way forward.
var ErrSeatNotStopped = errors.New("seatengine: a seat could not be stopped")

// SeatResult is what one advisor's run came to.
// Name is the seat's name in the table.
// StrandGUID and RunDir are empty for an advisor that never started, and StartError then holds why.
// Outcome is the advisor's own terminal outcome, empty while it has none.
// Result is its terminal shuttle result, zero while it has none, and WaitErr is its wait's error.
// AcceptErr is the error its Accept hook returned.
// EndedStep is true for a required advisor whose end ended the step before the chair's.
// Notified reports whether the chair was told, from Go, that the advisor ended or never started.
type SeatResult struct {
	Name       string
	StrandGUID string
	RunDir     string
	StartError string
	Outcome    shuttleengine.Outcome
	Result     shuttleengine.Result
	WaitErr    error
	AcceptErr  error
	EndedStep  bool
	Notified   bool
}

// Result is what one run of a table came to.
// Chair is the chair's result exactly as shuttle reported it, and ChairOutputs are the chair's output paths.
// Advisors holds one entry per advisor seat, in table order.
type Result struct {
	Chair        shuttleengine.Result
	ChairOutputs []string
	Advisors     []SeatResult
}

// Engine runs a table's seats through a Shuttle, in the paths its Geometry tells it.
type Engine struct {
	shuttle     Shuttle
	geom        Geometry
	noticeRetry time.Duration
}

// New returns an Engine that starts and probes seats through shuttle and reads the paths geom tells it.
func New(shuttle Shuttle, geom Geometry) *Engine {
	return &Engine{shuttle: shuttle, geom: geom, noticeRetry: defaultNoticeRetry}
}

// seatRun is one advisor: its handle, and what its goroutine recorded when its wait ended, or why it never started.
type seatRun struct {
	seat   Seat
	name   string
	handle Handle
	done   chan struct{}

	mu         sync.Mutex
	startError string
	outcome    shuttleengine.Outcome
	terminal   shuttleengine.Result
	waitErr    error
	acceptErr  error
	endedStep  bool
	notice     string
}

// stepEnder records the first required advisor whose end ends the step, and wakes the join that waits on the chair.
type stepEnder struct {
	once    sync.Once
	signal  chan struct{}
	advisor *seatRun
}

func newStepEnder() *stepEnder {
	return &stepEnder{signal: make(chan struct{})}
}

// end records advisor as the one that ended the step; only the first call counts.
func (s *stepEnder) end(advisor *seatRun) {
	s.once.Do(func() {
		s.advisor = advisor
		close(s.signal)
	})
}

// fired reports whether a required advisor has ended the step.
func (s *stepEnder) fired() bool {
	select {
	case <-s.signal:
		return true
	default:
		return false
	}
}

// newSeatRun returns the run of an advisor that started, and begins waiting on it.
// An advisor that ends without finishing queues one notice on sender, unless it is required, which ends the step on ender instead.
func newSeatRun(seat Seat, name string, handle Handle, outputs []string, sender *noticeSender, ender *stepEnder) *seatRun {
	run := &seatRun{seat: seat, name: name, handle: handle, done: make(chan struct{})}
	go run.watch(outputs, sender, ender)
	return run
}

// watch waits on the advisor's run and records how it ended.
// An advisor that reached done queues nothing, since it stays reachable and the chair reads its file.
// A required advisor that ended done with its gate passed has its Accept hook called once with the terminal result; every other end of a required advisor, and an Accept error, ends the step with no line queued.
func (r *seatRun) watch(outputs []string, sender *noticeSender, ender *stepEnder) {
	defer close(r.done)
	result, err := r.handle.Wait()
	r.mu.Lock()
	r.outcome, r.terminal, r.waitErr = result.Outcome, result, err
	r.mu.Unlock()

	finished := err == nil && result.Outcome == shuttleengine.OutcomeDone
	if r.seat.Required {
		if finished && (result.Gate == nil || result.Gate.Passed) && r.accept(result) {
			return
		}
		ender.end(r)
		return
	}
	if finished {
		return
	}
	r.mu.Lock()
	r.notice = advisorNotice(r.name, outputs, result.Outcome)
	notice := r.notice
	r.mu.Unlock()
	sender.queue(notice)
}

// accept calls the seat's Accept hook with result and records its error; it reports whether the result was accepted.
func (r *seatRun) accept(result shuttleengine.Result) bool {
	if r.seat.Accept == nil {
		return true
	}
	err := r.seat.Accept(result)
	r.mu.Lock()
	r.acceptErr = err
	r.mu.Unlock()
	return err == nil
}

// ended reports whether the advisor's watch goroutine has returned.
func (r *seatRun) ended() bool {
	select {
	case <-r.done:
		return true
	default:
		return false
	}
}

// markEnded records that this advisor's end ended the step.
func (r *seatRun) markEnded() {
	r.mu.Lock()
	r.endedStep = true
	r.mu.Unlock()
}

// result snapshots the advisor's result; sender tells whether its notice landed.
func (r *seatRun) result(sender *noticeSender) SeatResult {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := SeatResult{
		Name: r.seat.Name, StartError: r.startError, Outcome: r.outcome,
		Result: r.terminal, WaitErr: r.waitErr, AcceptErr: r.acceptErr, EndedStep: r.endedStep,
	}
	if r.handle != nil {
		result.StrandGUID = r.handle.StrandGUID()
		result.RunDir = r.handle.RunDir()
	}
	result.Notified = r.notice != "" && sender.wasSent(r.notice)
	return result
}

// absolute returns paths with each relative one joined onto the worktree root, the way shuttle resolves a spec's outputs.
func (e *Engine) absolute(paths []string) []string {
	resolved := make([]string, len(paths))
	for i, path := range paths {
		resolved[i] = path
		if !filepath.IsAbs(path) {
			resolved[i] = filepath.Join(e.geom.WorktreeRoot, path)
		}
	}
	return resolved
}

// Run starts table's seats fresh and returns once the chair has ended.
// It starts the chair first, then the advisors in table order, and tells the chair of each advisor once its start ends: up, or never started.
// A chair that fails to start starts no advisor and returns the error, which wraps shuttleengine.ErrNotStarted when the provider never came up.
// An advisor that fails to start is recorded and the step goes on without it.
// A required advisor that fails to start, or whose run later ends without being accepted, ends the step: Run starts no later advisor, stops the chair and every started advisor, and returns with that advisor's EndedStep set and no line queued to the chair.
// An advisor whose misnamed strand could not be stopped is still live, so Run stops the chair and every started advisor and returns the error.
// The result carries the chair's outcome, gate and NotStarted as shuttle reported them; the engine judges nothing about finishing itself.
// A seat that cannot be stopped is an error wrapping ErrSeatNotStopped.
func (e *Engine) Run(table Table) (Result, error) {
	if err := table.Validate(e.geom.StencilsDir); err != nil {
		return Result{}, err
	}
	names, err := strandNames(e.geom, table)
	if err != nil {
		return Result{}, err
	}
	chairOutputs := e.absolute(table.Chair().Outputs)
	sender := newNoticeSender(chairOutputs, e.noticeRetry)
	work := &advisorWork{}
	work.bindSender(sender)
	chair, err := e.startSeat(table, table.Chair(), names, work)
	if err != nil {
		return Result{ChairOutputs: chairOutputs}, err
	}
	sender.start(chair)

	ender := newStepEnder()
	var advisors []*seatRun
	for _, seat := range table.Seats {
		if seat.Name == leadName {
			continue
		}
		if ender.fired() {
			break
		}
		name := names[seat.Name]
		handle, err := e.startSeat(table, seat, names, work)
		if errors.Is(err, ErrSeatNotStopped) {
			sender.stop()
			stopErr := errors.Join(stopSeat(chair), stopAdvisors(advisors))
			return Result{ChairOutputs: chairOutputs, Advisors: snapshot(advisors, sender)}, errors.Join(err, stopErr)
		}
		if err != nil {
			logger.Warn("seatengine: advisor did not start", "seat", seat.Name, "error", err)
			failed := &seatRun{seat: seat, name: name, startError: err.Error()}
			advisors = append(advisors, failed)
			if seat.Required {
				ender.end(failed)
				break
			}
			failed.notice = advisorNotStartedNotice(name)
			sender.queue(failed.notice)
			continue
		}
		sender.queue(advisorUpNotice(name))
		started := newSeatRun(seat, name, handle, e.absolute(seat.Outputs), sender, ender)
		work.add(started)
		advisors = append(advisors, started)
	}
	return e.join(table, chair, advisors, sender, ender)
}

// startSeat composes seat's prompt and spec, starts it, and checks it took the strand name its peers address it by.
// The chair starts with the table's gate and an advisor with its seat's own.
// The chair's spec defers its held-turn notice while work reports advisor work outstanding; an advisor's spec defers nothing.
func (e *Engine) startSeat(table Table, seat Seat, names map[string]string, work *advisorWork) (Handle, error) {
	values, err := seatValues(e.geom, table, seat, names)
	if err != nil {
		return nil, err
	}
	prompt, err := composeSeatPrompt(e.geom.StencilsDir, seat, values, table.Optional)
	if err != nil {
		return nil, err
	}
	spec := seatSpec(table, seat, amendLeadPrompt(table, seat, prompt))
	if seat.Name == leadName {
		spec.HoldDeferral = work.outstanding
	}
	handle, err := e.shuttle.StartGated(spec, seatGate(table, seat))
	if err != nil {
		return nil, fmt.Errorf("seatengine: seat %q: start: %w", seat.Name, err)
	}
	if err := checkName(handle, names[seat.Name]); err != nil {
		if stopErr := stopSeat(handle); stopErr != nil {
			return nil, errors.Join(err, stopErr)
		}
		return nil, err
	}
	return handle, nil
}

// checkName refuses a started seat whose strand is not called want.
// Reed numbers a name it already holds, so a different name means a strand of an earlier step still holds want.
func checkName(handle Handle, want string) error {
	got := handle.StrandName()
	if got == want {
		return nil
	}
	return fmt.Errorf("seatengine: strand %q started as %q, because another strand holds that name; way forward: run \"lyx reed remove --name %s\", then re-step the row", want, got, want)
}

// stopSeat stops handle through its Stop, which records the stop before it removes the strand.
// A failed stop returns an error wrapping ErrSeatNotStopped whose message ends with the way forward, and is never retried.
func stopSeat(handle Handle) error {
	guid := handle.StrandGUID()
	if err := handle.Stop(); err != nil {
		return fmt.Errorf("%w: strand %s: %v; way forward: run \"lyx reed remove %s\", then re-step the row", ErrSeatNotStopped, guid, err, guid)
	}
	return nil
}

// stopAdvisors stops every started advisor and waits for each stopped one's goroutine to end.
// An advisor whose stop failed is still live, so it is not waited for.
func stopAdvisors(advisors []*seatRun) error {
	var stopErr error
	for _, advisor := range advisors {
		if advisor.handle == nil {
			continue
		}
		if err := stopSeat(advisor.handle); err != nil {
			stopErr = errors.Join(stopErr, err)
			continue
		}
		<-advisor.done
	}
	return stopErr
}

// snapshot returns every advisor's result, in table order.
func snapshot(advisors []*seatRun, sender *noticeSender) []SeatResult {
	results := make([]SeatResult, 0, len(advisors))
	for _, advisor := range advisors {
		results = append(results, advisor.result(sender))
	}
	return results
}

// join waits on the chair, stops the notice sender, then stops every started advisor, a kept done one included.
// A chair whose wait errored carries no terminal outcome and may still be running, and a chair that timed out keeps its strand live, so each is stopped too.
// A required advisor whose end ended the step before the chair's makes join stop the sender and the chair, wait for the chair's result, and mark that advisor EndedStep; a chair that also ended by then decides the step as usual.
// join returns the chair's result, the wait error and any stop error; a stop that failed is not waited for.
func (e *Engine) join(table Table, chair Handle, advisors []*seatRun, sender *noticeSender, ender *stepEnder) (Result, error) {
	type chairEnd struct {
		result shuttleengine.Result
		err    error
	}
	chairDone := make(chan chairEnd, 1)
	go func() {
		result, err := chair.Wait()
		chairDone <- chairEnd{result, err}
	}()

	var end chairEnd
	var stopErr error
	stepEnded := false
	select {
	case end = <-chairDone:
	case <-ender.signal:
		select {
		case end = <-chairDone:
		default:
			stepEnded = true
			sender.stop()
			ender.advisor.markEnded()
			if stopErr = stopSeat(chair); stopErr == nil {
				end = <-chairDone
			}
		}
	}
	chairResult, waitErr := end.result, end.err
	sender.stop()

	if (waitErr != nil || chairResult.Outcome == shuttleengine.OutcomeTimeout) && !stepEnded {
		stopErr = stopSeat(chair)
	}
	stopErr = errors.Join(stopErr, stopAdvisors(advisors))

	result := Result{
		Chair:        chairResult,
		ChairOutputs: e.absolute(table.Chair().Outputs),
		Advisors:     snapshot(advisors, sender),
	}
	logger.Info("seatengine: step joined", "chairOutcome", chairResult.Outcome, "advisors", len(advisors))

	if waitErr != nil {
		waitErr = fmt.Errorf("seatengine: chair wait: %w", waitErr)
	}
	return result, errors.Join(waitErr, stopErr)
}
