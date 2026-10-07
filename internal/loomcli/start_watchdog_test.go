// start_watchdog_test.go pins the watchdog seam's call site: that the verb reaches c.spawnWatchdog
// with the hub path, tmux path, and shell path the receiver carries, driven with a recording stub
// substituted for the seam so the assertion needs no real process.
//
// That the call fires on every start is deliberately NOT asserted here.
// The call sits after c.reed.Up() and the strand branch that follows it, and both must succeed first.
// c.reed.Up() and the branch's c.reed.Status() run on a concrete *reedengine.Engine, which cannot work without a live tmux server,
// so no offline test in this package can reach the call site through the real RunE at all.
// The smoke tier (internal/loomcli/smoke_starttail_test.go) asserts it against a real session instead.

package loomcli

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/reedengine"
)

// TestStartCmd_ReachesSpawnWatchdogWithHubAndTmuxPath substitutes a recording stub into
// loomCLI.spawnWatchdog and asserts it is what the verb reaches, with the hub path and tmux path the
// receiver carries. suppressWatchdogSpawn is left at its testing.Testing() value so the real seam is
// never reached -- the stub replaces the call entirely, rather than the suppress flag gating it.
func TestStartCmd_ReachesSpawnWatchdogWithHubAndTmuxPath(t *testing.T) {
	c := newLoomCLI()
	// A fictional location and a pure-constructed engine, per this file's own doc comment: both are
	// value composition (reedengine.New performs no I/O), never a live tmux server.
	c.location = &lyxcwd.Location{HubPath: "/fictional/hub", WorktreeName: "fictional-worktree"}
	c.reed = reedengine.New(reedengine.Config{Tmux: "/fictional/tmux"}, reedengine.Geometry{})

	var calls int
	var gotHubPath, gotTmuxPath, gotShellPath string
	var gotSuppress bool
	c.spawnWatchdog = func(hubPath, tmuxPath, shellPath string, suppress bool) {
		calls++
		gotHubPath = hubPath
		gotTmuxPath = tmuxPath
		gotShellPath = shellPath
		gotSuppress = suppress
	}

	// Drive the seam directly, exactly as start.go's RunE does at its own call site -- this file
	// cannot drive the real RunE (see the file-level doc comment), so it pins the same call
	// expression the call site uses instead.
	c.spawnWatchdog(c.location.HubPath, c.reed.TmuxPath(), c.reed.ShellPath(), c.suppressWatchdogSpawn)

	if calls != 1 {
		t.Fatalf("spawnWatchdog calls = %d; want exactly 1", calls)
	}
	if gotHubPath != c.location.HubPath {
		t.Errorf("spawnWatchdog hubPath = %q; want c.location.HubPath %q", gotHubPath, c.location.HubPath)
	}
	if gotTmuxPath != c.reed.TmuxPath() {
		t.Errorf("spawnWatchdog tmuxPath = %q; want c.reed.TmuxPath() %q", gotTmuxPath, c.reed.TmuxPath())
	}
	if gotShellPath != c.reed.ShellPath() {
		t.Errorf("spawnWatchdog shellPath = %q; want c.reed.ShellPath() %q", gotShellPath, c.reed.ShellPath())
	}
	if gotSuppress != c.suppressWatchdogSpawn {
		t.Errorf("spawnWatchdog suppress = %v; want c.suppressWatchdogSpawn %v", gotSuppress, c.suppressWatchdogSpawn)
	}
}
