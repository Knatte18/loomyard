// strand.go reads a fixer strand's state from outside:
// whether reed still tracks it as live, and whether its newest turn has ended.

package gatefixer

import (
	"fmt"
	"os"
	"time"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// StrandLive reports whether guid names a strand reed currently tracks as live.
// A guid the status does not list reports (false, nil).
// Liveness comes only from a live Status() query, never from persisted reed state.
func StrandLive(reed shuttleengine.ReedOps, guid string) (bool, error) {
	status, err := reed.Status()
	if err != nil {
		return false, fmt.Errorf("gatefixer: reed status: %w", err)
	}
	for _, s := range status.Strands {
		if s.GUID == guid {
			return s.Live, nil
		}
	}
	return false, nil
}

// turnEndSettle is how long a strand's newest turn end must stand before it counts:
// a background shell that finishes just before a turn ends queues a completion notification,
// and that notification starts the strand's next turn an instant after the turn end.
const turnEndSettle = 5 * time.Second

// TurnEndRead carries the time a turn-end read is taken at, which decides whether a turn end has settled.
type TurnEndRead struct {
	Now time.Time
}

// TurnEndedAfter reports whether a strand's newest turn, read from the events at byte offset on, has ended.
// The offset skips the skill-load turns shuttle ran before the prompt; an offset past the file's end reads no events.
// With an engine that has the SessionSignalParser capability the newest turn start or turn end decides:
// a turn start, or no turn signal, is a strand still working;
// a turn end with nothing outstanding counts once it has stood for turnEndSettle;
// a just-finished shell's notification starts the next turn, so the turn end must stand first;
// a turn end left waiting on any task, a shell of either signal or a fork, never counts;
// that strand keeps running until its report exists or its own timeout.
// A turn end with no hook time counts at once.
// An engine without the capability counts the newest ParseEvents event only when it is an EventStop.
// ParseEvents errors propagate.
// A missing events file reports (false, nil).
func TurnEndedAfter(eventsPath string, offset int64, engine shuttleengine.Engine, read TurnEndRead) (bool, error) {
	data, err := os.ReadFile(eventsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("gatefixer: read events file %s: %w", eventsPath, err)
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
			return false, fmt.Errorf("gatefixer: parse events %s: %w", eventsPath, err)
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
	if len(newest.Outstanding) != 0 {
		return false, nil
	}
	return newest.At.IsZero() || read.Now.Sub(newest.At) >= turnEndSettle, nil
}
