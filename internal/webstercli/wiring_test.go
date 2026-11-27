// wiring_test.go pins wire's mode truth table at tier 0.
// Every case drives c.wire directly with a told preflight.Mode, never through the real pre-run,
// so no case spawns a process or resolves cwd.
// websterCLI carries no *lymcwd.Location field: a compile-time property no test can assert at runtime.
//
// The hub-mode cases hand wire fictional *lymcwd.Location values whose anchor is absent on disk.
// Every module config load in wire degrades to its embedded template on a proven-absent _lym/ directory,
// so a fictional anchor drives the whole hub branch without touching disk.
// The one entry each hub case creates is the worktree's .git directory.
// hubgeom.ReedGeometry stats it to tell the prime from a task worktree.
//
// The standalone-mode cases reach standalonestate.Derive through cliwire.ResolveStandalone.
// Each such case redirects XDG_STATE_HOME and LOCALAPPDATA to a t.TempDir() before calling wire,
// so both of Derive's per-OS branches land inside the test's own temp tree on every platform.
// None of these cases is parallel, because t.Setenv panics under a parallel test.
//
// Two gaps are deliberate, not omissions.
// No (loc non-nil, ModeStandalone) row exists; preflight.ResolveMode returns a nil Location for both standalone causes.
// The refuse case is absent too: the error aborts in resolvePersistentPreRun before c.wire is called,
// and the real pre-run would spawn git here, breaching the Test Tier Purity Invariant.
// The refusal is pinned in internal/webstercli/cli_integration_test.go.

package webstercli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/cliwire"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/preflight"
	"github.com/Knatte18/loomyard/internal/standalonestate"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// hubLocation returns a *lymcwd.Location standing in for a real hub's prime.
// Its only filesystem preparation is the worktree's .git directory; wire's hub branch tolerates the rest being absent.
func hubLocation(t *testing.T, hub, worktreeName, anchorRel string) *lymcwd.Location {
	t.Helper()
	loc := &lymcwd.Location{HubPath: hub, WorktreeName: worktreeName, AnchorRel: anchorRel}
	if err := os.MkdirAll(filepath.Join(loc.WorktreePath(), ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.git) = %v; want nil", err)
	}
	return loc
}

// seedLoomConfigWithFriction writes <anchorPath>/_lym/config/loom.yaml with a full literal.
// Its friction key is the caller-chosen value; empty means Tier 2 is off.
// Every key is written explicitly so the literal pins every value rather than relying on template fills.
func seedLoomConfigWithFriction(t *testing.T, anchorPath, friction string) {
	t.Helper()
	configDir := filepath.Join(anchorPath, "_lym", "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", configDir, err)
	}
	cfgPath := filepath.Join(configDir, "loom.yaml")
	contents := fmt.Sprintf(`discussion: opus[effort=high]
discussion_timeout_min: 480
discussion_interactive: false
plan: opus[effort=high]
plan_timeout_min: 120
review: opus[effort=high]
review_timeout_min: 240
friction: %s
friction_timeout_min: 30
driver: ""
`, friction)
	if err := os.WriteFile(cfgPath, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", cfgPath, err)
	}
}

// seedStandalonePlanDir writes one minimal, non-empty ".md" file into dir.
// The plan directory then passes the content check that standalone wiring applies, so wire() reaches its success path.
func seedStandalonePlanDir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir standalone plan dir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "00-overview.md"), []byte("placeholder"), 0o644); err != nil {
		t.Fatalf("write standalone plan file: %v", err)
	}
}

// TestWire_ModeHubSelectsHubMode covers the locations that run webster verbs under hub mode without being wired.
// One is an ordinary worktree standing in for the hub's Bolt checkout.
// Another is an unpaired sibling, and the last is a worktree whose paired sibling has been removed.
// Keying mode on whether a location is wired would send every one of these to standalone mode.
// None of them may be refused, and none of them may land in standalone.
func TestWire_ModeHubSelectsHubMode(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name         string
		worktreeName string
	}{
		{"BoardLevelWorktree", fabricengine.BoltDirName},
		{"UnpairedSibling", "orphan-code"},
		{"WorktreeWhosePairWasRemoved", "was-paired-code"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hub := t.TempDir()
			loc := hubLocation(t, hub, tt.worktreeName, ".")

			c := &websterCLI{}
			if err := c.wire(loc, preflight.ModeHub, "", "", "", ""); err != nil {
				t.Fatalf("wire() = %v; want nil -- a healthy-but-unwired hub location must never be refused", err)
			}

			if c.geom.AnchorRoot != loc.AnchorPath() {
				t.Errorf("geom.AnchorRoot = %q; want %q", c.geom.AnchorRoot, loc.AnchorPath())
			}
			if c.geom.WorktreeRoot != loc.WorktreePath() {
				t.Errorf("geom.WorktreeRoot = %q; want %q -- hub mode's WorktreeRoot is the git worktree, not the anchor", c.geom.WorktreeRoot, loc.WorktreePath())
			}
			if c.geom.PaneCwd != loc.AnchorPath() {
				t.Errorf("geom.PaneCwd = %q; want %q -- hub mode's sessions run at the anchor", c.geom.PaneCwd, loc.AnchorPath())
			}
			if c.anchorRel != loc.AnchorRel {
				t.Errorf("anchorRel = %q; want %q", c.anchorRel, loc.AnchorRel)
			}
			if c.refMatcher == nil {
				t.Error("refMatcher = nil; want a non-nil matcher in hub mode")
			}
			if c.runDeps().InstructionFiles == nil {
				t.Error("runDeps().InstructionFiles = nil; want Master's instruction file lister supplied for `lym webster run`")
			}
			if c.geom.PlanDir == "" {
				t.Error("geom.PlanDir is empty; want the hub default")
			}
			if c.reedUp != nil {
				t.Error("wireHub armed c.reedUp; want nil -- hub mode's reed session is not run's to boot")
			}
			wantPause, err := hubgeom.PauseGeometry(loc)
			if err != nil {
				t.Fatalf("hubgeom.PauseGeometry(loc) = %v; want nil", err)
			}
			if got := c.runner.PauseGeometry(); got != wantPause {
				t.Errorf("runner.PauseGeometry() = %+v; want %+v", got, wantPause)
			}
			if c.pause != wantPause {
				t.Errorf("c.pause = %+v; want the record path the runner was told, %+v", c.pause, wantPause)
			}
			if c.geom.VerifyHold == nil {
				t.Error("geom.VerifyHold = nil; want the verify pause hold wired in hub mode")
			}
			if c.openFabric == nil {
				t.Fatal("openFabric = nil; want a wired opener closure in hub mode")
			}

			// The explicit, not-inferred-from-absence-of-error half of the laziness proof: this
			// fictional location's code path does not exist on disk, so calling the very closure
			// wire() built -- were it ever invoked internally -- would fail loud with
			// *fabricengine.ErrMissingPath. wire() itself returned nil above despite that
			// guaranteed failure, which is only possible because wireHub never calls c.openFabric;
			// it only constructs and stores the closure.
			if _, err := c.openFabric(); err == nil {
				t.Fatal("c.openFabric() error = nil; want *fabricengine.ErrMissingPath -- this fixture's code path does not exist, so the opener itself must fail when finally invoked by the test, proving wire() never called it")
			} else if !strings.Contains(err.Error(), "fabricengine:") {
				t.Errorf("c.openFabric() error = %v; want a fabricengine error naming the missing path", err)
			}
		})
	}
}

// hash8For returns the hash8 that standalone state derivation computes for target.
// The caller must first redirect the state home with t.Setenv("XDG_STATE_HOME", ...) or the per-OS equivalent.
func hash8For(t *testing.T, target string) string {
	t.Helper()
	_, hash8, err := standalonestate.Derive(target)
	if err != nil {
		t.Fatalf("standalonestate.Derive(%q) = %v; want nil error", target, err)
	}
	return hash8
}

// TestWire_StandaloneMode wires one standalone CLI over a plain target and checks what the mode promises.
// The worktree root, also the fork-audit workdir and the {{.worktree_root}} prompt token, is the target.
// Every _lym/.lym path and module config base resolves under the derived state directory.
// The seams are nil or eager.
// The RefMatcher is non-nil and never matches; there is no fabric opener;
// the in-process reed bring-up is armed, and the friction directory is empty.
// The default plan directory has its run-refusal mark clear.
// A runner's public entry points return their ordinary verdict, not a told-path refusal.
// It sets XDG_STATE_HOME and LOCALAPPDATA, so it is not parallel.
func TestWire_StandaloneMode(t *testing.T) {
	tiergate.Require(t, 0)
	target := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateHome)
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Cleanup(func() { logger.SetDurableSinkDir("") })
	stateDir := filepath.Join(stateHome, "lym", hash8For(t, target))
	defaultPlanDir := filepath.Join(stateDir, "_lym", "plan")
	seedStandalonePlanDir(t, defaultPlanDir)
	// A worktree language.yaml that standalone wiring must never read.
	targetConfigDir := filepath.Join(target, "_lym", "config")
	if err := os.MkdirAll(targetConfigDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", targetConfigDir, err)
	}
	if err := os.WriteFile(filepath.Join(targetConfigDir, "language.yaml"), []byte("skills:\n  - scribe:a\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(language.yaml) = %v; want nil", err)
	}

	c := &websterCLI{}
	// loc is nil here, as it is for both causes that resolve to ModeStandalone:
	// a plain downloaded repository and an unresolvable cwd.
	// wire consults mode alone to choose the branch.
	if err := c.wire(nil, preflight.ModeStandalone, target, "", "", ""); err != nil {
		t.Fatalf("wire() = %v; want nil", err)
	}

	if c.geom.WorktreeRoot != target {
		t.Errorf("geom.WorktreeRoot = %q; want the target %q", c.geom.WorktreeRoot, target)
	}
	if c.gateTiers != "none" {
		t.Errorf("c.gateTiers = %q; want none for a target with no gate.yaml", c.gateTiers)
	}
	if c.geom.AnchorRoot != stateDir {
		t.Errorf("geom.AnchorRoot = %q; want the derived state directory %q", c.geom.AnchorRoot, stateDir)
	}
	for _, dir := range []struct {
		name string
		got  string
	}{
		{"WebsterDir", c.geom.WebsterDir},
		{"ReportsDir", c.geom.ReportsDir},
		{"PromptsDir", c.geom.PromptsDir},
		{"ScratchDir", c.geom.ScratchDir},
		{"StencilsDir", c.geom.StencilsDir},
		{"PlanDir", c.geom.PlanDir},
	} {
		if !strings.HasPrefix(dir.got, stateDir) {
			t.Errorf("geom.%s = %q; want it to resolve under the derived state directory %q, never under the target", dir.name, dir.got, stateDir)
		}
	}
	if c.geom.PlanDir != defaultPlanDir {
		t.Errorf("geom.PlanDir = %q; want the standalone default %q", c.geom.PlanDir, defaultPlanDir)
	}
	if c.planDirOverridden {
		t.Error("planDirOverridden = true; want false when the plan sits at the default")
	}
	if c.anchorRel != "" {
		t.Errorf("anchorRel = %q; want empty in standalone", c.anchorRel)
	}
	if _, ok := c.refMatcher.(websterengine.NeverMatches); !ok {
		t.Errorf("refMatcher = %T; want websterengine.NeverMatches in standalone", c.refMatcher)
	}
	if c.runDeps().ReadOnly == nil {
		t.Error("runDeps().ReadOnly = nil; want the read-only classifier supplied for `lym webster run`")
	}
	if c.runDeps().InstructionFiles == nil {
		t.Error("runDeps().InstructionFiles = nil; want Master's instruction file lister supplied for `lym webster run`")
	}
	if c.openFabric != nil {
		t.Error("openFabric != nil; want nil in standalone -- there is no fabric repo to reach")
	}
	if c.reedUp == nil {
		t.Error("wireStandalone left c.reedUp nil; want the in-process reed bring-up seam armed")
	}
	if c.frictionDir != "" {
		t.Errorf("c.frictionDir = %q; want \"\" in standalone mode", c.frictionDir)
	}
	if len(c.languageSkills) != 0 {
		t.Errorf("c.languageSkills = %v; want empty in standalone, which reads no language.yaml", c.languageSkills)
	}

	if pause := c.runner.PauseGeometry(); !strings.HasPrefix(pause.RecordPath, stateDir) || pause.Slug != "" {
		t.Errorf("runner.PauseGeometry() = %+v; want a record path under the derived state directory %q and an empty slug", pause, stateDir)
	} else if c.pause != pause {
		t.Errorf("c.pause = %+v; want the record path the runner was told, %+v", c.pause, pause)
	}
	if c.geom.VerifyHold == nil {
		t.Error("geom.VerifyHold = nil; want the verify pause hold wired in standalone mode")
	}

	// A runner that is merely non-nil proves nothing.
	// A new runner holds a told-path verdict and surfaces it only when a verb runs,
	// so this drives a public entry point with a guid no strand will ever match.
	err := c.runner.Interrupt("no-such-strand-guid")
	if err == nil {
		t.Fatal("runner.Interrupt() error = nil; want \"not a shuttle strand\", since no such strand exists")
	}
	if strings.Contains(err.Error(), "NewRunner") || strings.Contains(err.Error(), "NewDetachedRunner") {
		t.Fatalf("runner.Interrupt() error = %v; want the ordinary \"not a shuttle strand\" verdict, not a told-path refusal against standalone's detached anchor/worktree-root pair", err)
	}
	if !strings.Contains(err.Error(), "not a shuttle strand") {
		t.Errorf("runner.Interrupt() error = %v; want it to name \"not a shuttle strand\"", err)
	}
}

// TestWire_PlanDirResolution covers an explicit --plan-dir override in each mode.
// It covers a spelling of the hub default that is no override.
// It covers the standalone refusal for an absent default plan directory.
func TestWire_PlanDirResolution(t *testing.T) {
	tiergate.Require(t, 0)
	t.Run("ExplicitOverride_HubMode", func(t *testing.T) {
		hub := t.TempDir()
		loc := hubLocation(t, hub, "pair", ".")
		override := filepath.Join(t.TempDir(), "custom-plan")

		c := &websterCLI{}
		if err := c.wire(loc, preflight.ModeHub, "", "", override, ""); err != nil {
			t.Fatalf("wire() = %v; want nil", err)
		}
		if c.geom.PlanDir != override {
			t.Errorf("geom.PlanDir = %q; want the override %q", c.geom.PlanDir, override)
		}
		// Master's in-pane verbs are flagless in hub mode too, so a moved plan directory must be recorded here.
		// Standalone mode records it the same way.
		// Otherwise `lym webster run --plan-dir` spawns a Master told to read one directory.
		// Its own begin-batch would re-wire against another.
		if !c.planDirOverridden {
			t.Error("planDirOverridden = false; want true -- hub mode's Master types its verbs flagless too, so run must refuse a moved plan")
		}
		if c.planDirDefault == "" || c.planDirDefault == override {
			t.Errorf("planDirDefault = %q; want the hub default location, distinct from the override", c.planDirDefault)
		}
	})

	// The missing-plan refusal's recourse must name the default location, not --plan-dir alone.
	// Every first-time standalone operator hits this refusal before the plan is staged.
	// Following "pass --plan-dir" got past wiring, only for `run` to refuse it.
	// `run` cannot spawn Master over a moved plan, and the default location is what it requires.
	t.Run("MissingPlanRefusalNamesTheDefaultLocation_StandaloneMode", func(t *testing.T) {
		target := t.TempDir()
		stateHome := t.TempDir()
		t.Setenv("XDG_STATE_HOME", stateHome)
		t.Setenv("LOCALAPPDATA", t.TempDir())
		t.Cleanup(func() { logger.SetDurableSinkDir("") })
		defaultPlanDir := filepath.Join(stateHome, "lym", hash8For(t, target), "_lym", "plan")

		c := &websterCLI{}
		err := c.wire(nil, preflight.ModeStandalone, target, "", "", "")
		if err == nil {
			t.Fatal("wire() = nil; want a refusal — no plan is staged")
		}
		if !strings.Contains(err.Error(), defaultPlanDir) {
			t.Errorf("wire() error = %q; want it to name the default plan directory %q that `run` requires", err, defaultPlanDir)
		}
		if !strings.Contains(err.Error(), "`run` refuses it") {
			t.Errorf("wire() error = %q; want it to say `run` refuses a --plan-dir override, so the recourse is not a dead end", err)
		}
		if !strings.Contains(err.Error(), "--plan-dir") {
			t.Errorf("wire() error = %v; want it to name --plan-dir", err)
		}
	})

	t.Run("DefaultSpellingIsNotAnOverride_HubMode", func(t *testing.T) {
		hub := t.TempDir()
		loc := hubLocation(t, hub, "pair", ".")
		c := &websterCLI{}
		if err := c.wire(loc, preflight.ModeHub, "", "", "", ""); err != nil {
			t.Fatalf("wire() = %v; want nil", err)
		}
		defaultPlanDir := c.geom.PlanDir

		spelled := &websterCLI{}
		if err := spelled.wire(loc, preflight.ModeHub, "", "", filepath.Join(defaultPlanDir, "."), ""); err != nil {
			t.Fatalf("wire() = %v; want nil", err)
		}
		if spelled.planDirOverridden {
			t.Error("planDirOverridden = true; want false -- a --plan-dir naming the default location has moved nothing")
		}
	})

	t.Run("ExplicitOverride_StandaloneMode", func(t *testing.T) {
		target := t.TempDir()
		stateHome := t.TempDir()
		t.Setenv("XDG_STATE_HOME", stateHome)
		t.Setenv("LOCALAPPDATA", t.TempDir())
		t.Cleanup(func() { logger.SetDurableSinkDir("") })
		override := t.TempDir()
		seedStandalonePlanDir(t, override)

		c := &websterCLI{}
		if err := c.wire(nil, preflight.ModeStandalone, target, "", override, ""); err != nil {
			t.Fatalf("wire() = %v; want nil", err)
		}
		if c.geom.PlanDir != override {
			t.Errorf("geom.PlanDir = %q; want the override %q", c.geom.PlanDir, override)
		}
		if !c.planDirOverridden {
			t.Error("planDirOverridden = false; want true for a moved plan directory")
		}
		if c.planDirDefault == "" || c.planDirDefault == override {
			t.Errorf("planDirDefault = %q; want the default location, distinct from the override", c.planDirDefault)
		}
	})
}

// TestWire_RelativeFlagDirsResolveAgainstCwd proves that a relative --plan-dir or --stencils-dir resolves against cwd.
// Unresolved, the value would be read against the CLI's working directory;
// the pane webster spawns runs in the target (standalone) or the anchor (hub).
// The standalone check of the default plan directory also depends on it:
// a relative spelling of the default must not count as an override.
//
// The standalone case checks both halves.
// It passes the default plan directory spelled relative to cwd,
// and it asserts that geom.PlanDir comes out at the absolute default and that planDirOverridden stays false.
// A true planDirOverridden makes `run` refuse to spawn Master over a plan in the default location.
func TestWire_RelativeFlagDirsResolveAgainstCwd(t *testing.T) {
	tiergate.Require(t, 0)
	t.Run("HubMode", func(t *testing.T) {
		hub := t.TempDir()
		loc := hubLocation(t, hub, "pair", ".")
		cwd := t.TempDir()

		// The told stencils directory must exist on disk, because the wiring boundary stats it.
		// This test checks only how the path resolves.
		if err := os.MkdirAll(filepath.Join(cwd, "custom", "stencils"), 0o755); err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}

		c := &websterCLI{}
		if err := c.wire(loc, preflight.ModeHub, cwd, filepath.Join("custom", "stencils"), filepath.Join("custom", "plan"), ""); err != nil {
			t.Fatalf("wire() = %v; want nil", err)
		}
		if want := filepath.Join(cwd, "custom", "plan"); c.geom.PlanDir != want {
			t.Errorf("geom.PlanDir = %q; want the relative --plan-dir resolved against cwd, %q", c.geom.PlanDir, want)
		}
		if want := filepath.Join(cwd, "custom", "stencils"); c.geom.StencilsDir != want {
			t.Errorf("geom.StencilsDir = %q; want the relative --stencils-dir resolved against cwd, %q", c.geom.StencilsDir, want)
		}
	})

	t.Run("StandaloneModeRelativeDefaultIsNotAnOverride", func(t *testing.T) {
		target := t.TempDir()
		stateHome := t.TempDir()
		t.Setenv("XDG_STATE_HOME", stateHome)
		t.Setenv("LOCALAPPDATA", t.TempDir())
		t.Cleanup(func() { logger.SetDurableSinkDir("") })

		hash8 := hash8For(t, target)
		defaultPlanDir := filepath.Join(stateHome, "lym", hash8, "_lym", "plan")
		seedStandalonePlanDir(t, defaultPlanDir)

		// The working directory is the state home, so the default plan directory has a short relative spelling from it.
		// That is the spelling an operator types.
		relativeDefault, err := filepath.Rel(stateHome, defaultPlanDir)
		if err != nil {
			t.Fatalf("filepath.Rel(%q, %q) = %v; want nil", stateHome, defaultPlanDir, err)
		}

		c := &websterCLI{}
		if err := c.wire(nil, preflight.ModeStandalone, stateHome, "", relativeDefault, target); err != nil {
			t.Fatalf("wire() = %v; want nil", err)
		}
		if c.geom.PlanDir != defaultPlanDir {
			t.Errorf("geom.PlanDir = %q; want the relative value resolved against cwd, %q", c.geom.PlanDir, defaultPlanDir)
		}
		if c.planDirOverridden {
			t.Error("planDirOverridden = true; want false -- a relative spelling of the DEFAULT plan directory has not moved the plan, and run must not refuse to spawn Master over it")
		}
	})
}

// TestWireHub_AbsentStencilsDirIsRefused proves that hub mode refuses a --stencils-dir that does not exist.
// Without the check, a typo'd directory fails only at the first prompt render, after the run lock and substrate boot.
// The boundary stat goes through the same cliwire.Module method as standalone's prologue,
// so the two modes cannot drift apart on it.
func TestWireHub_AbsentStencilsDirIsRefused(t *testing.T) {
	tiergate.Require(t, 0)
	hub := t.TempDir()
	loc := hubLocation(t, hub, "pair", ".")
	told := filepath.Join(t.TempDir(), "no-such-stencils")

	c := &websterCLI{}
	err := c.wire(loc, preflight.ModeHub, t.TempDir(), told, "", "")
	if err == nil {
		t.Fatal("wire() = nil; want the unreadable --stencils-dir refusal")
	}
	if !strings.Contains(err.Error(), "--stencils-dir") || !strings.Contains(err.Error(), told) {
		t.Errorf("wire() error = %q; want it to name --stencils-dir and the told directory %q", err.Error(), told)
	}
}

// TestWire_TargetDirRefusedInHubMode proves that hub mode refuses --target-dir with an error naming the flag.
// The refusal comes before any config is loaded: it holds for an empty hub with no module config on disk.
func TestWire_TargetDirRefusedInHubMode(t *testing.T) {
	tiergate.Require(t, 0)
	hub := t.TempDir()
	loc := hubLocation(t, hub, "pair", ".")

	c := &websterCLI{}
	err := c.wire(loc, preflight.ModeHub, "", "", "", filepath.Join(t.TempDir(), "elsewhere"))
	if err == nil {
		t.Fatal("wire() error = nil; want a refusal naming --target-dir")
	}
	if !strings.Contains(err.Error(), "--target-dir") {
		t.Errorf("wire() error = %v; want it to name --target-dir", err)
	}
}

// TestWireHub_LeavesDurableSinkDirUntouched proves that hub mode leaves the durable sink directory alone.
// It sets a sentinel directory before calling wireHub, then asserts that the sink still writes there afterward.
// A wireHub that overwrote the sentinel would send the trace file elsewhere.
//
//testtiming:keep pins that wireHub leaves the durable sink override untouched, which its covering test does not assert
func TestWireHub_LeavesDurableSinkDirUntouched(t *testing.T) {
	tiergate.Require(t, 0)
	sentinelDir := t.TempDir()
	logger.SetDurableSinkDir(sentinelDir)
	t.Cleanup(func() { logger.SetDurableSinkDir("") })

	hub := t.TempDir()
	loc := hubLocation(t, hub, "pair", ".")

	c := &websterCLI{}
	if err := c.wire(loc, preflight.ModeHub, "", "", "", ""); err != nil {
		t.Fatalf("wire() = %v; want nil", err)
	}

	logger.Info("wiring_test: arm the sink")

	matches, err := filepath.Glob(filepath.Join(sentinelDir, "trace-*.log"))
	if err != nil {
		t.Fatalf("glob %s: %v", sentinelDir, err)
	}
	if len(matches) == 0 {
		t.Errorf("no trace-*.log file under sentinel dir %s; want wireHub to have left the sink override untouched", sentinelDir)
	}
}

// seedGitRepositoryRoot marks dir as a git repository root by creating its ".git" entry, and returns dir.
// It writes no git objects and runs no git: the repository-root lookup it feeds reads only the marker's presence.
// A real repository would prove nothing more, and building one would breach the Test Tier Purity Invariant.
func seedGitRepositoryRoot(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir %s/.git: %v", dir, err)
	}
	return dir
}

// TestWireStandalone_SubdirectoryOfRepositoryWiresLikeItsRoot wires from a repository subdirectory.
// It proves that wiring resolves to the same derived state directory as wiring from the root;
// the default plan directory follows from it.
// A subdirectory must not derive a state directory of its own, one that nothing seeds.
func TestWireStandalone_SubdirectoryOfRepositoryWiresLikeItsRoot(t *testing.T) {
	tiergate.Require(t, 0)
	repoRoot := seedGitRepositoryRoot(t, t.TempDir())
	subDir := filepath.Join(repoRoot, "src")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}

	stateHome := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateHome)
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Cleanup(func() { logger.SetDurableSinkDir("") })

	normalizedRoot := standalonestate.Normalize(repoRoot)
	rootStateDir := filepath.Join(stateHome, "lym", hash8For(t, normalizedRoot))
	seedStandalonePlanDir(t, filepath.Join(rootStateDir, "_lym", "plan"))

	c := &websterCLI{}
	if err := c.wire(nil, preflight.ModeStandalone, subDir, "", "", ""); err != nil {
		t.Fatalf("wire() from a repository subdirectory = %v; want nil -- where the operator stands inside a repository must not change which repository webster drives", err)
	}
	if c.geom.WorktreeRoot != normalizedRoot {
		t.Errorf("geom.WorktreeRoot = %q; want the repository root %q", c.geom.WorktreeRoot, normalizedRoot)
	}
	if want := filepath.Join(rootStateDir, "_lym", "plan"); c.geom.PlanDir != want {
		t.Errorf("geom.PlanDir = %q; want the root's own default plan directory %q", c.geom.PlanDir, want)
	}
}

// TestWireModule_DescriptorIsVerbatim pins the text of webster's wireModule descriptor.
// No other test in this package or in internal/cliwire asserts that text.
// Without this test, a reworded field would fail no test anywhere.
func TestWireModule_DescriptorIsVerbatim(t *testing.T) {
	tiergate.Require(t, 0)
	if wireModule.Name != "webster" {
		t.Errorf("wireModule.Name = %q; want %q", wireModule.Name, "webster")
	}
	if want := "state, locks, rendered prompts and trace logs"; wireModule.StateArtifacts != want {
		t.Errorf("wireModule.StateArtifacts = %q; want %q", wireModule.StateArtifacts, want)
	}
	if want := "the repository it drives"; wireModule.TargetRole != want {
		t.Errorf("wireModule.TargetRole = %q; want %q", wireModule.TargetRole, want)
	}
	if want := "Drive a target"; wireModule.TargetRecourse != want {
		t.Errorf("wireModule.TargetRecourse = %q; want %q", wireModule.TargetRecourse, want)
	}
	if want := "the worktree is already the target"; wireModule.HubTargetSubject != want {
		t.Errorf("wireModule.HubTargetSubject = %q; want %q", wireModule.HubTargetSubject, want)
	}
	if wireModule.Plan == nil {
		t.Fatal("wireModule.Plan = nil; want a non-nil PlanRules -- webster parses a plan")
	}
	base := filepath.Join(t.TempDir(), "anchor")
	wantPlanDefault := filepath.Join(base, "_lym", "plan")
	if got := wireModule.Plan.DefaultPlanDir(base); got != wantPlanDefault {
		t.Errorf("wireModule.Plan.DefaultPlanDir(%q) = %q; want %q", base, got, wantPlanDefault)
	}

	t.Run("RefuseTargetDirInHubMode", func(t *testing.T) {
		flag := filepath.Join(t.TempDir(), "elsewhere")
		err := wireModule.RefuseTargetDirInHubMode(flag)
		if err == nil {
			t.Fatal("RefuseTargetDirInHubMode() = nil; want a refusal for a non-empty flag")
		}
		want := "webster: --target-dir is not honoured in hub mode: the worktree is already the target, and honouring any other value would strand its artifacts outside fabric's positive-only commit pathspec"
		if err.Error() != want {
			t.Errorf("RefuseTargetDirInHubMode() error = %q; want %q", err.Error(), want)
		}
		if err := wireModule.RefuseTargetDirInHubMode(""); err != nil {
			t.Errorf("RefuseTargetDirInHubMode(\"\") = %v; want nil", err)
		}
	})

	t.Run("MissingPlanRefusal", func(t *testing.T) {
		planDir := filepath.Join(t.TempDir(), "plan")
		recourse := filepath.Join(t.TempDir(), "default-plan")
		got := wireModule.Plan.MissingPlanRefusal(planDir, recourse)
		want := fmt.Sprintf("webster: standalone plan directory %s does not exist or contains no plan files -- there is no bootstrap and no empty-plan fallback. Place an authored plan at %s, which is what `run` requires (Master's own in-pane verbs are flagless and resolve that default); --plan-dir points the bracket and read-only verbs at a plan elsewhere, but `run` refuses it", planDir, recourse)
		if got != want {
			t.Errorf("MissingPlanRefusal() = %q; want %q", got, want)
		}
	})

	// This subtest composes Name, StateArtifacts and TargetRole into a real message
	// rather than reading them as bare fields: it derives a state directory nested inside the target.
	t.Run("ResolveStandalone_NestedStateDirRefusalNamesTheDescriptorFields", func(t *testing.T) {
		target := t.TempDir()
		t.Setenv("XDG_STATE_HOME", filepath.Join(target, ".local", "state"))
		t.Setenv("LOCALAPPDATA", filepath.Join(target, "AppData", "Local"))

		stateDir, _, err := standalonestate.Derive(target)
		if err != nil {
			t.Fatalf("standalonestate.Derive(%q) = %v; want nil error", target, err)
		}
		normalizedTarget := cliwire.NormalizeForContainment(target)
		normalizedStateDir := cliwire.NormalizeForContainment(stateDir)

		sentinelDir := t.TempDir()
		logger.SetDurableSinkDir(sentinelDir)
		t.Cleanup(func() { logger.SetDurableSinkDir("") })

		_, err = wireModule.ResolveStandalone(cliwire.StandaloneRequest{Cwd: target})
		if err == nil {
			t.Fatal("ResolveStandalone() error = nil; want a refusal -- the state directory nests under the target")
		}
		want := fmt.Sprintf("webster: the derived state directory %s lies inside the standalone target %s: standalone mode keeps its %s strictly outside %s, so the two must be disjoint. The state home is nested under the target -- a repository rooted at your home directory is the usual cause. Point XDG_STATE_HOME (LOCALAPPDATA on Windows) at a directory outside %s and re-run", normalizedStateDir, normalizedTarget, wireModule.StateArtifacts, wireModule.TargetRole, normalizedTarget)
		if err.Error() != want {
			t.Errorf("ResolveStandalone() error = %q; want %q", err.Error(), want)
		}
	})
}

// TestWireHub_FrictionDirResolution covers the tolerant friction-directory resolution of wireHub.
// It returns a non-empty directory when loom's config enables Tier 2.
// It returns an empty directory with no error when the friction key is present but empty.
// It returns an empty directory with no error when loom.yaml is absent.
// That tolerance matters: a `lym webster` verb must never fail because an unrelated module's config could not be read.
func TestWireHub_FrictionDirResolution(t *testing.T) {
	tiergate.Require(t, 0)
	t.Run("FrictionEnabled", func(t *testing.T) {
		hub := t.TempDir()
		loc := hubLocation(t, hub, "pair", ".")
		seedLoomConfigWithFriction(t, loc.AnchorPath(), "opus[effort=high]")

		c := &websterCLI{}
		if err := c.wire(loc, preflight.ModeHub, "", "", "", ""); err != nil {
			t.Fatalf("wire() = %v; want nil", err)
		}

		want := loomengine.LoomFrictionDir(loc)
		if c.frictionDir != want {
			t.Errorf("c.frictionDir = %q; want %q", c.frictionDir, want)
		}
	})

	t.Run("FrictionPresentButEmpty", func(t *testing.T) {
		hub := t.TempDir()
		loc := hubLocation(t, hub, "pair", ".")
		seedLoomConfigWithFriction(t, loc.AnchorPath(), "")

		c := &websterCLI{}
		if err := c.wire(loc, preflight.ModeHub, "", "", "", ""); err != nil {
			t.Fatalf("wire() = %v; want nil", err)
		}

		if c.frictionDir != "" {
			t.Errorf("c.frictionDir = %q; want \"\" when the friction key is present but empty", c.frictionDir)
		}
	})

	t.Run("LoomConfigAbsent_ResolvesEmptyWithoutError", func(t *testing.T) {
		// Leave loom.yaml unseeded: the hub location's anchor has no _lym/config directory.
		// As a result, loomengine.LoadConfig fails with its own "not initialized" error.
		// That error must never propagate out of wire.
		hub := t.TempDir()
		loc := hubLocation(t, hub, "pair", ".")

		c := &websterCLI{}
		if err := c.wire(loc, preflight.ModeHub, "", "", "", ""); err != nil {
			t.Fatalf("wire() = %v; want nil -- an unreadable loom config must never fail a webster verb", err)
		}

		if c.frictionDir != "" {
			t.Errorf("c.frictionDir = %q; want \"\" when loom.yaml cannot be loaded", c.frictionDir)
		}
	})
}

// TestWireHub_LanguageSkills asserts that hub wiring loads the hub's language.yaml once.
// It hands the list to both the master's and the recovery strand's deps,
// and an unusable file refuses with the module's own way forward.
// Writing the file creates the board directory.
// Every other module wireHub loads then resolves from an absent board default.
// Wire therefore reaches the language load in every row.
func TestWireHub_LanguageSkills(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name         string
		fileContents string
		wantSkills   []string
		wantRename   []string
		wantRefusal  bool
		// noGate leaves the hub without a gate.yaml; wantGateRefusal writes one wire cannot load.
		noGate          bool
		wantGateRefusal bool
	}{
		{
			name:         "ListedSkillsAndRenameToolsReachRunBeginAndRecoverDepsAndGateTiersRenderFromGateYaml",
			fileContents: "skills:\n  - scribe:golang-comments\n  - scribe:golang-testing\nverify_tiers:\n  - a retired key still set\nrename_tools:\n  - rename Go symbols with gopls rename\n",
			wantSkills:   []string{"scribe:golang-comments", "scribe:golang-testing"},
			wantRename:   []string{"rename Go symbols with gopls rename"},
		},
		{
			name:         "EntryWithLeadingSpaceIsRefused",
			fileContents: "skills:\n  - \" scribe:golang-comments\"\n",
			wantRefusal:  true,
		},
		{
			name:            "UnloadableGateYamlRefusesNamingLymConfigGate",
			fileContents:    "skills: []\n",
			wantGateRefusal: true,
		},
		{
			name:         "NoGateYamlWiresAndRendersNone",
			fileContents: "skills: []\n",
			noGate:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hub := t.TempDir()
			loc := hubLocation(t, hub, "pair", ".")
			configDir := filepath.Join(fabricengine.BoardDir(hub), "_lym", "config")
			if err := os.MkdirAll(configDir, 0o755); err != nil {
				t.Fatalf("MkdirAll(%q) = %v; want nil", configDir, err)
			}
			if err := os.WriteFile(filepath.Join(configDir, "language.yaml"), []byte(tt.fileContents), 0o644); err != nil {
				t.Fatalf("WriteFile(language.yaml) = %v; want nil", err)
			}
			switch {
			case tt.wantGateRefusal:
				if err := os.WriteFile(filepath.Join(configDir, "gate.yaml"), []byte("tiers: [\n"), 0o644); err != nil {
					t.Fatalf("WriteFile(gate.yaml) = %v; want nil", err)
				}
			case !tt.noGate:
				seedHubGateConfig(t, hub)
			}

			c := &websterCLI{}
			err := c.wire(loc, preflight.ModeHub, "", "", "", "")

			if tt.wantGateRefusal {
				if err == nil || !strings.Contains(err.Error(), "lym config gate") {
					t.Fatalf("wire() = %v; want a refusal naming lym config gate", err)
				}
				return
			}
			if tt.noGate {
				if err != nil {
					t.Fatalf("wire() = %v; want nil when the hub has no gate.yaml", err)
				}
				if c.gateTiers != "none" {
					t.Errorf("c.gateTiers = %q; want none", c.gateTiers)
				}
				return
			}
			if tt.wantRefusal {
				var refusal configLoadRefusal
				if !errors.As(err, &refusal) {
					t.Fatalf("wire() = %v; want a configLoadRefusal", err)
				}
				if !refusal.invalid {
					t.Error("refusal.invalid = false; want true, so the refusal carries config_invalid")
				}
				for _, want := range []string{"language.yaml", "skills", "lym config language"} {
					if !strings.Contains(refusal.message, want) {
						t.Errorf("refusal message = %q; want it to contain %q", refusal.message, want)
					}
				}
				if strings.Contains(refusal.message, "under _lym/config") {
					t.Errorf("refusal message = %q; want no worktree _lym/config clause for a hub-wide file", refusal.message)
				}
				return
			}

			if err != nil {
				t.Fatalf("wire() = %v; want nil", err)
			}
			if !slices.Equal(c.languageSkills, tt.wantSkills) {
				t.Errorf("c.languageSkills = %v; want %v", c.languageSkills, tt.wantSkills)
			}
			if got := c.runDeps().LanguageSkills; !slices.Equal(got, tt.wantSkills) {
				t.Errorf("runDeps().LanguageSkills = %v; want %v", got, tt.wantSkills)
			}
			begin, err := c.beginDeps(nil, nil, nil)
			if err != nil {
				t.Fatalf("beginDeps() = %v; want nil", err)
			}
			recovered, err := c.recoverDeps(nil, nil, nil)
			if err != nil {
				t.Fatalf("recoverDeps() = %v; want nil", err)
			}
			if !slices.Equal(recovered.LanguageSkills, tt.wantSkills) {
				t.Errorf("recoverDeps().LanguageSkills = %v; want %v", recovered.LanguageSkills, tt.wantSkills)
			}
			if !strings.HasPrefix(c.gateTiers, "- tier0:") {
				t.Errorf("c.gateTiers = %q; want the seeded gate.yaml's tier lines", c.gateTiers)
			}
			if begin.GateTiers != c.gateTiers {
				t.Errorf("beginDeps().GateTiers = %q; want %q", begin.GateTiers, c.gateTiers)
			}
			if recovered.GateTiers != c.gateTiers {
				t.Errorf("recoverDeps().GateTiers = %q; want %q", recovered.GateTiers, c.gateTiers)
			}
			if !slices.Equal(c.renameTools, tt.wantRename) {
				t.Errorf("c.renameTools = %v; want %v", c.renameTools, tt.wantRename)
			}
			if !slices.Equal(begin.RenameTools, tt.wantRename) {
				t.Errorf("beginDeps().RenameTools = %v; want %v", begin.RenameTools, tt.wantRename)
			}
			if !slices.Equal(recovered.RenameTools, tt.wantRename) {
				t.Errorf("recoverDeps().RenameTools = %v; want %v", recovered.RenameTools, tt.wantRename)
			}
			if _, err := begin.Gate.TierList("batch"); err != nil {
				t.Errorf("beginDeps().Gate.TierList(batch) = %v; want the seeded gate config", err)
			}
		})
	}
}

// TestProductionFiles_NeverReferenceHubWatchdogMechanism proves no production file uses the per-hub watchdog daemon.
// It belongs to hub mode alone: standalone computes no hub lock path (fabricengine.HubScratchDir),
// and it spawns no daemon (the "reed watchdog" verb).
//
//testtiming:keep internal/loomcli/cli_test.go names this test
func TestProductionFiles_NeverReferenceHubWatchdogMechanism(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	matches, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob *.go: %v", err)
	}
	for _, path := range matches {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		content := string(data)
		if strings.Contains(content, "fabricengine.HubScratchDir") {
			t.Errorf("%s references fabricengine.HubScratchDir; standalone must compute no hub lock path", path)
		}
		if strings.Contains(content, "reed watchdog") {
			t.Errorf("%s references \"reed watchdog\"; standalone must spawn no daemon", path)
		}
	}
}
