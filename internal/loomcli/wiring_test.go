// wiring_test.go drives wire directly against a hand-built *lyxcwd.Location over a temporary
// directory seeded with the one module config load that does not tolerate absence (loomengine's own
// strict configengine.Load): every other config load this task's wire performs
// (reedengine.LoadConfig, shuttleengine.LoadConfig, websterengine.LoadConfig, modelspec.LoadRegistry,
// batcher.Active) degrades to its embedded template on the same proven-present _lyx/ directory, so
// seeding loom.yaml alone is enough to drive the whole hub-only wire() without touching a real hub.

package loomcli

import (
	"fmt"
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
// loomshed.Deps equals the corresponding loomengine accessor's own output for the same location.
func TestWire_PathFieldsMatchLoomengineAccessors(t *testing.T) {
	t.Parallel()

	loc := hubLocation(t, "warp", ".")
	cwd := loc.AnchorPath()

	c := &loomCLI{}
	if err := c.wire(loc, cwd); err != nil {
		t.Fatalf("wire() = %v; want nil", err)
	}

	if want := loomengine.LoomStatusFile(loc); c.deps.StatusPath != want {
		t.Errorf("c.deps.StatusPath = %q; want %q", c.deps.StatusPath, want)
	}
	if want := loomengine.LoomRunLock(loc); c.deps.LockPath != want {
		t.Errorf("c.deps.LockPath = %q; want %q", c.deps.LockPath, want)
	}
	if want := loomengine.LoomStatusLock(loc); c.deps.StatusLockPath != want {
		t.Errorf("c.deps.StatusLockPath = %q; want %q", c.deps.StatusLockPath, want)
	}
	if want := loc.AnchorPath(); c.deps.AnchorPath != want {
		t.Errorf("c.deps.AnchorPath = %q; want %q", c.deps.AnchorPath, want)
	}
	if want := loc.WorktreePath(); c.deps.WorktreeRoot != want {
		t.Errorf("c.deps.WorktreeRoot = %q; want %q", c.deps.WorktreeRoot, want)
	}
	if want := loomengine.DiscussionDecisionRecord(loc); c.deps.DecisionRecordPath != want {
		t.Errorf("c.deps.DecisionRecordPath = %q; want %q", c.deps.DecisionRecordPath, want)
	}
	if want := loomengine.DiscussionSupportLog(loc); c.deps.SupportLogPath != want {
		t.Errorf("c.deps.SupportLogPath = %q; want %q", c.deps.SupportLogPath, want)
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

	if c.deps.LockPath == c.deps.StatusLockPath {
		t.Errorf("c.deps.LockPath == c.deps.StatusLockPath == %q; want them distinct", c.deps.LockPath)
	}
}

// TestWire_PreflightIsTheAdapter asserts the Preflight field is non-nil and is the adapter type
// (preflightshed.NewPreflight's own concrete type), never a bare function value.
func TestWire_PreflightIsTheAdapter(t *testing.T) {
	t.Parallel()

	loc := hubLocation(t, "warp", ".")

	c := &loomCLI{}
	if err := c.wire(loc, loc.AnchorPath()); err != nil {
		t.Fatalf("wire() = %v; want nil", err)
	}

	if c.deps.Preflight == nil {
		t.Fatal("c.deps.Preflight = nil; want the preflightshed adapter")
	}
	// preflightProducer is unexported in package preflightshed, so its concrete type is confirmed by
	// its rendered %T rather than a type assertion -- proving it is a struct value from that package
	// (the adapter), never a bare func value (which %T would render as
	// "func(context.Context) (shedengine.Outcome, shedengine.OutputPointer, error)").
	gotType := fmt.Sprintf("%T", c.deps.Preflight)
	if gotType != "*preflightshed.preflightProducer" {
		t.Errorf("%%T(c.deps.Preflight) = %q; want %q (the adapter, not a bare function)", gotType, "*preflightshed.preflightProducer")
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

	// The same value must also be embedded verbatim in c.deps.WebsterDeps.
	if c.deps.WebsterDeps.Geom != deps.Geom {
		t.Error("c.deps.WebsterDeps is not the same value stored in c.runDeps")
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
