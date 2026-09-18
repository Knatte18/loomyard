// watchdog.go implements the per-hub watchdog daemon: the `lyx reed watchdog` verb, its discovery
// loop, and the pure seams that make discovery and the idle-exit rule unit-testable without a live
// tmux server.
//
// internal/reedcli owns the daemon outright, per the discussion's
// told-geometry-keeps-the-daemon-out-of-reedengine decision: CONSTRAINTS.md's Told-Geometry
// Invariant bars internal/reedengine from importing internal/lyxcwd, and reedcli already holds the
// *lyxcwd.Location and already imports internal/hubgeom, so it may import internal/fabricengine
// directly as hubgeom does. internal/reedengine gains exactly one new engine-less function
// (ListSessions) and learns nothing about the daemon's existence.

package reedcli

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/reedengine"
)

// watchdogHubDiscoveryCycle is how often the daemon's outer loop runs one list-sessions round trip
// against its hub socket.
//
// It lives here, beside the discovery loop that consumes it, rather than alongside
// internal/reedengine/watchdog.go's existing unexported watchdog* constants: those govern
// Engine.Watch's own internals, which reedengine still owns, and exporting two constants from that
// package purely so another package could read them would put the timings somewhere their only
// consumer is not.
//
// Five seconds is well below any human `down` + `up` gap while being an order of magnitude slower
// than the 100ms signal tick a per-worktree Engine.Watch already polls at, so discovery costs one
// list-sessions round trip per hub every five seconds rather than riding the per-worktree tick.
const watchdogHubDiscoveryCycle = 5 * time.Second

// watchdogHubIdleCycles is how many consecutive idle discovery cycles (see sessionsAreIdle) the
// daemon tolerates before exiting.
//
// Three cycles covers a `down` immediately followed by an `up` without the daemon dying and
// respawning in between.
const watchdogHubIdleCycles = 3

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
// it, plus the cancel for the goroutine (if any) running Engine.Watch on its behalf.
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
	// CONSTRAINTS.md's Live-Substrate Spawn Observability rule.
	logger.Debug("reed: watchdog resolving worktree for session", "hub", hub, "session", sessionName)
	return lyxcwd.ResolveWorktree(worktreeRoot)
}

// enterSession builds the watchedSession entry for a newly-appeared live tmux session name,
// starting its Engine.Watch goroutine unless the worktree's own config says watchdog: off.
//
// LoadConfig, not a strict load, is used deliberately: a worktree with no _lyx still resolves the
// embedded template, which keeps reedengine on the degrading side of the Config Strictness
// Invariant rather than refusing a worktree the daemon should otherwise watch.
//
// A watchdog: off worktree still gets an entry, with a nil cancel and no goroutine started at all:
// watchLoop answers a disabled watchdog by blocking on <-ctx.Done() rather than returning, so
// starting one here would cost a goroutine per disabled worktree to do nothing — while keeping the
// entry still keeps the session known and keeps departure bookkeeping (runWatchdogLoop) uniform
// across enabled and disabled worktrees.
func enterSession(hub, tmuxPath, sessionName string) (watchedSession, error) {
	location, err := resolveWatchedSession(hub, sessionName)
	if err != nil {
		return watchedSession{}, err
	}

	geom := hubgeom.ReedGeometry(location)
	cfg, err := reedengine.LoadConfig(location.AnchorPath(), "reed")
	if err != nil {
		return watchedSession{}, err
	}
	eng := reedengine.New(cfg, geom)

	// A worktree reaches the daemon's discovery loop only after "lyx reed up" already booted it,
	// and that boot already validated cfg.Watchdog loudly (ensureServerAndSessionLocked), so the
	// only value this needs to recognize here is the "off" spelling itself — every other value,
	// valid or not, behaves as enabled, matching Engine.Watch's own fail-safe-toward-watching
	// posture for a value it cannot parse.
	if strings.EqualFold(strings.TrimSpace(cfg.Watchdog), "off") {
		return watchedSession{eng: eng}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		if err := eng.Watch(ctx); err != nil {
			logger.Debug("reed: watchdog's watch loop returned", "session", sessionName, "err", err)
		}
	}()
	return watchedSession{eng: eng, cancel: cancel}, nil
}

// runWatchdogLoop is the daemon's outer loop: it polls hub for live sessions every
// watchdogHubDiscoveryCycle, enters newly-appeared sessions, tears down departed ones, and returns
// once watchdogHubIdleCycles consecutive cycles are idle (see sessionsAreIdle) or ctx is done.
//
// Teardown on departure is not optional: Engine.Watch never returns while its context is live, so
// without cancelling a departed entry's goroutine, a worktree whose session goes away while
// siblings remain would leave a goroutine polling a dead session for the daemon's whole remaining
// lifetime. Cancelling on departure is also what makes "re-entry re-reads config" true at all:
// watchLoop reads cfg.Watchdog exactly once at start, so a flipped watchdog: value only takes
// effect once the entry leaves (this departure teardown) and re-enters (enterSession, on the next
// appearance) — there is no other re-read path.
func runWatchdogLoop(ctx context.Context, hub, tmuxPath string) error {
	logger.Info("reed: watchdog daemon starting", "hub", hub)

	known := make(map[string]watchedSession)
	defer func() {
		for name, ws := range known {
			if ws.cancel != nil {
				ws.cancel()
			}
			logger.Debug("reed: watchdog stopped watching session on daemon exit", "hub", hub, "session", name)
		}
	}()

	ticker := time.NewTicker(watchdogHubDiscoveryCycle)
	defer ticker.Stop()

	idleCycles := 0
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}

		live, err := reedengine.ListSessions(tmuxPath, reedengine.ServerName(hub))
		if sessionsAreIdle(live, err) {
			idleCycles++
			if idleCycles >= watchdogHubIdleCycles {
				logger.Info("reed: watchdog daemon exiting after consecutive idle discovery cycles", "hub", hub, "cycles", idleCycles)
				return nil
			}
			continue
		}
		idleCycles = 0

		appeared, departed := planSessionDiff(live, known)
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
