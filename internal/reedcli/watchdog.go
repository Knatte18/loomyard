// watchdog.go implements the per-hub watchdog daemon behind `lym reed watchdog`.
// It holds the daemon's discovery loop and idle-exit rule.
// Its pure seams are unit-testable without a live tmux server.

package reedcli

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
	"github.com/spf13/cobra"
)

// watchdogHubDiscoveryCycle is the base wait between the daemon's discovery cycles.
// Each cycle runs one list-sessions round trip against its hub socket.
// The wait doubles across cycles that find the session set unchanged, up to watchdogHubDiscoveryCeiling.
// A changed set or a discover signal returns it to this base.
//
// The base is well below any human `down` and `up` gap.
// Discovery therefore costs at most one list-sessions round trip per hub per cycle.
const watchdogHubDiscoveryCycle = 5 * time.Second

// watchdogHubDiscoveryCeiling is the longest wait the discovery loop backs off to across unchanged cycles.
const watchdogHubDiscoveryCeiling = 60 * time.Second

// watchdogHubIdleCycles is how many consecutive idle discovery cycles the daemon tolerates before it exits.
// A cycle is idle unless its list-sessions round trip succeeds and lists at least one session.
//
// Three cycles covers a `down` followed by an `up` whose boot touch replaced the stop before the daemon read it.
// It also covers a transient list-sessions error.
const watchdogHubIdleCycles = 3

// watchdogStopFloor is the shortest wait between a cycle and the next one that a stop request wakes.
// It keeps a writer that touches the stop in a loop to one cycle per second.
const watchdogStopFloor = time.Second

// watchdogOrphanGoneCycles is how many consecutive affirmative discovery cycles must observe a session name gone.
// The daemon reaps the session only after that many such cycles.
//
// The reap is destructive and unattended, and a worktree directory can be legitimately absent for a moment:
// a `git worktree move`, an editor or backup tool swapping a directory, or a filesystem remount.
// None of those may cost a session its live agent work.
const watchdogOrphanGoneCycles = 3

// watchdogTiming carries the daemon's loop tunables as data.
// A test drives the loop with compressed cycles.
// Production uses the fixed package constants through watchdogDefaultTiming.
//
// DiscoveryCycle is the base cadence, and DiscoveryCeiling is the cap it backs off to across unchanged cycles.
// A ceiling equal to the cycle means no backoff.
// StopFloor is the shortest wait between a cycle and the next one that a stop request wakes.
// ReapTimeout bounds the reap's graceful wait before it force-kills.
// Zero selects reedengine's reap budget, which production uses.
type watchdogTiming struct {
	DiscoveryCycle   time.Duration
	DiscoveryCeiling time.Duration
	StopFloor        time.Duration
	IdleCycles       int
	OrphanGoneCycles int
	ReapTimeout      time.Duration
}

// watchdogDefaultTiming returns the daemon's production timings, built from the fixed watchdog* constants.
// ReapTimeout stays zero, which selects reedengine's reap budget.
func watchdogDefaultTiming() watchdogTiming {
	return watchdogTiming{
		DiscoveryCycle:   watchdogHubDiscoveryCycle,
		DiscoveryCeiling: watchdogHubDiscoveryCeiling,
		StopFloor:        watchdogStopFloor,
		IdleCycles:       watchdogHubIdleCycles,
		OrphanGoneCycles: watchdogOrphanGoneCycles,
	}
}

// discoveryWake plans when the discovery loop runs its next cycle.
// It holds the last cycle's time and the current cadence, which backs off while cycles find the session set unchanged.
// A consumed discover signal runs a cycle sooner, but never sooner than the base after the last one.
// A process that touches the signal in a loop therefore costs one cycle per base.
// A signal that carried a stop request is held to the stop floor instead of the base.
type discoveryWake struct {
	base      time.Duration
	ceiling   time.Duration
	stopFloor time.Duration
	// cadence is the wait that follows the last cycle.
	cadence   time.Duration
	lastCycle time.Time
	// signalAt is when the pending signal was consumed; zero when none is pending.
	signalAt time.Time
	// stop is whether a pending signal carried a stop request.
	stop bool
}

// afterCycle records a cycle that ran at now and moves the cadence.
// It returns the cadence to the base when the session set changed, and otherwise doubles it up to the ceiling.
// It clears a pending signal and its stop request, which the cycle served.
func (d *discoveryWake) afterCycle(now time.Time, changed bool) {
	d.lastCycle = now
	d.cadence = reedengine.NextWakeCadence(d.cadence, d.base, d.ceiling, changed)
	d.signalAt = time.Time{}
	d.stop = false
}

// onSignal marks a discover signal consumed at now and returns the cadence to the base.
// A stop request is kept until the cycle runs, so a discover landing after a stop does not lose it.
func (d *discoveryWake) onSignal(now time.Time, stop bool) {
	d.signalAt = now
	d.cadence = d.base
	d.stop = d.stop || stop
}

// next returns how long to wait from now before the next cycle.
// With a pending signal, the cycle is due at the later of two times:
// the signal itself, or the last cycle plus the stop floor when a stop is pending and the base otherwise.
// Without a pending signal, the cycle is due when the cadence ends.
func (d *discoveryWake) next(now time.Time) time.Duration {
	due := d.lastCycle.Add(d.cadence)
	if !d.signalAt.IsZero() {
		floor := d.base
		if d.stop {
			floor = d.stopFloor
		}
		due = d.lastCycle.Add(floor)
		if d.signalAt.After(due) {
			due = d.signalAt
		}
	}
	return max(due.Sub(now), 0)
}

// sameSessionSet reports whether a and b list the same session names, order and repeats aside.
func sameSessionSet(a, b []string) bool {
	set := make(map[string]bool, len(a))
	for _, name := range a {
		set[name] = true
	}
	other := make(map[string]bool, len(b))
	for _, name := range b {
		if !set[name] {
			return false
		}
		other[name] = true
	}
	return len(set) == len(other)
}

// consumeDiscoverSignal reads and removes the discover signal file at path.
// It reports whether the file was there, and whether its trimmed content is reedengine.DiscoverStopMarker.
// An absent file answers false, false with no error.
// An empty file, which a cold boot's touch leaves in place of a pending stop, answers true, false.
// A read error other than not-exist, or a removal error after a successful read, is returned with both results false.
func consumeDiscoverSignal(path string) (consumed, stop bool, err error) {
	content, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	if err := os.Remove(path); err != nil {
		return false, false, err
	}
	return true, strings.TrimSpace(string(content)) == reedengine.DiscoverStopMarker, nil
}

// sessionsAreIdle reports whether one discovery cycle's list-sessions round trip counts toward the
// daemon's idle-exit counter.
//
// It returns false only when err is nil and names is non-empty, an affirmative listing.
// Every other combination is idle:
// an exit-0 empty listing, a "no server running" error, and any other list-sessions failure alike.
//
// This totality is forced rather than merely tolerated.
// The normal last-`down` case is an error, not an empty list,
// so a rule counting only exit-0 empty listings would leave the daemon's own main exit path undefined.
// internal/reedengine/proctree_windows.go records that tmux CLI probes exit identically with or without a server.
// A rule telling a no-server error from a transient one would therefore be unimplementable there.
func sessionsAreIdle(names []string, err error) bool {
	return !(err == nil && len(names) > 0)
}

// watchedSession is one worktree session the daemon has entered.
// It holds the *reedengine.Engine built for the session.
// Its cancel stops the name-repair goroutine running on its behalf, if any.
//
// cancel is nil for a `watchdog: off` worktree.
// enterSession starts no goroutine for such a worktree, but the entry is still kept:
// departure bookkeeping in runWatchdogLoop stays uniform across enabled and disabled worktrees.
type watchedSession struct {
	eng    *reedengine.Engine
	cancel context.CancelFunc
}

// planSessionDiff compares one discovery cycle's live session names against the daemon's known set.
// It reports which names newly appeared and which known names are no longer live.
//
// It is pure, with no tmux or filesystem access, so it needs no fixture to test.
func planSessionDiff(live []string, known map[string]watchedSession) (appeared, departed []string) {
	liveSet := make(map[string]bool, len(live))
	for _, name := range live {
		liveSet[name] = true
		if _, ok := known[name]; !ok {
			appeared = append(appeared, name)
		}
	}
	for name := range known {
		if !liveSet[name] {
			departed = append(departed, name)
		}
	}
	return appeared, departed
}

// planReapCycle decides one whole cycle's reap bookkeeping.
// It is the daemon's pure decision seam for that bookkeeping.
// The caller passes the cycle's inputs, and the three rules below interact only in this function.
//
// It performs no filesystem access; gone-ness is an input the caller computes.
// It mutates counters in place and returns two name lists:
// reap, the names to dispatch a reap for this cycle, and remaining, the names that go on in the cycle.
//
// "Three consecutive cycles" means three consecutive *affirmative* cycles.
// A cycle whose listing is non-affirmative never reaches this function, so it advances, resets and prunes nothing.
//
// Behaviour, in this order:
//
//  1. When hubLive is false, short-circuit: return a nil reap and a remaining holding every name in
//     live except those in inFlight.
//     No file is read, and every entry in counters stays untouched: neither advanced, reset, nor pruned.
//     This is the hub probe's refusal to act.
//     The in-flight exclusion holds on this branch too: a hub outage must not be the one path
//     on which the caller sees a name whose reap goroutine is still running.
//  2. Otherwise, prune counters: delete every key that is not present in live.
//     A name that left the live list starts from zero when it returns.
//  3. Then walk live in order.
//     A name in inFlight is skipped entirely: it appears in neither return value.
//     For a name whose gone entry is true, increment counters[name].
//     If the incremented value is at or above threshold, append it to reap and delete(counters, name);
//     otherwise, append it to remaining.
//     For a name whose gone entry is false or absent, delete(counters, name) and append it to remaining.
//
// The counter is deleted at dispatch, not left at threshold, because kill-session teardown is asynchronous.
// A reaped name can still appear in the next cycle's listing,
// and a still-listed name must re-confirm across three more affirmative cycles before a second kill is issued.
func planReapCycle(live []string, hubLive bool, gone map[string]bool, counters map[string]int, inFlight map[string]bool, threshold int) (reap []string, remaining []string) {
	if !hubLive {
		for _, name := range live {
			if inFlight[name] {
				continue
			}
			remaining = append(remaining, name)
		}
		return nil, remaining
	}

	liveSet := make(map[string]bool, len(live))
	for _, name := range live {
		liveSet[name] = true
	}
	for name := range counters {
		if !liveSet[name] {
			delete(counters, name)
		}
	}

	for _, name := range live {
		if inFlight[name] {
			continue
		}
		if gone[name] {
			counters[name]++
			if counters[name] >= threshold {
				reap = append(reap, name)
				delete(counters, name)
			} else {
				remaining = append(remaining, name)
			}
			continue
		}
		delete(counters, name)
		remaining = append(remaining, name)
	}
	return reap, remaining
}

// resolveWatchedSession resolves a live tmux session name back to its worktree's location.
// It does this by a direct join.
//
// The join is exact: in hub mode a session name is the base name of its worktree root, verbatim,
// and an unusable name is refused rather than sanitized.
// The hub is the directory of the worktree root, so joining the hub and the session name recovers the root exactly.
// Resolving the location is still required on top of the join.
// The geometry needs a resolved location.
// Resolving also rejects a session name that does not name a worktree at all.
func resolveWatchedSession(hub, sessionName string) (*lymcwd.Location, error) {
	worktreeRoot := filepath.Join(hub, sessionName)
	// The direct join is a git spawn inside a polling probe, so it is logged at Debug per
	// PATTERN-child-processes.
	logger.Debug("reed: watchdog resolving worktree for session", "hub", hub, "session", sessionName)
	return lymcwd.ResolveWorktree(worktreeRoot)
}

// enterSession builds the watchedSession entry for a newly-appeared live tmux session name,
// starting its name-repair goroutine unless the worktree's own config says watchdog: off.
//
// The config loads leniently on purpose: a worktree with no _lym still resolves the embedded template.
// The daemon therefore watches that worktree instead of refusing it.
//
// A watchdog: off worktree still gets an entry, with a nil cancel and no goroutine started at all.
// The entry keeps the session known and keeps departure bookkeeping uniform across enabled and disabled worktrees.
//
// The engine drives tmuxPath, the binary the daemon was told, whatever the worktree's own config names.
// So a --tmux shim counts every subcommand of every watcher.
// That binary is the tmux value of the worktree whose spawn started the daemon.
// The watchers ignore every other worktree's tmux value, including a debugging wrapper.
// Nothing compares the two, since a config value and a resolved path to one binary differ as text.
func enterSession(hub, tmuxPath, sessionName string) (watchedSession, error) {
	location, err := resolveWatchedSession(hub, sessionName)
	if err != nil {
		return watchedSession{}, err
	}

	geom, err := hubgeom.ReedGeometry(location)
	if err != nil {
		return watchedSession{}, err
	}
	cfg, err := reedengine.LoadConfig(fabricengine.ConfigDirs(location), "reed")
	if err != nil {
		return watchedSession{}, err
	}
	cfg.Tmux = tmuxPath
	eng := reedengine.New(cfg, geom, shuttleengines.EnvScrub())

	// A worktree reaches the daemon's discovery loop only after "lym reed up" already booted it,
	// and that boot already validated cfg.Watchdog loudly (ensureServerAndSessionLocked), so the
	// only value this needs to recognize here is the "off" spelling itself — every other value,
	// valid or not, behaves as enabled, failing safe toward watching for a value it cannot parse.
	if strings.EqualFold(strings.TrimSpace(cfg.Watchdog), "off") {
		return watchedSession{eng: eng}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		if err := eng.WatchNames(ctx, shuttleengines.SessionNamers()); err != nil {
			logger.Debug("reed: watchdog's name repair loop returned", "session", sessionName, "err", err)
		}
	}()
	return watchedSession{eng: eng, cancel: cancel}, nil
}

// dispatchReap starts the one off-loop goroutine that reaps sessionName, registering it with wg and
// returning immediately so the calling cycle is never blocked by a reap in progress.
//
// The goroutine's only cross-goroutine act, after the reap itself returns, is a non-blocking send
// of sessionName on done: `select { case done <- sessionName: default: }`.
// The send must not block, because nothing drains done once the loop has exited.
// A blocking or unbuffered send would deadlock this goroutine against the deferred wg.Wait at daemon exit.
// A dropped send is inert: the loop is already gone, and with it the in-flight set the send would have cleared.
//
// A failed reap is only logged, at Warn: there is no retry loop, no backoff, and no escalation.
// Recovery is the ordinary cycle loop: the gone counter for sessionName was deleted at dispatch,
// so a still-live orphan must re-confirm across three more affirmative cycles before it is reaped again.
//
// This goroutine never touches the in-flight map, the gone-counter map, or the known map.
// All three stay single-threaded on the loop goroutine, and reapDone is the only channel between them.
func dispatchReap(wg *sync.WaitGroup, done chan<- string, hub, tmuxPath, shellPath, sessionName string, reapTimeout time.Duration) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := reedengine.ReapSession(tmuxPath, shellPath, reedengine.ServerName(hub), sessionName, reapTimeout); err != nil {
			// A destructive unattended action's failure is what the operator needs in the hub's
			// durable log.
			logger.Warn("reed: watchdog could not reap orphaned session", "hub", hub, "session", sessionName, "err", err)
		}
		select {
		case done <- sessionName:
		default:
		}
	}()
}

// runWatchdogLoop is the daemon's outer loop.
// It polls hub for live sessions, enters newly appeared sessions, tears down departed ones, and reaps orphaned ones.
// It returns when one of these holds:
//
//   - a cycle woken by a stop request lists no live session: it returns nil at once, without waiting for timing.IdleCycles;
//   - timing.IdleCycles consecutive cycles are idle, meaning the listing failed or names no session: it returns nil;
//   - replaced answers true: when that cycle's listing is affirmative, it returns errWatchdogBinaryReplaced;
//     when that cycle is idle, it returns nil, since a successor is only for a live hub;
//   - ctx is done: it returns ctx.Err().
//
// list is the session lister; it is called once per cycle with tmuxPath and the hub's server name.
//
// The wait between cycles starts at timing.DiscoveryCycle.
// It doubles up to timing.DiscoveryCeiling across cycles that find the session set unchanged,
// and a changed session set returns it to the base.
// The loop also waits on the hub-level discover signal file, opened through open.
// A consumed signal runs a cycle at once, never sooner than the base after the previous one.
// A signal file holding reedengine.DiscoverStopMarker is a stop request;
// its cycle runs no sooner than timing.StopFloor after the previous one.
// When open fails, or the event channel closes, the loop logs that once at Warn and wakes on its timer alone.
// From then on each timer wake first consumes the signal file itself, so a stop request is still seen.
//
// tmuxPath and shellPath are told together, and the reap pass needs both.
// An empty shellPath is logged once at Warn when the loop starts, naming hub.
// The reap still runs, but on Windows it kills each orphaned session's pane root pids without their descendants.
//
// Each cycle that is not idle runs these steps in order:
//
//  1. Drain the finished reaps from the in-flight set without blocking.
//  2. Probe whether hub is a live directory.
//  3. When it is, check each listed name's worktree root under hub for being gone; when it is not, check none.
//  4. Split the listed names into those to reap and those remaining.
//  5. For each name to reap, cancel and remove its known entry.
//     That stops the goroutine polling a session about to be killed.
//     Mark the name in-flight, log the reap at Warn naming hub and session, and dispatch the reap off-loop.
//  6. Diff the remaining names, not the full listing, against the known entries, then enter each appeared session
//     and tear down each departed one.
//     The remaining names exclude this cycle's reaps and every name still in-flight from an earlier cycle.
//     That exclusion is load-bearing: a session under reap that tmux still lists would otherwise read as appeared,
//     and the daemon would start watching a session it is killing.
//
// A cycle whose listing is affirmative resets the idle count, even when every session in it was reaped:
// the hub socket did answer with sessions.
// A hub that goes quiet later is seen by the following cycles through the normal idle path.
//
// The reap runs off-loop because an inline reap would stall the loop for as long as it takes.
// That would stretch the confirmation cadence and delay entry into a newly appeared healthy session.
// It would also leave ctx.Done() unread for the whole stall.
//
// A departed session's entry is torn down because its name-repair loop never returns while its context is live;
// left running, its goroutine would keep polling a dead session for the rest of the daemon's life.
// Cancelling on departure also lets a changed watchdog value take effect:
// enterSession reads cfg.Watchdog once, so a flipped value applies only when the entry leaves and re-enters.
//
// replaced, when non-nil, is called once on each timer wake, after the session listing.
// A true answer returns through the deferred teardown.
// It adds no wakeup of its own and never runs on a discover-signal wake.
func runWatchdogLoop(ctx context.Context, hub, tmuxPath, shellPath string, timing watchdogTiming, open reedengine.FileWatchOpener, list func(tmuxPath, socketKey string) ([]string, error), replaced func() bool) error {
	logger.Info("reed: watchdog daemon starting", "hub", hub)
	if shellPath == "" {
		logger.Warn("reed: watchdog daemon starting with no shell path; the reap will kill pane root pids without their descendants on Windows", "hub", hub)
	}

	known := make(map[string]watchedSession)
	goneCounters := make(map[string]int)
	inFlight := make(map[string]bool)
	reapDone := make(chan string, 64)
	var wg sync.WaitGroup
	defer func() {
		for name, ws := range known {
			if ws.cancel != nil {
				ws.cancel()
			}
			logger.Debug("reed: watchdog stopped watching session on daemon exit", "hub", hub, "session", name)
		}
		// The daemon never returns mid-reap and never force-kills its own reaper:
		// wg.Wait() runs only after every known entry's cancel has already been issued above.
		wg.Wait()
	}()

	signalDir := fabricengine.HubScratchDir(hub)
	signalPath := filepath.Join(signalDir, reedengine.DiscoverSignalFileName)
	// A failed open leaves events nil, a channel that never delivers.
	// unwatched is whether no watch delivers the signal's events, so each timer wake reads the file itself.
	events, closeWatch, err := open(signalDir, reedengine.DiscoverSignalFileName)
	unwatched := err != nil
	var consumeFailure logger.Episode
	if err != nil {
		logger.Warn("reed: watchdog could not watch the discover signal file, reading it on its backed-off cadence instead", "hub", hub, "err", err)
	} else {
		defer closeWatch()
	}

	start := time.Now()
	wake := &discoveryWake{base: timing.DiscoveryCycle, ceiling: timing.DiscoveryCeiling, stopFloor: timing.StopFloor, cadence: timing.DiscoveryCycle, lastCycle: start}
	timer := time.NewTimer(wake.next(start))
	defer timer.Stop()

	var previousLive []string
	idleCycles := 0
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			if unwatched {
				consumed, stop, err := consumeDiscoverSignal(signalPath)
				if err != nil {
					consumeFailure.Fail("reed: watchdog could not consume the discover signal", "hub", hub, "err", err)
				} else {
					consumeFailure.Recover("reed: watchdog consumes the discover signal again", "hub", hub)
					if consumed {
						wake.onSignal(time.Now(), stop)
					}
				}
			}
		case _, ok := <-events:
			if !ok {
				events = nil
				unwatched = true
				logger.Warn("reed: watchdog's discover signal watch closed, reading the file on its backed-off cadence instead", "hub", hub)
				continue
			}
			consumed, stop, err := consumeDiscoverSignal(signalPath)
			if err != nil {
				logger.Warn("reed: watchdog could not consume the discover signal", "hub", hub, "err", err)
				continue
			}
			if consumed {
				now := time.Now()
				wake.onSignal(now, stop)
				timer.Reset(wake.next(now))
			}
			continue
		}

		stopWoken := wake.stop
		live, err := list(tmuxPath, reedengine.ServerName(hub))
		now := time.Now()
		wake.afterCycle(now, !sameSessionSet(live, previousLive))
		previousLive = live
		timer.Reset(wake.next(now))
		idle := sessionsAreIdle(live, err)
		if replaced != nil && replaced() {
			if !idle {
				return errWatchdogBinaryReplaced
			}
			logger.Info("reed: watchdog retires on a replaced binary with no successor of its own; the hub has no session", "hub", hub)
			return nil
		}
		if idle {
			if stopWoken {
				logger.Info("reed: watchdog daemon exiting on a stop request", "hub", hub)
				return nil
			}
			idleCycles++
			if idleCycles >= timing.IdleCycles {
				logger.Info("reed: watchdog daemon exiting after consecutive idle discovery cycles", "hub", hub, "cycles", idleCycles)
				return nil
			}
			continue
		}
		idleCycles = 0

	drainReapDone:
		for {
			select {
			case name := <-reapDone:
				delete(inFlight, name)
			default:
				break drainReapDone
			}
		}

		hubLive := hubIsLiveDir(hub)
		var gone map[string]bool
		if hubLive {
			gone = make(map[string]bool, len(live))
			for _, name := range live {
				gone[name] = worktreeRootGone(filepath.Join(hub, name))
			}
		}
		reap, remaining := planReapCycle(live, hubLive, gone, goneCounters, inFlight, timing.OrphanGoneCycles)
		for _, name := range reap {
			// Cancelling the entry first stops a goroutine from polling a session the daemon is killing.
			if ws, ok := known[name]; ok {
				if ws.cancel != nil {
					ws.cancel()
				}
				delete(known, name)
			}
			inFlight[name] = true
			logger.Warn("reed: watchdog reaping orphaned session", "hub", hub, "session", name)
			dispatchReap(&wg, reapDone, hub, tmuxPath, shellPath, name, timing.ReapTimeout)
		}

		appeared, departed := planSessionDiff(remaining, known)
		for _, name := range appeared {
			ws, err := enterSession(hub, tmuxPath, name)
			if err != nil {
				// A name that does not resolve to a worktree is skipped.
				// It is retried only when it next re-appears in the live set, since an immediate retry would fail the same way.
				logger.Debug("reed: watchdog could not enter session, skipping", "hub", hub, "session", name, "err", err)
				continue
			}
			known[name] = ws
			logger.Debug("reed: watchdog entered session", "hub", hub, "session", name)
		}
		for _, name := range departed {
			ws, ok := known[name]
			if !ok {
				continue
			}
			if ws.cancel != nil {
				ws.cancel()
			}
			delete(known, name)
			logger.Debug("reed: watchdog session departed", "hub", hub, "session", name)
		}
	}
}

// errWatchdogBinaryReplaced is returned when the watchdog finds its binary replaced while the hub still lists live sessions.
// The caller retires the daemon and respawns it.
var errWatchdogBinaryReplaced = errors.New("reed: the watchdog's binary was replaced")

// exeReplaced reports whether the binary at the daemon's path is no longer the one it started from.
// A nil start (no baseline) or a non-nil statErr reports false, so a binary missing mid-build never retires the daemon.
// Otherwise it reports true when now is not the same file as start, or when its modification time differs from start's.
func exeReplaced(start, now os.FileInfo, statErr error) bool {
	if start == nil || statErr != nil {
		return false
	}
	return !os.SameFile(start, now) || !now.ModTime().Equal(start.ModTime())
}

// exeWatch stats the binary once through stat to record its baseline,
// and it returns a check that reports whether the binary was replaced since.
// A failed stat, at startup or in the check, logs at Debug and counts as unchanged.
func exeWatch(stat func() (os.FileInfo, error)) func() bool {
	baseline, err := stat()
	if err != nil {
		logger.Debug("reed: could not stat the watchdog's binary at startup; it will not retire on a replaced binary", "err", err)
		baseline = nil
	}
	return func() bool {
		now, err := stat()
		if err != nil {
			logger.Debug("reed: could not stat the watchdog's binary; treating it as unchanged", "err", err)
		}
		return exeReplaced(baseline, now, err)
	}
}

// ownExecutableStat returns a stat function for the path this process started from, resolved once at the call.
// The path is not re-resolved per stat.
// On Linux, a replaced binary's /proc/self/exe link names a deleted file.
// Re-resolving it would stat a path that no longer exists.
func ownExecutableStat() func() (os.FileInfo, error) {
	path, pathErr := os.Executable()
	return func() (os.FileInfo, error) {
		if pathErr != nil {
			return nil, pathErr
		}
		return os.Stat(path)
	}
}

// validateWatchdogFlags returns an error when --hub-path is empty or not absolute, or when --tmux is empty.
// It takes no shell parameter, so --shell is accepted and never validated.
//
// A hard check on --shell would silently cost the hub its whole watchdog daemon:
// the spawn is best-effort and detached, with no stderr anyone watches.
// One empty config value would then take name repair away from every worktree.
func validateWatchdogFlags(hubPath, tmuxPath string) error {
	if hubPath == "" || !filepath.IsAbs(hubPath) {
		return errors.New("--hub-path must be an absolute, non-empty path")
	}
	if tmuxPath == "" {
		return errors.New("--tmux must not be empty")
	}
	return nil
}

// worktreeRootGone reports whether path is proven gone.
// That holds when a stat fails with a not-exist error, or succeeds on something other than a directory.
// Every other outcome reports false:
// a successful stat of a directory, or any other stat error such as EACCES, EIO or a network-filesystem hiccup.
//
// An unreadable path is not gone.
// Treating it as gone would let a momentary permission or I/O fault destroy a session full of live work.
func worktreeRootGone(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return errors.Is(err, fs.ErrNotExist)
	}
	return !info.IsDir()
}

// hubIsLiveDir reports whether hub is proven live: a stat succeeds and reports a directory.
// Every error reports false, so an unreadable path is neither live nor gone.
func hubIsLiveDir(hub string) bool {
	info, err := os.Stat(hub)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// watchdogLockFileName names the daemon's single-instance lock file.
// It lies in the directory that fabricengine.HubScratchDir returns for the hub.
const watchdogLockFileName = "reed-watchdog.lock"

// The bounds of the wait that `down` makes, after it ends the hub's server, for the watchdog to leave its loop.
// watchdogDownWait is the longest the wait lasts, and watchdogDownPoll is the pause between lock probes.
const (
	watchdogDownWait = 10 * time.Second
	watchdogDownPoll = 100 * time.Millisecond //lym:one-shot bounded lock poll of at most watchdogDownWait
)

// probeWatchdogLock reports whether the watchdog lock at lockPath is free.
// A probe that acquires the lock releases it at once and reports free;
// contention reports not free, and an unusable lock path is returned as the error.
func probeWatchdogLock(lockPath string) (free bool, err error) {
	held, acquired, err := lock.TryAcquireWriteLock(lockPath)
	if err != nil {
		return false, err
	}
	if !acquired {
		return false, nil
	}
	held.Release()
	return true, nil
}

// waitWatchdogLockFree waits for the watchdog lock of hub to come free.
// It probes through probe and pauses poll through sleep between probes, making at most wait/poll + 1 probes.
// A probe error means no scratch directory exists, so no daemon can hold the lock; it logs at Debug and returns.
// When the probes run out, it logs one Warn naming the hub and the lock path.
// The wait never fails its caller.
func waitWatchdogLockFree(hub string, wait, poll time.Duration, probe func(lockPath string) (bool, error), sleep func(time.Duration)) {
	lockPath := filepath.Join(fabricengine.HubScratchDir(hub), watchdogLockFileName)
	for probes := int(wait/poll) + 1; probes > 0; probes-- {
		free, err := probe(lockPath)
		if err != nil {
			logger.Debug("reed: down could not probe the watchdog lock, not waiting for the watchdog", "hub", hub, "lock", lockPath, "err", err)
			return
		}
		if free {
			return
		}
		if probes > 1 {
			sleep(poll)
		}
	}
	logger.Warn("reed: down gave up waiting for the watchdog to release its lock", "hub", hub, "lock", lockPath)
}

// watchdogCmd builds the `watchdog` subcommand, a blocking, single-instance, per-hub daemon.
// It runs the watchdog loop until it idles out or its context is cancelled.
//
// Every fallible step runs before the command blocks.
// An absent or non-absolute --hub-path, or an empty --tmux, reports through output.Err.
// Lock contention with another running daemon exits 0 rather than erroring:
// a racing spawn costing one short-lived process is the expected, harmless outcome.
func (c *reedCLI) watchdogCmd() *cobra.Command {
	var hubPath, tmuxPath, shellPath string

	cmd := &cobra.Command{
		Use:         "watchdog",
		Short:       "run the blocking, single-instance, per-hub watchdog daemon",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceInternal},
		Long: `watchdog is the detached, single-instance, per-hub daemon that hosts reed's
session-name repair for every worktree session on the hub named by
--hub-path. It is told its hub path, the tmux binary to use, and (optionally)
the shell it should spawn to walk a reaped session's descendant pids on its
command line — it opts out of reed's normal cwd/location/config resolution
entirely and must never derive any of them from its own environment.

up, resume and attach each attempt to spawn this daemon detached after
their own engine op returns without error; a spawn that finds the lock
already held exits 0 immediately. Running it directly in the foreground
is a real diagnosis path: its diagnostics land in the hub's durable log
directory, and the foreground run honours -v, -vv, LYM_LOG_LEVEL and
LYM_LOG_FILE like every other verb.

The daemon retires when the lym binary it was started from is replaced,
checked on each discovery tick, and respawns only while the hub has a session,
so a deploy never leaves it laying out sessions with stale render code. The
last reed down asks the daemon to stop; it exits at its next cycle once no
session is left. A daemon started before this check
existed carries none and is ended once by hand after the deploy; the next
reed up, attach or resume spawns a successor that has it.

A detached daemon's stderr reaches nothing, so its Debug trace reaches only
the durable log directory. Two routes give a Debug trace of the daemon: run
it in the foreground with -vv, whose Debug lines then go to stderr or to the
LYM_LOG_FILE set for that run; or, while no daemon runs for the hub, run
lym reed up, attach or resume with LYM_LOG_LEVEL=debug set, so the daemon
that verb spawns starts at Debug and writes its trace to the hub logs
directory. A daemon already running keeps the level it started with, since a
new spawn finds the hub lock held and exits.

Example:
  lym reed watchdog --hub-path /abs/path/to/hub --tmux /usr/bin/tmux --shell /bin/bash`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			if err := validateWatchdogFlags(hubPath, tmuxPath); err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			// Point the durable sink before taking the lock.
			// Its cwd-anchored fallback arms only inside a lym-owned worktree, never from a hub cwd,
			// and a detached spawn pins its cwd to the hub, so without this call every diagnostic would go nowhere.
			logger.SetDurableSinkDir(fabricengine.HubLogsDir(hubPath))

			lockPath := filepath.Join(fabricengine.HubScratchDir(hubPath), watchdogLockFileName)
			fl, acquired, err := lock.TryAcquireWriteLock(lockPath)
			if err != nil {
				// A non-nil error is a failure, not contention.
				// The lock path itself is unusable: an absent parent directory, bad permissions or a read-only filesystem.
				// That must never fail silently.
				logger.Error("reed: watchdog could not acquire its lock", "hub", hubPath, "lock", lockPath, "err", err)
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			if !acquired {
				// Contention: another daemon already holds the lock.
				// This costs one short-lived process and nothing else.
				// It logs at Debug, so a lost race at the default level writes nothing and creates no trace file.
				logger.Debug("reed: watchdog lock already held, exiting", "hub", hubPath, "lock", lockPath)
				return nil
			}
			defer fl.Release()

			timing := watchdogDefaultTiming()
			if c.watchdogTiming != nil {
				timing = *c.watchdogTiming
			}
			exeStat := c.watchdogExeStat
			if exeStat == nil {
				exeStat = ownExecutableStat()
			}
			list := c.watchdogListSessions
			if list == nil {
				list = reedengine.ListSessions
			}
			respawn := c.respawnWatchdog
			if respawn == nil {
				respawn = reedengine.SpawnWatchdog
			}
			openWatch := c.watchdogOpenWatch
			if openWatch == nil {
				openWatch = reedengine.OpenFileWatch
			}
			err = runWatchdogLoop(cmd.Context(), hubPath, tmuxPath, shellPath, timing, openWatch, list, exeWatch(exeStat))
			if errors.Is(err, errWatchdogBinaryReplaced) {
				// The successor's lock race is lost while this process still holds the lock, so release it first;
				// the deferred release that follows is a no-op on an unlocked flock.
				fl.Release()
				respawn(hubPath, tmuxPath, shellPath, testing.Testing())
				logger.Info("reed: watchdog retired; its binary was replaced and a successor was spawned", "hub", hubPath)
				return nil
			}
			if err != nil {
				logger.Warn("reed: watchdog daemon's loop returned", "hub", hubPath, "err", err)
				return nil
			}
			// The lock is released before the re-check,
			// so a successor spawned for a session that appeared during the exit wins the lock race.
			fl.Release()
			if cmd.Context().Err() != nil {
				return nil
			}
			names, listErr := list(tmuxPath, reedengine.ServerName(hubPath))
			if !sessionsAreIdle(names, listErr) {
				respawn(hubPath, tmuxPath, shellPath, testing.Testing())
				logger.Info("reed: watchdog exited and a successor was spawned for a live hub", "hub", hubPath)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&hubPath, "hub-path", "", "absolute path to the hub this daemon watches (required)")
	cmd.Flags().StringVar(&tmuxPath, "tmux", "", "path to the tmux binary this daemon uses (required)")
	cmd.Flags().StringVar(&shellPath, "shell", "", "shell this daemon spawns to walk a reaped session's descendant pids on Windows (accepted, never required)")

	return cmd
}
