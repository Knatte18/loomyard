// wiring_test.go drives wire directly against a hand-built *lyxcwd.Location over a temporary
// directory seeded with the one module config load that does not tolerate absence (loomengine's own
// strict configengine.Load): every other config load this task's wire performs
// (reedengine.LoadConfig, shuttleengine.LoadConfig, websterengine.LoadConfig, modelspec.LoadRegistry,
// batcher.Active) degrades to its embedded template on the same proven-present _lyx/ directory, so
// seeding loom.yaml alone is enough to drive the whole hub-only wire() without touching a real hub.

package loomcli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// seedLoomConfig creates <anchorPath>/_lyx/config/loom.yaml with the embedded template's contents --
// the one config load loomengine.LoadConfig performs strictly (see loomengine's own config_test.go).
func seedLoomConfig(t *testing.T, anchorPath string) {
	t.Helper()
	configDir := filepath.Join(anchorPath, "_lyx", "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", configDir, err)
	}
	cfgPath := filepath.Join(configDir, "loom.yaml")
	if err := os.WriteFile(cfgPath, []byte(loomengine.ConfigTemplate()), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", cfgPath, err)
	}
}

// hubLocation returns a *lyxcwd.Location standing in for a real hub location, with its anchor path
// seeded on disk with loom.yaml only.
func hubLocation(t *testing.T, worktreeName, anchorRel string) *lyxcwd.Location {
	t.Helper()
	hub := t.TempDir()
	loc := &lyxcwd.Location{HubPath: hub, WorktreeName: worktreeName, AnchorRel: anchorRel}
	seedLoomConfig(t, loc.AnchorPath())
	return loc
}

// TestWire_PathFieldsMatchLoomengineAccessors asserts every path field of the assembled
// shedrecipe.Env and loomrecipe.ShedPaths equals the corresponding loomengine accessor's own output
// for the same location.
//
// StatusPath and StatusLockPath are told twice -- once on c.env (read by loomPreflightEntry) and
// once on c.shedPaths (read by shedengine.Shed) -- so both copies are asserted against the same
// loomengine accessor, per the deliberate duplication ShedPaths' own doc comment describes.
func TestWire_PathFieldsMatchLoomengineAccessors(t *testing.T) {
	t.Parallel()

	loc := hubLocation(t, "warp", ".")
	cwd := loc.AnchorPath()

	c := &loomCLI{}
	if err := c.wire(loc, cwd); err != nil {
		t.Fatalf("wire() = %v; want nil", err)
	}

	if want := loomengine.LoomStatusFile(loc); c.shedPaths.StatusPath != want {
		t.Errorf("c.shedPaths.StatusPath = %q; want %q", c.shedPaths.StatusPath, want)
	}
	if want := loomengine.LoomRunLock(loc); c.shedPaths.LockPath != want {
		t.Errorf("c.shedPaths.LockPath = %q; want %q", c.shedPaths.LockPath, want)
	}
	if want := loomengine.LoomStatusLock(loc); c.shedPaths.StatusLockPath != want {
		t.Errorf("c.shedPaths.StatusLockPath = %q; want %q", c.shedPaths.StatusLockPath, want)
	}
	if want := loc.AnchorPath(); c.env.AnchorPath != want {
		t.Errorf("c.env.AnchorPath = %q; want %q", c.env.AnchorPath, want)
	}
	if want := loc.WorktreePath(); c.env.WorktreeRoot != want {
		t.Errorf("c.env.WorktreeRoot = %q; want %q", c.env.WorktreeRoot, want)
	}
	if want := loomengine.DiscussionDecisionRecord(loc); c.env.DecisionRecordPath != want {
		t.Errorf("c.env.DecisionRecordPath = %q; want %q", c.env.DecisionRecordPath, want)
	}
	if want := loomengine.DiscussionSupportLog(loc); c.env.SupportLogPath != want {
		t.Errorf("c.env.SupportLogPath = %q; want %q", c.env.SupportLogPath, want)
	}
	if want := loomengine.LoomStatusFile(loc); c.env.StatusPath != want {
		t.Errorf("c.env.StatusPath = %q; want %q", c.env.StatusPath, want)
	}
	if want := loomengine.LoomStatusLock(loc); c.env.StatusLockPath != want {
		t.Errorf("c.env.StatusLockPath = %q; want %q", c.env.StatusLockPath, want)
	}
}

// TestWire_RunLockDiffersFromStatusLock asserts the run-lock path differs from the status-lock path,
// the pair shedengine.Shed rejects outright when equal.
func TestWire_RunLockDiffersFromStatusLock(t *testing.T) {
	t.Parallel()

	loc := hubLocation(t, "warp", ".")

	c := &loomCLI{}
	if err := c.wire(loc, loc.AnchorPath()); err != nil {
		t.Fatalf("wire() = %v; want nil", err)
	}

	if c.shedPaths.LockPath == c.shedPaths.StatusLockPath {
		t.Errorf("c.shedPaths.LockPath == c.shedPaths.StatusLockPath == %q; want them distinct", c.shedPaths.LockPath)
	}
}

// TestWire_CwdIsToldToTheEnv asserts c.env.Cwd equals the cwd argument wire was called with -- the
// only preflight-related property wire still owns.
//
// wire no longer builds the Preflight row itself: preflightEntry now builds it from Env.Cwd,
// exactly the way loomshed.Deps.Preflight used to be built here. The old
// TestWire_PreflightIsTheAdapter assertion ("row 1 is the preflightshed adapter, not a bare func")
// moved with that construction -- it is now internal/shedrecipe's own entry test, and the row's
// engine name is pinned by the recipe-side coverage guard in internal/loomrecipe.
func TestWire_CwdIsToldToTheEnv(t *testing.T) {
	t.Parallel()

	loc := hubLocation(t, "warp", ".")
	cwd := loc.AnchorPath()

	c := &loomCLI{}
	if err := c.wire(loc, cwd); err != nil {
		t.Fatalf("wire() = %v; want nil", err)
	}

	if c.env.Cwd != cwd {
		t.Errorf("c.env.Cwd = %q; want %q", c.env.Cwd, cwd)
	}
}

// TestWire_WebsterRunIsFilled asserts c.env.WebsterRun is non-nil.
//
// This is the single most likely regression the conversion reintroduces: websterEntry calls
// requireSeam("Webster", "WebsterRun", env.WebsterRun) and errors on nil, while the pre-conversion
// wire deliberately left the corresponding field (loomshed.Deps.WebsterRun) nil and relied on
// shedadapters.NewWebsterProducer's own nil-defaulting.
func TestWire_WebsterRunIsFilled(t *testing.T) {
	t.Parallel()

	loc := hubLocation(t, "warp", ".")

	c := &loomCLI{}
	if err := c.wire(loc, loc.AnchorPath()); err != nil {
		t.Fatalf("wire() = %v; want nil", err)
	}

	if c.env.WebsterRun == nil {
		t.Error("c.env.WebsterRun = nil; want websterengine.Run")
	}
}

// TestWire_WebsterDepsFullyPopulated asserts every field the webster hub wiring fills is non-zero in
// the assembled websterengine.RunDeps.
func TestWire_WebsterDepsFullyPopulated(t *testing.T) {
	t.Parallel()

	loc := hubLocation(t, "warp", ".")

	c := &loomCLI{}
	if err := c.wire(loc, loc.AnchorPath()); err != nil {
		t.Fatalf("wire() = %v; want nil", err)
	}

	deps := c.runDeps
	if deps.Starter == nil {
		t.Error("runDeps.Starter = nil; want the runnerMasterStarter adapter")
	}
	if deps.Reed == nil {
		t.Error("runDeps.Reed = nil; want the constructed reed engine")
	}
	if deps.Engine == nil {
		t.Error("runDeps.Engine = nil; want the constructed claude engine")
	}
	if deps.Roles == nil {
		t.Error("runDeps.Roles = nil; want the resolved role map")
	}
	if deps.Batcher == nil {
		t.Error("runDeps.Batcher = nil; want the active batchifier")
	}
	if deps.Geom.WebsterDir == "" {
		t.Error("runDeps.Geom.WebsterDir = \"\"; want the told webster geometry")
	}
	if deps.RefMatcher == nil {
		t.Error("runDeps.RefMatcher = nil; want the real fabric reference matcher")
	}
	if deps.OpenBisector == nil {
		t.Error("runDeps.OpenBisector = nil; want the lazy fabric opener")
	}

	// The same value must also be embedded verbatim in c.env.WebsterDeps.
	if c.env.WebsterDeps.Geom != deps.Geom {
		t.Error("c.env.WebsterDeps is not the same value stored in c.runDeps")
	}
}

// TestWire_RefMatcherIsRealScanner asserts the reference matcher is a non-nil *fabricengine.RefScanner
// and never websterengine.NeverMatches, the standalone-only stand-in -- loom is hub-only.
func TestWire_RefMatcherIsRealScanner(t *testing.T) {
	t.Parallel()

	loc := hubLocation(t, "warp", ".")

	c := &loomCLI{}
	if err := c.wire(loc, loc.AnchorPath()); err != nil {
		t.Fatalf("wire() = %v; want nil", err)
	}

	if _, ok := c.runDeps.RefMatcher.(*fabricengine.RefScanner); !ok {
		t.Errorf("runDeps.RefMatcher = %T; want *fabricengine.RefScanner", c.runDeps.RefMatcher)
	}
}

// TestWire_BisectorOpenerNonNilInHubOnlyMode asserts the bisector opener is non-nil, since loom is
// hub-only and always has a fabric repo to open.
func TestWire_BisectorOpenerNonNilInHubOnlyMode(t *testing.T) {
	t.Parallel()

	loc := hubLocation(t, "warp", ".")

	c := &loomCLI{}
	if err := c.wire(loc, loc.AnchorPath()); err != nil {
		t.Fatalf("wire() = %v; want nil", err)
	}

	if c.runDeps.OpenBisector == nil {
		t.Fatal("runDeps.OpenBisector = nil; want a non-nil closure in hub-only mode")
	}
}
