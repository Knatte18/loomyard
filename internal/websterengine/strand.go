// strand.go implements webster's own strand/spawn seam helpers: StrandLive and TurnEndedAfter, the Starter seam, the OrchestratorStarter/OrchestratorHandle spawn seam, and the StrandStopper seam, inlining direct shuttleengine calls.
// These are deliberately module-local rather than shared, since the spawn seam is part of
// webster's own contract shape.
// The spawn-seam interfaces are exported so webstercli can assign a real *shuttleengine.Runner into
// them (Go's structural typing means *shuttleengine.Runner satisfies Starter and StrandStopper with no adapter glue).

package websterengine

import (
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// StrandLive reports whether guid names a strand reed currently tracks as live.
// guid absent from the result reports (false, nil).
// Liveness is NEVER read from persisted reed state;
// only a live Status() query can answer this.
func StrandLive(reed shuttleengine.ReedOps, guid string) (bool, error) {
	status, err := reed.Status()
	if err != nil {
		return false, fmt.Errorf("websterengine: reed status: %w", err)
	}
	for _, s := range status.Strands {
		if s.GUID == guid {
			return s.Live, nil
		}
	}
	return false, nil
}

// turnEndSettle is how long a strand's newest turn end must stand before it counts:
// a background shell that finished just before the turn ended queues its completion notification, which starts the strand's next turn an instant after the turn end.
const turnEndSettle = 5 * time.Second

// TurnEndRead carries what TurnEndedAfter's reading needs besides the events file: the read's time and the background-shell wait bound.
type TurnEndRead struct {
	Now       time.Time
	ShellWait time.Duration // shuttleengine.ShellWaitBound of the strand's shuttle config.
}

// TurnEndedAfter reports whether a strand's newest turn, read from the events at byte offset on, has ended with nothing it still waits on, so the turn end counts.
// The offset skips the skill-load turns shuttle ran before the prompt; an offset past the file's end reads no events.
// With an engine that has the SessionSignalParser capability the newest turn start or turn end decides:
// a turn start, or no turn signal, is a strand still working;
// a turn end with nothing outstanding counts once it has stood for turnEndSettle, since a just-finished shell's notification starts the next turn;
// a turn end left waiting on background work counts once read.ShellWait has passed when shuttleengine.ShellWaitExpires says the outstanding list expires, the bound Master's wait applies, and never otherwise, so a fork or a payload-reported shell keeps the strand running until its own timeout.
// A turn end with no hook time counts at once.
// An engine without the capability counts the newest ParseEvents event when it is an EventStop, and ParseEvents errors propagate.
// A missing events file reports (false, nil).
func TurnEndedAfter(eventsPath string, offset int64, engine shuttleengine.Engine, read TurnEndRead) (bool, error) {
	data, err := os.ReadFile(eventsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("websterengine: read events file %s: %w", eventsPath, err)
	}
	if offset > int64(len(data)) {
		offset = int64(len(data))
	}
	data = data[offset:]
	if len(data) == 0 {
		return false, nil
	}

	parser, ok := engine.(shuttleengine.SessionSignalParser)
	if !ok {
		events, err := engine.ParseEvents(data)
		if err != nil {
			return false, fmt.Errorf("websterengine: parse events %s: %w", eventsPath, err)
		}
		return len(events) > 0 && events[len(events)-1].Kind == shuttleengine.EventStop, nil
	}

	signals, _ := parser.ParseSessionSignals(data)
	var newest *shuttleengine.SessionSignal
	for i := range signals {
		switch signals[i].Kind {
		case shuttleengine.SessionSignalTurnStart, shuttleengine.SessionSignalTurnEnd:
			newest = &signals[i]
		}
	}
	if newest == nil || newest.Kind == shuttleengine.SessionSignalTurnStart {
		return false, nil
	}
	stood := func(d time.Duration) bool { return newest.At.IsZero() || read.Now.Sub(newest.At) >= d }
	if len(newest.Outstanding) == 0 {
		return stood(turnEndSettle), nil
	}
	return shuttleengine.ShellWaitExpires(newest.Outstanding, nil) && stood(read.ShellWait), nil
}

// Starter is the seam a batch's implementer or recovery strand spawns through.
// Start blocks until the spawned provider is past its startup gates (shuttle's guarantee) and
// returns an error when it never became ready; it never waits for the run to finish.
type Starter interface {
	Start(shuttleengine.Spec) (*shuttleengine.Run, error)
}

// OrchestratorHandle is the started-but-not-yet-finished orchestrator-role spawn a caller blocks
// on.
// StrandGUID identifies the reed strand, Wait blocks until terminal shuttle outcome.
type OrchestratorHandle interface {
	StrandGUID() string
	Wait() (shuttleengine.Result, error)
}

// OrchestratorStarter is the seam a caller spawns an orchestrator-role strand through, deliberately
// two-phase (start, then wait) so strand identity is learned and persisted before blocking.
type OrchestratorStarter interface {
	StartOrchestrator(shuttleengine.Spec) (OrchestratorHandle, error)
}

// RecoveryStrandRemoveError is RemoveRecoveryStrands's failure: the strand it could not remove and why.
type RecoveryStrandRemoveError struct {
	GUID string
	Err  error
}

func (e *RecoveryStrandRemoveError) Error() string {
	return fmt.Sprintf("could not remove recovery strand %s: %v", e.GUID, e.Err)
}

func (e *RecoveryStrandRemoveError) Unwrap() error { return e.Err }

// StrandStopper is the seam a leftover strand is stopped through by guid.
// StopStrand records the stop on the strand's run, then ends the strand when it is live, logging that kill at Warn, and is a no-op for a strand that is not.
type StrandStopper interface {
	StopStrand(guid string) error
}

// RemoveRecoveryStrands stops every live recovery strand the state records, in batch-number order.
// Only a batch of Kind "recovery" with a StrandGUID names one;
// implementer forks and the Master have no recorded strand.
// The first failure returns a *RecoveryStrandRemoveError naming the strand guid.
// A nil state stops nothing.
func RemoveRecoveryStrands(stopper StrandStopper, st *State) error {
	if st == nil {
		return nil
	}
	numbers := make([]int, 0, len(st.Batches))
	for number := range st.Batches {
		numbers = append(numbers, number)
	}
	sort.Ints(numbers)
	for _, number := range numbers {
		bs := st.Batches[number]
		if bs == nil || bs.Kind != "recovery" || bs.StrandGUID == "" {
			continue
		}
		if err := stopper.StopStrand(bs.StrandGUID); err != nil {
			return &RecoveryStrandRemoveError{GUID: bs.StrandGUID, Err: err}
		}
	}
	return nil
}
