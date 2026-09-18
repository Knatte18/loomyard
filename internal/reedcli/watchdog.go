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

import "time"

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
