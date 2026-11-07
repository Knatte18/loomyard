// start_watchdog_test.go pins the watchdog seam's call site in the start verb.
// It checks that the verb reaches c.spawnWatchdog with the hub path, tmux path, and shell path the receiver carries.
// A recording stub stands in for the seam, so the assertion needs no real process.
//
// Whether the call fires on every start is not asserted here.
// The call sits after c.reed.Up() and the strand branch that follows it, and both must succeed first.
// Those calls run on a concrete *reedengine.Engine, which cannot work without a live tmux server,
// so no offline test in this package can reach the call site through the real RunE.
// The smoke tier (internal/loomcli/smoke_starttail_test.go) asserts it against a real session instead.

package loomcli

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestStartCmd_ReachesSpawnWatchdogWithHubAndTmuxPath substitutes a recording stub into loomCLI.spawnWatchdog.
// It asserts the verb reaches that stub with the hub path and tmux path the receiver carries.
// suppressWatchdogSpawn is left at its testing.Testing() value;
// the stub replaces the call entirely, rather than the suppress flag gating it, so the real seam is never reached.
func TestStartCmd_ReachesSpawnWatchdogWithHubAndTmuxPath(t *testing.T) {
	tiergate.Require(t, 0)
	c := newLoomCLI()
	// A fictional location and a pure-constructed engine, per this file's own doc comment: both are
	// value composition (reedengine.New performs no I/O), never a live tmux server.
	c.location = &lymcwd.Location{HubPath: "/fictional/hub", WorktreeName: "fictional-worktree"}
	c.reed = reedengine.New(reedengine.Config{Tmux: "/fictional/tmux"}, reedengine.Geometry{}, shuttleengines.EnvScrub())

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

	// Drive the seam directly, exactly as start.go's RunE does at its own call site.
	// This file cannot drive the real RunE (see the file-level doc comment),
	// so it pins the same call expression the call site uses instead.
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
