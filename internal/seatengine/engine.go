// engine.go declares Engine, which starts a table's seats fresh, joins them and decides the step from the chair's result, and the result types it returns.

package seatengine

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// defaultNoticeRetry is how long the notice sender waits before retrying a line the chair's session could not take.
const defaultNoticeRetry = 5 * time.Second

// ErrSeatNotStopped marks a seat whose strand could not be stopped; its message ends with the way forward.
var ErrSeatNotStopped = errors.New("seatengine: a seat could not be stopped")

// SeatResult is what one seat's run came to, a seat after the lead.
// Name is the seat's name in the table.
// StrandGUID and RunDir are empty for a seat that never started, and StartError then holds why.
// Outcome is the seat's own terminal outcome, empty while it has none.
// Result is its terminal shuttle result, zero while it has none, and WaitErr is its wait's error.
// AcceptErr is the error its Accept hook returned.
// EndedStep is true for a required advisor or a writer whose end ended the step before the chair's.
// Notified reports whether the chair was told, from Go, that the seat ended or never started.
// Writer reports whether the seat is a writer, Outputs are the seat's absolute output paths, and GateFindingsPath is the seat's own, where the producer keeps its failed gate findings.
// Assistant reports whether the seat is an assistant, and EndMarker is an assistant's absolute end-marker path.
type SeatResult struct {
	Name             string
	StrandGUID       string
	RunDir           string
	StartError       string
	Outcome          shuttleengine.Outcome
	Result           shuttleengine.Result
	WaitErr          error
	AcceptErr        error
	EndedStep        bool
	Notified         bool
	Writer           bool
	Outputs          []string
	GateFindingsPath string
	Assistant        bool
	EndMarker        string
}

// Result is what one run of a table came to.
// Chair is the chair's result exactly as shuttle reported it, and ChairOutputs are the chair's output paths.
// Advisors holds one entry per seat after the lead, advisor, writer or assistant, in table order.
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
	noticeAfter func(time.Duration) <-chan time.Time
}

// New returns an Engine that starts and probes seats through shuttle and reads the paths geom tells it.
func New(shuttle Shuttle, geom Geometry) *Engine {
	return &Engine{shuttle: shuttle, geom: geom, noticeRetry: defaultNoticeRetry, noticeAfter: time.After}
}

// seatRun is one seat after the lead: its handle, its absolute outputs and an assistant's absolute end marker, and what its goroutine recorded when its wait ended, or why it never started.
type seatRun struct {
	seat      Seat
	name      string
	handle    Handle
	outputs   []string
	endMarker string
	done      chan struct{}

	mu         sync.Mutex
	startError string
	outcome    shuttleengine.Outcome
	terminal   shuttleengine.Result
	waitErr    error
	acceptErr  error
	endedStep  bool
	notice     string
	// stopping is set before the engine stops the seat, so an assistant's end marker reads stopped.
	stopping bool
}

// stepEnder records the first seat that ends the step, a required advisor or a writer, and wakes the join that waits on the chair.
type stepEnder struct {
	once    sync.Once
	signal  chan struct{}
	advisor *seatRun
}

func newStepEnder() *stepEnder {
	return &stepEnder{signal: make(chan struct{})}
}

// end records advisor, a seat that ends the step, as the one that ended it; only the first call counts.
func (s *stepEnder) end(advisor *seatRun) {
	s.once.Do(func() {
		s.advisor = advisor
		close(s.signal)
	})
}

// fired reports whether a seat that ends the step, a required advisor or a writer, has ended it.
func (s *stepEnder) fired() bool {
	select {
	case <-s.signal:
		return true
	default:
		return false
	}
}

// newSeatRun returns the run of a seat that started, with outputs as its absolute output paths and endMarker as an assistant's absolute end marker, and begins waiting on it.
// An advisor that ends without finishing queues one notice on sender, unless it is required, and a writer queues none; a required advisor or a writer ends the step on ender instead.
// An assistant writes its end marker when its run ends and queues nothing.
func newSeatRun(seat Seat, name string, handle Handle, outputs []string, endMarker string, sender *noticeSender, ender *stepEnder) *seatRun {
	run := &seatRun{seat: seat, name: name, handle: handle, outputs: outputs, endMarker: endMarker, done: make(chan struct{})}
	go run.watch(outputs, sender, ender)
	return run
}

// markStopping records that the engine is about to stop the seat.
func (r *seatRun) markStopping() {
	r.mu.Lock()
	r.stopping = true
	r.mu.Unlock()
}

// watch waits on the seat's run and records how it ended.
// An assistant writes its end marker, naming the ending, and queues nothing.
// An advisor that reached done queues nothing, since it stays reachable and the chair reads its file.
// A required advisor or a writer that ended done with its gate passed has its Accept hook called once with the terminal result; every other end of such a seat, and an Accept error, ends the step with no line queued.
// A writer that was accepted queues one line telling the chair its part stands and it keeps answering; a required advisor queues nothing.
func (r *seatRun) watch(outputs []string, sender *noticeSender, ender *stepEnder) {
	defer close(r.done)
	result, err := r.handle.Wait()
	r.mu.Lock()
	r.outcome, r.terminal, r.waitErr = result.Outcome, result, err
	stopping := r.stopping
	r.mu.Unlock()

	if r.seat.Assistant {
		_ = writeEnding(r.endMarker, endingOf(result, err, stopping))
		return
	}

	finished := err == nil && result.Outcome == shuttleengine.OutcomeDone
	if r.seat.endsStep() {
		if finished && (result.Gate == nil || result.Gate.Passed) && r.accept(result) {
			if r.seat.Writer {
				sender.queue(writerDoneNotice(r.name, outputs))
			}
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

// finishedDone reports whether the seat's wait ended without error on an outcome of done.
func (r *seatRun) finishedDone() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.waitErr == nil && r.outcome == shuttleengine.OutcomeDone
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

// markEnded records that this seat's end ended the step.
func (r *seatRun) markEnded() {
	r.mu.Lock()
	r.endedStep = true
	r.mu.Unlock()
}

// result snapshots the seat's result; sender tells whether its notice landed.
func (r *seatRun) result(sender *noticeSender) SeatResult {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := SeatResult{
		Name: r.seat.Name, StartError: r.startError, Outcome: r.outcome,
		Result: r.terminal, WaitErr: r.waitErr, AcceptErr: r.acceptErr, EndedStep: r.endedStep,
		Writer: r.seat.Writer, Outputs: r.outputs, GateFindingsPath: r.seat.GateFindingsPath,
		Assistant: r.seat.Assistant, EndMarker: r.endMarker,
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
	return absoluteUnder(e.geom.WorktreeRoot, paths)
}

// endMarkerOf returns seat's absolute end marker, empty for a seat that is no assistant.
func (e *Engine) endMarkerOf(seat Seat) string {
	if seat.EndMarker == "" {
		return ""
	}
	return e.absolute([]string{seat.EndMarker})[0]
}

// Run starts table's seats fresh and returns once the chair has ended.
// Before the chair starts it removes every assistant's stale end marker, outputs and subagent reports.
// It starts the chair first, then the other seats in table order, and tells the chair of each advisor once its start ends: up, or never started; a writer gets no up line, since its done line is what the chair waits for, and an assistant gets none, since the chair waits on its end marker.
// A chair that fails to start starts no other seat and returns the error, which wraps shuttleengine.ErrNotStarted when the provider never came up.
// An advisor that fails to start is recorded and the step goes on without it.
// An assistant that fails to start, or whose SkipReason is set, gets its end marker reading not started with the reason, and the step goes on without it.
// A required advisor or a writer that fails to start, or whose run later ends without being accepted, ends the step: Run starts no later seat, stops the chair and every started seat, and returns with that seat's EndedStep set and no line queued to the chair.
// An advisor whose misnamed strand could not be stopped is still live, so Run stops the chair and every started advisor and returns the error.
// The result carries the chair's outcome, gate and NotStarted as shuttle reported them; the engine judges nothing about finishing itself.
// A seat that cannot be stopped is an error wrapping ErrSeatNotStopped.
func (e *Engine) Run(table Table) (Result, error) {
	if err := table.Validate(e.geom.StencilsDir, e.geom.DefaultEngine); err != nil {
		return Result{}, err
	}
	names, err := strandNames(e.geom, table)
	if err != nil {
		return Result{}, err
	}
	if err := e.clearAssistantFiles(table); err != nil {
		return Result{}, err
	}
	chairOutputs := e.absolute(table.Chair().Outputs)
	sender := newNoticeSender(chairOutputs, e.noticeRetry, e.noticeAfter)
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
		if seat.SkipReason != "" {
			advisors = append(advisors, e.notStarted(seat, name, seat.SkipReason))
			continue
		}
		handle, err := e.startSeat(table, seat, names, work)
		if errors.Is(err, ErrSeatNotStopped) {
			sender.stop()
			stopErr := errors.Join(stopSeat(chair), stopAdvisors(advisors))
			return Result{ChairOutputs: chairOutputs, Advisors: snapshot(advisors, sender)}, errors.Join(err, stopErr)
		}
		if err != nil {
			logger.Warn("seatengine: advisor did not start", "seat", seat.Name, "error", err)
			if seat.Assistant {
				advisors = append(advisors, e.notStarted(seat, name, err.Error()))
				continue
			}
			failed := &seatRun{seat: seat, name: name, outputs: e.absolute(seat.Outputs), startError: err.Error()}
			advisors = append(advisors, failed)
			if seat.endsStep() {
				ender.end(failed)
				break
			}
			failed.notice = advisorNotStartedNotice(name)
			sender.queue(failed.notice)
			continue
		}
		if !seat.Writer && !seat.Assistant {
			sender.queue(advisorUpNotice(name))
		}
		started := newSeatRun(seat, name, handle, e.absolute(seat.Outputs), e.endMarkerOf(seat), sender, ender)
		work.add(started)
		advisors = append(advisors, started)
	}
	return e.join(table, chair, advisors, sender, ender)
}

// notStarted records an assistant that did not start for reason: its end marker reads not started with the reason, a Warn names it, and no line reaches the chair.
func (e *Engine) notStarted(seat Seat, name, reason string) *seatRun {
	logger.Warn("seatengine: assistant did not start", "seat", seat.Name, "reason", reason)
	run := &seatRun{seat: seat, name: name, outputs: e.absolute(seat.Outputs), endMarker: e.endMarkerOf(seat), startError: reason}
	_ = writeEnding(run.endMarker, notStartedEnding(reason))
	return run
}

// startSeat composes seat's prompt and spec, starts it, and checks it took the strand name its peers address it by.
// The chair starts with the table's gate and an advisor with its seat's own.
// The chair's spec defers its held-turn notice while work reports advisor work outstanding; an advisor's spec defers nothing.
func (e *Engine) startSeat(table Table, seat Seat, names map[string]string, work *advisorWork) (Handle, error) {
	values, err := seatValues(e.geom, table, seat, names)
	if err != nil {
		return nil, err
	}
	prompt, err := composeSeatPrompt(e.geom, table, seat, values)
	if err != nil {
		return nil, err
	}
	spec := seatSpec(table, seat, amendLeadPrompt(table, seat, prompt))
	if seat.Name == leadName {
		spec.HoldDeferral = work.outstanding
	}
	handle, err := e.shuttle.StartGated(spec, seatGate(table, seat))
	if err != nil {
		return nil, fmt.Errorf("seatengine: seat %q: start: %w", table.seatLabel(seat), seat.ModelKey.Wrap(err))
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
	return fmt.Errorf("seatengine: strand %q started as %q, because another strand holds that name; way forward: run \"lym reed remove --name %s\", then re-step the row", want, got, want)
}

// stopSeat stops handle through its Stop, which records the stop before it removes the strand.
// A failed stop returns an error wrapping ErrSeatNotStopped whose message ends with the way forward, and is never retried.
func stopSeat(handle Handle) error {
	guid := handle.StrandGUID()
	if err := handle.Stop(); err != nil {
		return fmt.Errorf("%w: strand %s: %v; way forward: run \"lym reed remove %s\", then re-step the row", ErrSeatNotStopped, guid, err, guid)
	}
	return nil
}

// stopAdvisors stops every started advisor, writer and assistant and waits for each stopped one's goroutine to end, so a stopped assistant's end marker reads stopped.
// An advisor whose stop failed is still live, so it is not waited for.
func stopAdvisors(advisors []*seatRun) error {
	var stopErr error
	for _, advisor := range advisors {
		if advisor.handle == nil {
			continue
		}
		advisor.markStopping()
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
// In a table that has writers, a writer whose end ended the step is marked EndedStep even when the chair's end arrives first or together with it: once the chair's end is received and before any seat is stopped, a fired step ender marks its writer.
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
		if table.hasWriters() && ender.fired() {
			ender.advisor.markEnded()
		}
	case <-ender.signal:
		select {
		case end = <-chairDone:
			if table.hasWriters() {
				ender.advisor.markEnded()
			}
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
