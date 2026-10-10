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
// Notified reports whether the chair was told, from Go, that the advisor ended.
type SeatResult struct {
	Name       string
	StrandGUID string
	RunDir     string
	StartError string
	Outcome    shuttleengine.Outcome
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

// seatRun is one started advisor: its handle, and what its goroutine recorded when its wait ended.
type seatRun struct {
	seat   Seat
	name   string
	handle Handle
	done   chan struct{}

	mu         sync.Mutex
	startError string
	outcome    shuttleengine.Outcome
	notice     string
}

// newSeatRun returns the run of an advisor that started, and begins waiting on it.
// An advisor that ends without finishing queues one notice on sender.
func newSeatRun(seat Seat, name string, handle Handle, outputs []string, sender *noticeSender) *seatRun {
	run := &seatRun{seat: seat, name: name, handle: handle, done: make(chan struct{})}
	go run.watch(outputs, sender)
	return run
}

// watch waits on the advisor's run and records how it ended.
// An advisor that reached done queues nothing, since it stays reachable and the chair reads its file.
func (r *seatRun) watch(outputs []string, sender *noticeSender) {
	defer close(r.done)
	result, err := r.handle.Wait()
	r.mu.Lock()
	r.outcome = result.Outcome
	if err == nil && result.Outcome == shuttleengine.OutcomeDone {
		r.mu.Unlock()
		return
	}
	r.notice = advisorNotice(r.name, outputs, result.Outcome)
	notice := r.notice
	r.mu.Unlock()
	sender.queue(notice)
}

// result snapshots the advisor's result; sender tells whether its notice landed.
func (r *seatRun) result(sender *noticeSender) SeatResult {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := SeatResult{Name: r.seat.Name, StartError: r.startError, Outcome: r.outcome}
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
// It starts the advisors in table order and the chair last, so the chair's prompt can name an advisor that never started.
// An advisor that fails to start is recorded and the step goes on without it.
// An advisor whose misnamed strand could not be stopped is still live, so Run stops every started advisor and returns the error.
// A chair that fails to start stops every started advisor and returns the error, which wraps shuttleengine.ErrNotStarted when the provider never came up.
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
	chairSeat := table.Chair()
	chairOutputs := e.absolute(chairSeat.Outputs)
	sender := newNoticeSender(chairOutputs, e.noticeRetry)

	var advisors []*seatRun
	var failed []string
	for _, seat := range table.Seats {
		if seat.Name == RoleChair {
			continue
		}
		outputs := e.absolute(seat.Outputs)
		handle, err := e.startSeat(table, seat, names, nil)
		if errors.Is(err, ErrSeatNotStopped) {
			sender.stop()
			stopErr := stopAdvisors(advisors)
			return Result{ChairOutputs: chairOutputs, Advisors: snapshot(advisors, sender)}, errors.Join(err, stopErr)
		}
		if err != nil {
			logger.Warn("seatengine: advisor did not start", "seat", seat.Name, "error", err)
			failed = append(failed, names[seat.Name])
			advisors = append(advisors, &seatRun{seat: seat, name: names[seat.Name], startError: err.Error()})
			continue
		}
		advisors = append(advisors, newSeatRun(seat, names[seat.Name], handle, outputs, sender))
	}

	chair, err := e.startSeat(table, chairSeat, names, failed)
	if err != nil {
		sender.stop()
		stopErr := stopAdvisors(advisors)
		return Result{ChairOutputs: chairOutputs, Advisors: snapshot(advisors, sender)}, errors.Join(err, stopErr)
	}
	sender.start(chair)
	return e.join(table, chair, advisors, sender)
}

// startSeat composes seat's prompt and spec, starts it, and checks it took the strand name its peers address it by.
// The chair starts with the table's gate and an advisor ungated.
// failed lists the strand names of advisors that never started, for the chair's prompt.
func (e *Engine) startSeat(table Table, seat Seat, names map[string]string, failed []string) (Handle, error) {
	values, err := seatValues(e.geom, table, seat, names, failed)
	if err != nil {
		return nil, err
	}
	prompt, err := composeSeatPrompt(e.geom.StencilsDir, seat, values, table.Optional)
	if err != nil {
		return nil, err
	}
	var gate shuttleengine.GateSpec
	if seat.Name == RoleChair {
		gate = table.Gate
	}
	handle, err := e.shuttle.StartGated(seatSpec(table, seat, prompt), gate)
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
// A chair whose wait errored carries no terminal outcome and may still be running, so it is stopped too.
// join returns the chair's result, the wait error and any stop error; a stop that failed is not waited for.
func (e *Engine) join(table Table, chair Handle, advisors []*seatRun, sender *noticeSender) (Result, error) {
	chairResult, waitErr := chair.Wait()
	sender.stop()

	var stopErr error
	if waitErr != nil {
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
