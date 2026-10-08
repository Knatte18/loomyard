// send.go holds what every verified send does before it types: the context it runs in and the wait for an idle session.
// The wait only delays a send or fails it as busy; it never ends, classifies or finalizes a run.

package shuttleengine

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
)

// ErrSessionBusy marks a verified send that found the session still busy when its idle wait ran out: a turn running, a draft in the input box or an unmatched turn start.
// Nothing was typed, so the caller may retry the send later.
var ErrSessionBusy = errors.New("shuttle: the session is not idle")

const (
	// defaultSendReadyTimeoutS is the template.yaml default idle wait in seconds.
	defaultSendReadyTimeoutS = 60

	// idlePollInitial is the first interval between idle polls; the interval doubles up to idlePollMax.
	idlePollInitial = 250 * time.Millisecond
	idlePollMax     = 2 * time.Second

	// turnStartIdleOverride is how long a pane must read idle on every poll before an unmatched turn start stops holding a send.
	turnStartIdleOverride = 10 * time.Second

	// paneTailLines is how many non-blank pane lines a send failure ends with.
	paneTailLines = 15
)

// sendContext is what one verified send needs: the pane it types into, the run's events file, the config and the clock its timed waits read.
// A zero deadline means none.
type sendContext struct {
	reed       ReedOps
	engine     Engine
	guid       string
	eventsPath string
	cfg        Config
	clock      Clock
	deadline   time.Time
}

// newSendContext returns the context of a verified send into run's own pane.
func (run *Run) newSendContext() sendContext {
	return sendContext{
		reed:       run.runner.reed,
		engine:     run.runner.engine,
		guid:       run.state.StrandGUID,
		eventsPath: run.state.EventsPath,
		cfg:        run.runner.cfg,
		clock:      run.clock,
	}
}

// newSendContext returns the context of a verified send into the pane of the run state names.
func (r *Runner) newSendContext(state RunState) sendContext {
	return sendContext{
		reed:       r.reed,
		engine:     r.engine,
		guid:       state.StrandGUID,
		eventsPath: state.EventsPath,
		cfg:        r.cfg,
		clock:      r.clock,
	}
}

// sendReadyTimeout returns how long a verified send waits for an idle session.
// A non-positive value in a hand-built Config is floored to the template default.
func sendReadyTimeout(cfg Config) time.Duration {
	if cfg.SendReadyTimeoutS <= 0 {
		return defaultSendReadyTimeoutS * time.Second
	}
	return time.Duration(cfg.SendReadyTimeoutS) * time.Second
}

// awaitIdleSession returns nil once the strand's session is idle in fact, and fails with an error wrapping ErrSessionBusy when it stays busy past the send-ready window or sc's deadline.
// An engine without the SessionCycler idle reading keeps requireReadyAgentPane alone, with no wait.
// Otherwise the pane must classify ready and idle.
// For an engine that parses session signals, no turn start may be left unmatched by a later turn end either, unless the pane has read idle for turnStartIdleOverride or the engine reports that turn interrupted.
func awaitIdleSession(sc sendContext) error {
	cycler, ok := sc.engine.(SessionCycler)
	if !ok {
		return requireReadyAgentPane(sc.reed, sc.engine, sc.guid)
	}
	if err := requireLiveStrand(sc.reed, sc.guid); err != nil {
		return err
	}

	limit := sc.clock.Now().Add(sendReadyTimeout(sc.cfg))
	if !sc.deadline.IsZero() && sc.deadline.Before(limit) {
		limit = sc.deadline
	}
	var hold turnStartHold
	interval := idlePollInitial
	for {
		busy := busyReading(sc, cycler, &hold)
		if busy == "" {
			return nil
		}
		now := sc.clock.Now()
		if !now.Before(limit) {
			return fmt.Errorf("%w: strand %q: %s; retry once the session is idle. The pane's last lines:\n%s", ErrSessionBusy, sc.guid, busy, paneTail(sc.reed, sc.guid))
		}
		sc.clock.Sleep(min(interval, limit.Sub(now)))
		interval = min(interval*2, idlePollMax)
	}
}

// busyReading polls the pane and the events file once and returns what keeps the session from being idle, or "" when it is idle.
func busyReading(sc sendContext, cycler SessionCycler, hold *turnStartHold) string {
	capture, err := sc.reed.CapturePane(sc.guid)
	if err != nil {
		return fmt.Sprintf("the pane could not be captured: %v", err)
	}
	ready := sc.engine.Startup(capture) == StartupReady
	paneIdle := ready && cycler.IdleSession(capture)

	var unmatched string
	if turnStart, found := unmatchedTurnStart(sc.engine, sc.eventsPath); found && hold.holds(sc.engine, turnStart, paneIdle, sc.clock.Now()) {
		unmatched = fmt.Sprintf("a turn start at %s has no turn end", turnStart.At.Format(time.RFC3339))
	}
	switch {
	case !ready:
		return "the pane shows no input-ready provider"
	case !paneIdle:
		return "the pane is not idle"
	}
	return unmatched
}

// unmatchedTurnStart returns the newest turn_start in the events file that no later turn_end or api_error_turn_end follows.
// An engine without SessionSignalParser and an unreadable file read as none.
func unmatchedTurnStart(engine Engine, eventsPath string) (SessionSignal, bool) {
	parser, ok := engine.(SessionSignalParser)
	if !ok {
		return SessionSignal{}, false
	}
	data, err := os.ReadFile(eventsPath)
	if err != nil {
		return SessionSignal{}, false
	}
	signals, _ := parser.ParseSessionSignals(data)
	var pending SessionSignal
	found := false
	for _, signal := range signals {
		switch signal.Kind {
		case SessionSignalTurnStart:
			pending, found = signal, true
		case SessionSignalTurnEnd, SessionSignalAPIErrorTurnEnd:
			found = false
		}
	}
	return pending, found
}

// turnStartHold decides, poll by poll, whether an unmatched turn start still holds a send against a pane that reads idle.
// The zero value holds nothing yet.
type turnStartHold struct {
	signal    SessionSignal
	tracking  bool
	idleSince time.Time
}

// holds reports whether turnStart, the unmatched turn start, still holds the send at now.
// It releases once the engine's SessionProber reports that turn interrupted,
// or once paneIdle has been true on every poll since turnStartIdleOverride ago, logging one Warn naming the signal.
// A poll with a busy pane resets the idle timer.
func (h *turnStartHold) holds(engine Engine, turnStart SessionSignal, paneIdle bool, now time.Time) bool {
	if !h.tracking || !sameSignal(h.signal, turnStart) {
		h.signal, h.tracking, h.idleSince = turnStart, true, time.Time{}
	}
	if !paneIdle {
		h.idleSince = time.Time{}
		return true
	}
	if prober, ok := engine.(SessionProber); ok {
		if _, interrupted := prober.TurnStartInterrupt(turnStart); interrupted {
			return false
		}
	}
	if h.idleSince.IsZero() {
		h.idleSince = now
	}
	if now.Sub(h.idleSince) >= turnStartIdleOverride {
		logger.Warn("shuttle: send released an unmatched turn start held against an idle pane", "turnStartAt", turnStart.At, "sessionID", turnStart.SessionID, "idleFor", now.Sub(h.idleSince))
		return false
	}
	return true
}

// sameSignal reports whether a and b are the one hook line.
func sameSignal(a, b SessionSignal) bool {
	return a.Kind == b.Kind && a.At.Equal(b.At) && a.SessionID == b.SessionID && string(a.Raw) == string(b.Raw)
}

// paneTail returns the last paneTailLines non-blank lines of the strand's pane, or a sentence saying the capture failed.
func paneTail(reed ReedOps, guid string) string {
	capture, err := reed.CapturePane(guid)
	if err != nil {
		return fmt.Sprintf("(the final pane capture failed: %v)", err)
	}
	var lines []string
	for _, line := range strings.Split(capture, "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, strings.TrimRight(line, " \t\r"))
		}
	}
	if len(lines) > paneTailLines {
		lines = lines[len(lines)-paneTailLines:]
	}
	return strings.Join(lines, "\n")
}
