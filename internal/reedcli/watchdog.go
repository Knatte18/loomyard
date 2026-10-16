// watchdog.go implements the per-hub watchdog daemon: the `lyx reed watchdog` verb, its discovery
// loop, and the pure seams that make discovery and the idle-exit rule unit-testable without a live
// tmux server.
//
// internal/reedcli still owns the daemon's verb, its discovery loop, and its idle-exit rule, per
// the discussion's told-geometry-keeps-the-daemon-out-of-reedengine decision:
// PATTERN-told-geometry bars internal/reedengine from importing internal/lyxcwd, and reedcli
// already holds the *lyxcwd.Location and already imports internal/hubgeom, so it may import
// internal/fabricengine directly as hubgeom does. That decision was revisited, not dropped, for one
// piece of the daemon alone: the detached spawn itself moved to internal/reedengine
// (SpawnWatchdog, spawnwatchdog.go) so internal/loomcli could reach it over an already-shipped
// import edge without depending on another module's cobra layer — see the
// watchdog-seam-lives-in-reedengine decision. reedengine's SpawnWatchdog takes hubPath and tmuxPath
// as plain parameters rather than resolving them itself, so it learns nothing about the daemon's
// discovery loop, its idle-exit rule, or its lock file, all of which remain here.

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
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
	"github.com/spf13/cobra"
)

// watchdogHubDiscoveryCycle is the base cadence of the daemon's outer loop: how often it runs one list-sessions round trip against its hub socket while the session set keeps changing.
// Across cycles that find the set unchanged the cadence doubles up to watchdogHubDiscoveryCeiling, and a changed set or a discover signal returns it to this base.
//
// It lives here, beside the discovery loop that consumes it, because that loop is its only consumer.
//
// Five seconds is well below any human `down` + `up` gap, so discovery costs one list-sessions round trip per hub every five seconds at most.
const watchdogHubDiscoveryCycle = 5 * time.Second

// watchdogHubDiscoveryCeiling is the longest wait the discovery loop backs off to across unchanged cycles.
const watchdogHubDiscoveryCeiling = 60 * time.Second

// watchdogHubIdleCycles is how many consecutive idle discovery cycles (see sessionsAreIdle) the
// daemon tolerates before exiting.
//
// Three cycles covers a `down` followed by an `up` for a daemon whose watch failed to open and so never sees the stop,
// and a transient list-sessions error, which sessionsAreIdle counts as idle.
const watchdogHubIdleCycles = 3

// watchdogStopFloor is the shortest wait between a cycle and the next one a stop request wakes.
// It is the one-second floor of PATTERN-daemon-wakeups, so a writer touching the stop in a loop costs one cycle per second.
const watchdogStopFloor = time.Second

// watchdogOrphanGoneCycles is how many consecutive **affirmative** discovery cycles a session name
// must be observed gone (see worktreeRootGone) before the daemon reaps it — roughly 15s at the
// existing 5s watchdogHubDiscoveryCycle.
//
// The reap this guards is destructive and unattended, and a worktree directory can be legitimately
// absent for a moment: a `git worktree move`, an editor or backup tool swapping a directory, a
// filesystem remount. None of those may cost a session its live agent work. Three is the same figure
// and the same reasoning watchdogHubIdleCycles already uses for the daemon's own exit, so this file
// carries one cadence idiom rather than two.
const watchdogOrphanGoneCycles = 3

// watchdogTiming carries the daemon's loop tunables as data, so a test can drive the loop against
// compressed cycles while production wires the fixed package constants through
// watchdogDefaultTiming.
//
// DiscoveryCycle is the base cadence and DiscoveryCeiling the cap it backs off to across unchanged cycles; a ceiling equal to the cycle means no backoff.
// StopFloor is the shortest wait between a cycle and the next one a stop request wakes.
// ReapTimeout bounds the reap's graceful wait before it force-kills.
// Zero selects reedengine's own 15s reap budget, which is what production runs.
type watchdogTiming struct {
	DiscoveryCycle   time.Duration
	DiscoveryCeiling time.Duration
	StopFloor        time.Duration
	IdleCycles       int
	OrphanGoneCycles int
	ReapTimeout      time.Duration
}

// watchdogDefaultTiming returns the daemon's production timings, sourced from the package's fixed watchdog* constants and nothing else.
// ReapTimeout stays zero, selecting reedengine's reap budget.
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
// A consumed discover signal runs a cycle sooner, but never sooner than the base after the last one, so a process touching the signal in a loop costs one cycle per base.
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

// afterCycle records a cycle that ran at now and moves the cadence: back to the base when the session set changed, otherwise doubled up to the ceiling.
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
// A pending signal's cycle is due at the later of the signal and the base after the last cycle, or the stop floor after it when a stop is pending; otherwise the cycle is due when the cadence ends.
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
// It reports whether the file was there, and stop when its trimmed content is reedengine.DiscoverStopMarker.
// An absent file answers false, false with no error; an empty file, which a cold boot's touch leaves in place of a pending stop, answers true, false.
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
// It returns false only when err is nil and names is non-empty — an affirmative listing. Every
// other combination is idle: an exit-0 empty listing, a "no server running" error, and any other
// list-sessions failure alike.
//
// This totality is forced rather than merely tolerated (Shared Decision
// daemon-idle-rule-is-anything-but-an-affirmative-listing): the normal last-`down` case is an
// ERROR, not an empty list, so a rule counting only exit-0-empty would leave the daemon's own main
// exit path undefined. And internal/reedengine/proctree_windows.go records that psmux exits
// identically with and without a server, so a rule distinguishing a no-server error from a
// transient one would be unimplementable there.
func sessionsAreIdle(names []string, err error) bool {
	return !(err == nil && len(names) > 0)
}

// watchedSession is one worktree session the daemon has entered: the *reedengine.Engine built for
// it, plus the cancel for the name-repair goroutine (if any) running on its behalf.
//
// cancel is nil for a `watchdog: off` worktree: enterSession starts no goroutine for it at all, but
// the entry is still kept so departure bookkeeping (runWatchdogLoop) stays uniform across enabled
// and disabled worktrees.
type watchedSession struct {
	eng    *reedengine.Engine
	cancel context.CancelFunc
}

// planSessionDiff is the daemon's TDD discovery seam: it compares one discovery cycle's live
// session names against the daemon's own known set and reports which names newly appeared and
// which known names are no longer live.
//
// It is pure — no tmux, no filesystem — so it is unit-testable with no fixture at all.
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

// planReapCycle is the daemon's pure decision seam for one whole cycle's reap bookkeeping. It is
// told the cycle's inputs rather than discovering them, and it is the only place the three
// bookkeeping rules below interact: it performs no filesystem access of any kind — gone-ness is an
// input, produced by worktreeRootGone outside it — and it mutates counters in place while returning
// the two name lists the caller needs: reap (names to dispatch a reap for this cycle) and remaining
// (names that survive into planSessionDiff).
//
// "Three consecutive cycles" means three consecutive *affirmative* cycles: a cycle whose listing was
// non-affirmative never reaches this function at all, so it advances, resets and prunes nothing.
//
// Behaviour, in this order:
//
//  1. When hubLive is false, short-circuit: return a nil reap and a remaining holding every name in
//     live except those in inFlight, no file read needed, with every entry in counters untouched —
//     neither advanced, reset, nor pruned. This is the hub probe's refusal to act, and the in-flight
//     exclusion holds on this branch too: a hub outage must not become the one path on which
//     planSessionDiff sees a name whose reap goroutine is still running.
//  2. Otherwise, prune counters: delete every key that is not present in live. A name that left the
//     live list starts from zero when it returns.
//  3. Then walk live in order. A name in inFlight is skipped entirely — it appears in neither return
//     value. For a name whose gone entry is true, increment counters[name]; if the incremented value
//     is at or above threshold, append it to reap and delete(counters, name), otherwise append it to
//     remaining. For a name whose gone entry is false or absent, delete(counters, name) and append it
//     to remaining.
//
// Deleting the counter at dispatch rather than leaving it at threshold is the discussion's
// counter-resets-after-a-reap decision: kill-session teardown is asynchronous, so a reaped name can
// still appear in the next cycle's listing, and a still-listed name must re-confirm across three more
// affirmative cycles before a second kill is issued.
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

// resolveWatchedSession resolves a live tmux session name back to the *lyxcwd.Location of the
// worktree it belongs to, by a direct join rather than a scan.
//
// The join is exact, not a candidate set, because hub-mode reedengine.SessionName(worktreeRoot) is
// filepath.Base(worktreeRoot) verbatim, and validateToldTmuxIdentity refuses an unusable name
// instead of sanitizing it. With hub being filepath.Dir(worktreeRoot), filepath.Join(hub,
// sessionName) recovers the worktree root exactly. lyxcwd.ResolveWorktree is still required on top
// of the join: hubgeom.ReedGeometry needs a resolved *lyxcwd.Location, and ResolveWorktree is also
// the gate that rejects a session name that does not name a worktree at all.
func resolveWatchedSession(hub, sessionName string) (*lyxcwd.Location, error) {
	worktreeRoot := filepath.Join(hub, sessionName)
	// The direct join is a git spawn inside a polling probe, so it is logged at Debug per
	// PATTERN-spawn-observability.
	logger.Debug("reed: watchdog resolving worktree for session", "hub", hub, "session", sessionName)
	return lyxcwd.ResolveWorktree(worktreeRoot)
}

// enterSession builds the watchedSession entry for a newly-appeared live tmux session name,
// starting its name-repair goroutine unless the worktree's own config says watchdog: off.
//
// LoadConfig, not a strict load, is used deliberately: a worktree with no _lyx still resolves the
// embedded template, which keeps reedengine on the degrading side of the Config Strictness
// Invariant rather than refusing a worktree the daemon should otherwise watch.
//
// A watchdog: off worktree still gets an entry, with a nil cancel and no goroutine started at all:
// the entry keeps the session known and keeps departure bookkeeping (runWatchdogLoop) uniform
// across enabled and disabled worktrees.
//
// The engine drives tmuxPath, the binary the daemon was told, whatever the worktree's own config names,
// so a --tmux shim counts every subcommand of every watcher.
// That told binary is the tmux value of the worktree whose spawn started the daemon, and the watchers ignore every other worktree's tmux value, a debugging wrapper included.
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

	// A worktree reaches the daemon's discovery loop only after "lyx reed up" already booted it,
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
// of sessionName on done: `select { case done <- sessionName: default: }`. The send must be
// non-blocking because nothing drains done once the loop is out of its for — a blocking or
// unbuffered send here would deadlock this goroutine against the deferred wg.Wait() the loop runs
// at daemon exit. A dropped send is inert: the loop is already gone, and with it the in-flight set
// the send would have cleared.
//
// A failed reap does nothing beyond the Warn below: there is no retry loop, no backoff, and no
// escalation. Recovery is the ordinary loop — the gone-counter for sessionName was deleted at
// dispatch, so a still-live orphan re-confirms across three more affirmative cycles and is reaped
// again.
//
// This goroutine never touches the in-flight map, the gone-counter map, or the known map: all
// three stay single-threaded on the loop goroutine, and reapDone is the only channel between them.
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

// runWatchdogLoop is the daemon's outer loop: it polls hub for live sessions, enters newly-appeared sessions, tears down departed ones, reaps orphaned
// ones, and returns when one of these holds:
//
//   - a cycle woken by a stop request lists no live session: it returns nil at once, without waiting for timing.IdleCycles;
//   - timing.IdleCycles consecutive cycles are idle (see sessionsAreIdle): it returns nil;
//   - replaced answers true: it returns errWatchdogBinaryReplaced when that cycle's listing is affirmative, and nil when it is idle, since a successor is only for a live hub;
//   - ctx is done: it returns ctx.Err().
//
// list is the session lister; it is called once per cycle with tmuxPath and the hub's server name.
//
// The wait between cycles starts at timing.DiscoveryCycle, doubles up to timing.DiscoveryCeiling across cycles that find the session set unchanged, and returns to the base when the set changes (discoveryWake).
// The loop also waits on the hub-level discover signal file, opened through open: a consumed signal runs a cycle at once, never sooner than the base after the previous one.
// A signal file holding reedengine.DiscoverStopMarker is a stop request, whose cycle runs no sooner than timing.StopFloor after the previous one.
// When open fails the loop logs that once at Warn and keeps its backed-off cadence with no event wake, and never sees a stop.
//
// shellPath sits immediately after tmuxPath because the two are told together and travel together —
// both are told-not-derived values the reap pass needs. An empty shellPath is logged once at Warn,
// here at loop start naming hub, because the reap still runs but kills each orphaned session's pane
// root pids without their descendants on Windows — the degradation the discussion's
// the-daemon-is-told-its-shell decision chose over refusing to start.
//
// One cycle's body runs, after the existing sessionsAreIdle/idleCycles handling and its continue, in
// this exact order:
//
//  1. Drain reapDone non-blockingly, clearing finished names from the in-flight set before anything
//     below reads it.
//  2. Probe hubLive := hubIsLiveDir(hub).
//  3. Build the per-name gone map when hubLive is true — the exact worktree-root join
//     (filepath.Join(hub, name)), not a scan, since hub-mode reedengine.SessionName(worktreeRoot) is
//     filepath.Base(worktreeRoot) verbatim — or pass a nil map when hubLive is false, since
//     planReapCycle never reads it on that branch.
//  4. Call planReapCycle for this cycle's reap/remaining split.
//  5. For each name planReapCycle selected for a reap: cancel its known entry (stopping the
//     goroutine polling a session about to be killed), remove it from known, mark it in-flight, log
//     the reap at Warn naming hub and session, and dispatchReap it off-loop.
//  6. Call planSessionDiff(remaining, known) rather than planSessionDiff(live, known) — remaining
//     already excludes both this cycle's reaps and every name still in-flight from an earlier cycle,
//     which is load-bearing: a reap dispatched last cycle whose session tmux still lists would
//     otherwise read as *appeared*, and the daemon would start watching a session it is in the
//     middle of killing.
//
// The idle counter keeps its existing meaning: a cycle whose listing was affirmative resets
// idleCycles to zero even when every session in it was reaped, because the hub socket did answer
// with sessions — a hub going genuinely quiet afterwards is observed by the following cycles
// through the existing path.
//
// The reap itself runs off-loop, in the goroutine dispatchReap starts, rather than inline in this loop:
// reapPaneChildren waits up to timing.ReapTimeout (default 15s) and then up to forceKillExitGrace (5s) per straggler,
// so an inline reap would stall one tick for ~20s — breaking the confirmation rule's quoted cadence, delaying entry into a newly-appeared healthy session, and leaving ctx.Done() unread for the whole stall.
//
// Teardown on departure is not optional: the name-repair loop never returns while its context is live, so
// without cancelling a departed entry's goroutine, a worktree whose session goes away while
// siblings remain would leave a goroutine polling a dead session for the daemon's whole remaining
// lifetime. Cancelling on departure is also what makes "re-entry re-reads config" true at all:
// enterSession reads cfg.Watchdog exactly once, so a flipped watchdog: value only takes
// effect once the entry leaves (this departure teardown) and re-enters (enterSession, on the next
// appearance) — there is no other re-read path.
//
// replaced is called once on each timer tick, after the session listing, and a true answer returns through the deferred teardown above.
// A nil replaced is never called.
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
		// The daemon never returns mid-reap and never force-kills its own reaper: wg.Wait() runs
		// only after every known entry's cancel has already been issued above.
		wg.Wait()
	}()

	signalDir := fabricengine.HubScratchDir(hub)
	signalPath := filepath.Join(signalDir, reedengine.DiscoverSignalFileName)
	// A failed open leaves events nil, a channel that never delivers.
	events, closeWatch, err := open(signalDir, reedengine.DiscoverSignalFileName)
	if err != nil {
		logger.Warn("reed: watchdog could not watch the discover signal file, keeping its backed-off cadence", "hub", hub, "err", err)
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
		case _, ok := <-events:
			if !ok {
				events = nil
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
			// Cancelling the entry first is what stops a goroutine from polling a session the
			// daemon is killing.
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
				// A name that does not resolve to a worktree is skipped, not retried until it next
				// re-appears in the live set: resolveWatchedSession's own ResolveWorktree call is
				// what actually failed, so there is nothing more to learn by retrying immediately.
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

// errWatchdogBinaryReplaced is what runWatchdogLoop returns when its replaced check answers true, so the verb retires the daemon and respawns it.
var errWatchdogBinaryReplaced = errors.New("reed: the watchdog's binary was replaced")

// exeReplaced reports whether the binary now at the daemon's path is no longer the one it started from.
// A nil start (no baseline) or a non-nil statErr answers false, so a binary missing mid-build never retires the daemon.
// Otherwise a now that is not the same file as start, or whose modification time differs from start's, answers true.
func exeReplaced(start, now os.FileInfo, statErr error) bool {
	if start == nil || statErr != nil {
		return false
	}
	return !os.SameFile(start, now) || !now.ModTime().Equal(start.ModTime())
}

// exeWatch stats once through stat to record the daemon's baseline binary and returns the check the discovery loop calls on each timer tick.
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

// ownExecutableStat returns a stat of the path this process started from, resolved once now.
// The path is not re-resolved per stat: on Linux a replaced binary's /proc/self/exe link names a deleted file, so re-resolving would stat a path that no longer exists.
func ownExecutableStat() func() (os.FileInfo, error) {
	path, pathErr := os.Executable()
	return func() (os.FileInfo, error) {
		if pathErr != nil {
			return nil, pathErr
		}
		return os.Stat(path)
	}
}

// validateWatchdogFlags is watchdogCmd's pure pre-flight: it reports the same two refusals its
// RunE performs inline today, lifted here so they are testable without a cobra invocation.
//
// It takes exactly two parameters and has no shell parameter at all. That absence is the structural
// guarantee behind the discussion's the-daemon-is-told-its-shell decision: --shell is accepted on
// every GOOS but never validated, and having no shell parameter to inspect makes that a property of
// the signature rather than a branch someone can add later. A hard --shell pre-flight would have a
// real consequence: ensureWatchdogSpawned is best-effort and detached with no stderr anyone watches, so a
// rejection there would silently cost the hub its entire watchdog daemon, name repair for every
// worktree included, over one empty config value.
func validateWatchdogFlags(hubPath, tmuxPath string) error {
	if hubPath == "" || !filepath.IsAbs(hubPath) {
		return errors.New("--hub-path must be an absolute, non-empty path")
	}
	if tmuxPath == "" {
		return errors.New("--tmux must not be empty")
	}
	return nil
}

// worktreeRootGone reports whether path is **proven** gone: a not-exist stat error, or a stat that
// succeeds against something other than a directory. Every other outcome — a successful stat of a
// directory, or any other stat error (EACCES, EIO, a network-filesystem hiccup) — answers false.
//
// This restates validateToldWorktreeRootLive's own only-proven-gone contract at the one layer that
// cannot call it: treating an unreadable path as gone would let a momentary permission or I/O blip
// destroy a session full of live work.
func worktreeRootGone(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return errors.Is(err, fs.ErrNotExist)
	}
	return !info.IsDir()
}

// hubIsLiveDir reports whether hub is **proven** live: a stat that succeeds and reports a directory.
// Every error — proven-gone or merely unreadable alike — answers false.
//
// It is written as its own predicate rather than as the negation of worktreeRootGone, per the
// overview's two-distinct-stat-predicates-not-one-negated decision: an EACCES must make both false,
// and a negation would make one of them true.
func hubIsLiveDir(hub string) bool {
	info, err := os.Stat(hub)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// watchdogLockFileName is the daemon's single-instance lock file's name inside
// fabricengine.HubScratchDir(hub).
const watchdogLockFileName = "reed-watchdog.lock"

// watchdogCmd builds the `watchdog` subcommand: a blocking, single-instance, per-hub daemon that
// runs runWatchdogLoop until it idles out or its context is cancelled.
//
// Everything fallible runs pre-flight, on the envelope, before the command blocks:
// validateWatchdogFlags' refusals (an absent or non-absolute --hub-path, an empty --tmux) each
// report through output.Err, and lock contention (another daemon already holds the lock) exits 0
// rather than erroring, since a racing spawn costing one short-lived process is the expected,
// harmless outcome.
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
directory, and the foreground run honours -v, -vv, LYX_LOG_LEVEL and
LYX_LOG_FILE like every other verb.

The daemon retires when the lyx binary it was started from is replaced,
checked on each discovery tick, and respawns only while the hub has a session,
so a deploy never leaves it laying out sessions with stale render code. The
last reed down asks the daemon to stop; it exits at its next cycle once no
session is left. A daemon started before this check
existed carries none and is ended once by hand after the deploy; the next
reed up, attach or resume spawns a successor that has it.

A detached daemon's stderr reaches nothing, so its Debug trace reaches only
the durable log directory. Two routes give a Debug trace of the daemon: run
it in the foreground with -vv, whose Debug lines then go to stderr or to the
LYX_LOG_FILE set for that run; or, while no daemon runs for the hub, run
lyx reed up, attach or resume with LYX_LOG_LEVEL=debug set, so the daemon
that verb spawns starts at Debug and writes its trace to the hub logs
directory. A daemon already running keeps the level it started with, since a
new spawn finds the hub lock held and exits.

Example:
  lyx reed watchdog --hub-path /abs/path/to/hub --tmux /usr/bin/tmux --shell /bin/bash`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			if err := validateWatchdogFlags(hubPath, tmuxPath); err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			// The durable sink is pointed FIRST, before the lock: its cwd-anchored
			// fallback arms only inside a lyx-owned worktree and never from a hub cwd, which is
			// exactly where cmd.Dir pins this process when it is spawned detached — without this
			// explicit call, every diagnostic the design leans on would go nowhere.
			logger.SetDurableSinkDir(fabricengine.HubLogsDir(hubPath))

			lockPath := filepath.Join(fabricengine.HubScratchDir(hubPath), watchdogLockFileName)
			fl, acquired, err := lock.TryAcquireWriteLock(lockPath)
			if err != nil {
				// A non-nil error is a FAILURE, not contention: the lock path itself is unusable
				// (an absent parent directory, permissions, a read-only filesystem), which must
				// never fail silently.
				logger.Error("reed: watchdog could not acquire its lock", "hub", hubPath, "lock", lockPath, "err", err)
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			if !acquired {
				// Contention: another daemon already holds the lock. This costs one short-lived
				// process and nothing else.
				// It logs at Debug so a lost race at the default level writes nothing and creates no trace file.
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
			err = runWatchdogLoop(cmd.Context(), hubPath, tmuxPath, shellPath, timing, reedengine.OpenFileWatch, list, exeWatch(exeStat))
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
			// The lock is released before the re-check so a successor spawned for a session that appeared during the exit wins the lock race.
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
