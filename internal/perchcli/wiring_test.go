// wiring_test.go pins wire's mode truth table at tier 1: every case drives c.wire directly with a
// told preflight.Mode -- never through the real pre-run -- so no case spawns a process or resolves
// cwd.
//
// The hub-mode cases below hand wire a fictional, on-disk-absent *lyxcwd.Location: every module
// config load wire performs (shuttleengine.LoadConfig, reedengine.LoadConfig, modelspec.LoadRegistry,
// perchengine.LoadConfigWithRegistry, burlerengine.LoadConfig) degrades to its embedded template (or,
// for burler.yaml/models.yaml, the zero value) on a proven-absent _lyx/ directory, so a fictional
// anchor path drives the whole hub branch without touching disk.
//
// The standalone-mode cases reach the one call site of standalonestate.Derive that exists anywhere
// in this package: each such case redirects both XDG_STATE_HOME and LOCALAPPDATA to a t.TempDir()
// BEFORE calling wire, so both of Derive's per-OS branches land inside the test's own temp tree on
// every platform, and none of those cases is marked t.Parallel(), since t.Setenv panics under a
// parallel test.
//
// Two structural facts a later reader might otherwise try to "fix": first, no (loc non-nil,
// ModeStandalone) row exists in this file because no caller can produce one -- preflight.ResolveMode
// returns a nil Location for both standalone causes. Second, the refuse case is deliberately absent
// from this file because wire never receives it -- the error return aborts upstream in
// resolvePersistentPreRun before c.wire is ever called -- and manufacturing one here would require
// driving the real pre-run, which spawns git and breaches the Test Tier Purity Invariant, the exact
// invariant wire's extraction exists to satisfy. The refusal is pinned instead in internal/preflight's
// own integration table.
//
// What this file does NOT assert: the reed geometry's field values. shuttleengine.Runner holds every
// field unexported with no geometry accessor, and perchCLI stores only c.runner, so SocketKey,
// SessionName, LogsDir, RepoName, and HubPath are unreachable from this in-package test. Those five
// values are already pinned one layer down, by TestReedGeometry in
// internal/standalonegeom/standalonegeom_test.go, which asserts all eight fields against told
// parameters. What that leaves uncovered by any test is the *linkage* -- that wireStandalone calls
// standalonegeom.ReedGeometry with this invocation's own target, stateDir, and hash8 rather than some
// other triple. No seam exposes it, so it is a review obligation rather than an assertion, recorded
// here so a later reader knows the gap is recorded rather than overlooked.

package perchcli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/perchengine"
	"github.com/Knatte18/loomyard/internal/preflight"
	"github.com/Knatte18/loomyard/internal/standalonegeom"
	"github.com/Knatte18/loomyard/internal/standalonestate"
)

// hubLocation returns a *lyxcwd.Location standing in for a real hub location. It performs no
// filesystem preparation of its own -- see the file header for why wire's hub branch tolerates that.
func hubLocation(hub, worktreeName, anchorRel string) *lyxcwd.Location {
	return &lyxcwd.Location{HubPath: hub, WorktreeName: worktreeName, AnchorRel: anchorRel}
}

// setStandaloneStateRoot redirects both env vars standalonestate.Derive reads to fresh t.TempDir()
// values, so a case that reaches wireStandalone stays hermetic. Not t.Parallel() -- t.Setenv panics
// under a parallel test.
func setStandaloneStateRoot(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("LOCALAPPDATA", t.TempDir())
}

// hash8AndStateDir derives both the state directory and hash8 for target under the environment
// t.Setenv has already redirected.
func hash8AndStateDir(t *testing.T, target string) (string, string) {
	t.Helper()
	stateDir, hash8, err := standalonestate.Derive(target)
	if err != nil {
		t.Fatalf("standalonestate.Derive(%q) = %v; want nil error", target, err)
	}
	return stateDir, hash8
}

// TestWire_ModeHubSelectsHubMode covers the (loc non-nil, ModeHub) row: wire must select hub mode and
// resolve c.mode/c.stateDir/c.stencilsDir/c.anchorRel/c.openFabric to their hub-mode values.
func TestWire_ModeHubSelectsHubMode(t *testing.T) {
	t.Parallel()

	hub := t.TempDir()
	loc := hubLocation(hub, "warp", "sub")

	c := &perchCLI{}
	if err := c.wire(loc, preflight.ModeHub, "", "", ""); err != nil {
		t.Fatalf("wire() = %v; want nil", err)
	}

	if c.mode != "hub" {
		t.Errorf("c.mode = %q; want %q", c.mode, "hub")
	}
	if c.stateDir != "" {
		t.Errorf("c.stateDir = %q; want empty in hub mode", c.stateDir)
	}
	if c.anchorRel != loc.AnchorRel {
		t.Errorf("c.anchorRel = %q; want %q (loc.AnchorRel)", c.anchorRel, loc.AnchorRel)
	}
	if c.openFabric == nil {
		t.Error("c.openFabric = nil; want a non-nil closure in hub mode")
	}
	if c.burlerEngine == nil {
		t.Fatal("c.burlerEngine = nil; want a constructed *burlerengine.Engine")
	}

	want := perchengine.RunsDir(c.perchGeom.AnchorPath)
	if c.runDirBase != want {
		t.Errorf("c.runDirBase = %q; want %q", c.runDirBase, want)
	}
	wantScratch := perchengine.ScratchDir(c.perchGeom.AnchorPath)
	if c.scratchDirBase != wantScratch {
		t.Errorf("c.scratchDirBase = %q; want %q", c.scratchDirBase, wantScratch)
	}
}

// TestWire_ModeStandaloneSelectsStandaloneMode covers both causes preflight.ResolveMode folds into
// ModeStandalone: a plain downloaded git repository (no hub-level directory beside it) and a genuine
// non-repository directory. ResolveMode returns a nil Location for each, and wire cannot and must not
// try to tell them apart -- both land in standalone mode.
func TestWire_ModeStandaloneSelectsStandaloneMode(t *testing.T) {
	tests := []struct {
		name string
	}{
		{"PlainDownloadedRepository"},
		{"NonRepositoryDirectory"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := t.TempDir()
			setStandaloneStateRoot(t)

			c := &perchCLI{}
			// loc is nil regardless of which real ResolveMode cause produced ModeStandalone: both
			// carry a nil Location, and wire consults mode alone to choose the dispatch branch.
			if err := c.wire(nil, preflight.ModeStandalone, target, "", ""); err != nil {
				t.Fatalf("wire() = %v; want nil", err)
			}

			if c.mode != "standalone" {
				t.Errorf("c.mode = %q; want %q", c.mode, "standalone")
			}
			stateDir, _, err := standalonestate.Derive(target)
			if err != nil {
				t.Fatalf("standalonestate.Derive(%q) = %v; want nil error", target, err)
			}
			if c.stateDir != stateDir {
				t.Errorf("c.stateDir = %q; want %q", c.stateDir, stateDir)
			}
			if want := standalonegeom.StencilsDir(stateDir); c.stencilsDir != want {
				t.Errorf("c.stencilsDir = %q; want %q", c.stencilsDir, want)
			}
			if c.anchorRel != "" {
				t.Errorf("c.anchorRel = %q; want empty in standalone mode", c.anchorRel)
			}
			if c.openFabric != nil {
				t.Error("c.openFabric != nil; want nil in standalone mode -- no fabric repo by construction")
			}
		})
	}
}

// TestWireStandalone_NeverReadsLoc proves wireStandalone never reads loc -- it takes only cwd and the
// flags.
func TestWireStandalone_NeverReadsLoc(t *testing.T) {
	target := t.TempDir()
	setStandaloneStateRoot(t)

	c := &perchCLI{}
	if err := c.wireStandalone(target, "", ""); err != nil {
		t.Fatalf("wireStandalone() = %v; want nil", err)
	}
	if c.mode != "standalone" {
		t.Errorf("c.mode = %q; want %q", c.mode, "standalone")
	}
}

// TestWire_StandalonePinnedValues pins every standalone value the design names: c.perchGeom.GateDir
// equals the target and c.perchGeom.AnchorPath equals stateDir, c.runDirBase equals
// perchengine.RunsDir(stateDir) and c.scratchDirBase equals perchengine.ScratchDir(stateDir) -- both
// therefore under stateDir -- the config base is stateDir (proven indirectly by wire succeeding
// against a fictional, on-disk-absent state directory), and c.stencilsDir equals
// standalonegeom.StencilsDir(stateDir) when the flag is unset.
func TestWire_StandalonePinnedValues(t *testing.T) {
	target := t.TempDir()
	setStandaloneStateRoot(t)
	stateDir, hash8 := hash8AndStateDir(t, target)

	c := &perchCLI{}
	if err := c.wire(nil, preflight.ModeStandalone, target, "", ""); err != nil {
		t.Fatalf("wire() = %v; want nil", err)
	}

	if c.perchGeom.GateDir != target {
		t.Errorf("c.perchGeom.GateDir = %q; want %q (target)", c.perchGeom.GateDir, target)
	}
	if c.perchGeom.AnchorPath != stateDir {
		t.Errorf("c.perchGeom.AnchorPath = %q; want %q (stateDir)", c.perchGeom.AnchorPath, stateDir)
	}
	if want := perchengine.RunsDir(stateDir); c.runDirBase != want {
		t.Errorf("c.runDirBase = %q; want %q", c.runDirBase, want)
	}
	if want := perchengine.ScratchDir(stateDir); c.scratchDirBase != want {
		t.Errorf("c.scratchDirBase = %q; want %q", c.scratchDirBase, want)
	}
	if c.stateDir != stateDir {
		t.Errorf("c.stateDir = %q; want %q", c.stateDir, stateDir)
	}
	if want := standalonegeom.StencilsDir(stateDir); c.stencilsDir != want {
		t.Errorf("c.stencilsDir = %q; want %q", c.stencilsDir, want)
	}

	// standalonegeom.ReedGeometry's own field values are pinned by TestReedGeometry in
	// internal/standalonegeom/standalonegeom_test.go; hash8 is asserted here only to prove this
	// invocation derived one at all, not to re-pin ReedGeometry's mapping. The linkage -- that
	// wireStandalone passes THIS invocation's own target, stateDir, and hash8 to
	// standalonegeom.ReedGeometry -- is a review obligation, not an assertion (see file header).
	if hash8 == "" {
		t.Error("hash8 = \"\"; want a derived hash8")
	}
}

// TestWire_TargetDirRefusedInHubMode proves --target-dir is refused in hub mode with an error
// explaining why, and that the refusal happens before any config is loaded (an empty hub, no module
// config on disk anywhere, still refuses on the flag alone).
func TestWire_TargetDirRefusedInHubMode(t *testing.T) {
	t.Parallel()

	hub := t.TempDir()
	loc := hubLocation(hub, "warp", ".")

	c := &perchCLI{}
	err := c.wire(loc, preflight.ModeHub, "", "", filepath.Join(t.TempDir(), "elsewhere"))
	if err == nil {
		t.Fatal("wire() error = nil; want a refusal naming --target-dir")
	}
	if !strings.Contains(err.Error(), "--target-dir") {
		t.Errorf("wire() error = %v; want it to name --target-dir", err)
	}
}

// TestWire_StencilsDirFlag covers --stencils-dir: honoured in both modes, the standalone default
// seeded on disk when the flag is unset, an explicit --stencils-dir never written to, and -- the
// load-bearing one -- ONE stencilsDir value reaching BOTH consumers (the nested burler engine and the
// value run.go would pass to perchengine.Engine.Run) in both modes.
//
// The nested burler engine's stencils directory is not observable from outside *burlerengine.Engine
// (its stencilsDir field is unexported with no accessor), so rather than silently weakening the
// assertion this test instead asserts that exactly one stencilsDir value exists on the receiver
// (c.stencilsDir) and that wireHub/wireStandalone pass that SAME field -- not a second, independently
// resolved value -- to burlerengine.New. That equality is enforced by construction (wiring.go passes
// stencilsDir, the local wire computed, to both burlerengine.New and c.stencilsDir), so this test pins
// c.stencilsDir against the expected value in each row and relies on a code-reading review, recorded
// here, to confirm burlerengine.New receives the identical local rather than a re-derived one.
func TestWire_StencilsDirFlag(t *testing.T) {
	t.Run("HonouredInHubMode", func(t *testing.T) {
		t.Parallel()
		hub := t.TempDir()
		loc := hubLocation(hub, "warp", ".")
		override := filepath.Join(t.TempDir(), "custom-stencils")

		c := &perchCLI{}
		if err := c.wire(loc, preflight.ModeHub, "", override, ""); err != nil {
			t.Fatalf("wire() = %v; want nil", err)
		}
		if c.stencilsDir != override {
			t.Errorf("c.stencilsDir = %q; want the override %q", c.stencilsDir, override)
		}
	})

	t.Run("HonouredInStandaloneMode", func(t *testing.T) {
		target := t.TempDir()
		setStandaloneStateRoot(t)
		override := t.TempDir()

		c := &perchCLI{}
		if err := c.wire(nil, preflight.ModeStandalone, target, override, ""); err != nil {
			t.Fatalf("wire() = %v; want nil", err)
		}
		if c.stencilsDir != override {
			t.Errorf("c.stencilsDir = %q; want the override %q", c.stencilsDir, override)
		}
	})

	t.Run("ExplicitOverride_NeverWrittenTo", func(t *testing.T) {
		target := t.TempDir()
		setStandaloneStateRoot(t)
		override := t.TempDir()

		before, err := readDirNames(t, override)
		if err != nil {
			t.Fatalf("readDirNames(%q) = %v", override, err)
		}
		if len(before) != 0 {
			t.Fatalf("fixture %q is not empty before wire(): %v", override, before)
		}

		c := &perchCLI{}
		if err := c.wire(nil, preflight.ModeStandalone, target, override, ""); err != nil {
			t.Fatalf("wire() = %v; want nil", err)
		}

		after, err := readDirNames(t, override)
		if err != nil {
			t.Fatalf("readDirNames(%q) = %v", override, err)
		}
		if len(after) != 0 {
			t.Errorf("explicit --stencils-dir %q gained entries: %v; want it untouched -- an explicit override is read-only, never seeded", override, after)
		}
	})

	t.Run("StandaloneDefaultSeededOnDisk", func(t *testing.T) {
		target := t.TempDir()
		setStandaloneStateRoot(t)
		stateDir, _ := hash8AndStateDir(t, target)

		c := &perchCLI{}
		if err := c.wire(nil, preflight.ModeStandalone, target, "", ""); err != nil {
			t.Fatalf("wire() = %v; want nil", err)
		}

		want := standalonegeom.StencilsDir(stateDir)
		entries, err := readDirNames(t, want)
		if err != nil {
			t.Fatalf("readDirNames(%q) = %v; want the standalone default to be seeded on disk", want, err)
		}
		if len(entries) == 0 {
			t.Errorf("standalone default stencils dir %q is empty; want it seeded", want)
		}
	})

	t.Run("OneValueReachesBothConsumers_HubMode", func(t *testing.T) {
		t.Parallel()
		hub := t.TempDir()
		loc := hubLocation(hub, "warp", ".")
		override := filepath.Join(t.TempDir(), "custom-stencils")

		c := &perchCLI{}
		if err := c.wire(loc, preflight.ModeHub, "", override, ""); err != nil {
			t.Fatalf("wire() = %v; want nil", err)
		}
		// c.stencilsDir is what run.go passes to perchengine.Engine.Run; wiring.go passes the
		// identical local to burlerengine.New for the nested burler engine -- see the test-level
		// doc comment for why that second half is a review obligation rather than an assertion.
		if c.stencilsDir != override {
			t.Errorf("c.stencilsDir = %q; want the override %q reaching both consumers", c.stencilsDir, override)
		}
	})

	t.Run("OneValueReachesBothConsumers_StandaloneMode", func(t *testing.T) {
		target := t.TempDir()
		setStandaloneStateRoot(t)
		override := t.TempDir()

		c := &perchCLI{}
		if err := c.wire(nil, preflight.ModeStandalone, target, override, ""); err != nil {
			t.Fatalf("wire() = %v; want nil", err)
		}
		if c.stencilsDir != override {
			t.Errorf("c.stencilsDir = %q; want the override %q reaching both consumers", c.stencilsDir, override)
		}
	})
}

// readDirNames returns the entry names inside dir, creating no directory of its own.
func readDirNames(t *testing.T, dir string) ([]string, error) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	return names, nil
}

// TestResolveStandaloneTarget covers resolveStandaloneTarget's three rows: unset returns cwd,
// absolute returns the cleaned path, relative returns the path joined onto cwd.
func TestResolveStandaloneTarget(t *testing.T) {
	t.Parallel()

	cwd := filepath.Join(string(filepath.Separator), "home", "operator", "repo")

	tests := []struct {
		name          string
		targetDirFlag string
		want          string
	}{
		{"Unset", "", cwd},
		{"Absolute", filepath.Join(string(filepath.Separator), "elsewhere", "target"), filepath.Join(string(filepath.Separator), "elsewhere", "target")},
		{"Relative", filepath.Join("..", "sibling"), filepath.Join(cwd, "..", "sibling")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveStandaloneTarget(cwd, tt.targetDirFlag)
			if err != nil {
				t.Fatalf("resolveStandaloneTarget(%q, %q) = %v; want nil error", cwd, tt.targetDirFlag, err)
			}
			if got != filepath.Clean(tt.want) {
				t.Errorf("resolveStandaloneTarget(%q, %q) = %q; want %q", cwd, tt.targetDirFlag, got, filepath.Clean(tt.want))
			}
		})
	}
}
