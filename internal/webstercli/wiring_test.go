// wiring_test.go pins wire's mode truth table at tier 1: every case drives c.wire directly with a
// told preflight.Mode -- never through the real pre-run -- so no case spawns a process or resolves
// cwd. websterCLI itself carries no *lyxcwd.Location field (see cli.go's struct): that is a
// compile-time property of this package, not something a test can additionally assert at runtime.
//
// The hub-mode cases below hand wire fictional, on-disk-absent *lyxcwd.Location values: every module
// config load wire performs (shuttleengine.LoadConfig, reedengine.LoadConfig, websterengine.LoadConfig,
// batcher.Active, modelspec.LoadRegistry) degrades to its embedded template on a proven-absent _lyx/
// directory, so a fictional anchor path drives the whole hub branch without touching disk.
//
// The standalone-mode cases reach the one call site of standalonestate.Derive that exists anywhere in
// this codebase: each such case redirects both XDG_STATE_HOME and LOCALAPPDATA to a t.TempDir() BEFORE
// calling wire, so both of Derive's per-OS branches land inside the test's own temp tree on every
// platform, and none of those cases is marked t.Parallel(), since t.Setenv panics under a parallel
// test.
//
// Two structural facts a later reader might otherwise try to "fix": first, no (loc non-nil,
// ModeStandalone) row exists in this file because no caller can produce one -- preflight.ResolveMode
// returns a nil Location for both standalone causes. Second, the refuse case is deliberately absent
// from this file because wire never receives it -- the error return aborts upstream in
// resolvePersistentPreRun before c.wire is ever called -- and manufacturing one here would require
// driving the real pre-run, which spawns git and breaches the Test Tier Purity Invariant, the exact
// invariant wire's extraction exists to satisfy. The refusal is pinned instead in
// internal/webstercli/cli_integration_test.go.

package webstercli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/preflight"
	"github.com/Knatte18/loomyard/internal/standalonegeom"
	"github.com/Knatte18/loomyard/internal/standalonestate"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// hubLocation returns a *lyxcwd.Location standing in for a real hub location. It performs no
// filesystem preparation of its own -- see the file header for why wire's hub branch tolerates that.
func hubLocation(hub, worktreeName, anchorRel string) *lyxcwd.Location {
	return &lyxcwd.Location{HubPath: hub, WorktreeName: worktreeName, AnchorRel: anchorRel}
}

// seedStandalonePlanDir writes one minimal, non-empty ".md" file into dir, satisfying
// standalonePlanDirHasContent so a standalone wire() call reaches its own success path.
func seedStandalonePlanDir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir standalone plan dir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "00-overview.md"), []byte("placeholder"), 0o644); err != nil {
		t.Fatalf("write standalone plan file: %v", err)
	}
}

// TestWire_ModeHubSelectsHubMode covers the three healthy-but-unwired locations that run
// webster verbs today under hub mode: an ordinary worktree standing in for <hub>/_board, an unpaired
// sibling, and a worktree whose paired sibling has been removed. A Wired-keyed implementation would
// silently send every one of these to standalone mode; none of them may be refused, and none of them
// may land in standalone.
func TestWire_ModeHubSelectsHubMode(t *testing.T) {
	tests := []struct {
		name         string
		worktreeName string
	}{
		{"BoardLevelWorktree", "_board"},
		{"UnpairedSibling", "orphan-warp"},
		{"WorktreeWhosePairWasRemoved", "was-paired-warp"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hub := t.TempDir()
			loc := hubLocation(hub, tt.worktreeName, ".")

			c := &websterCLI{}
			if err := c.wire(loc, preflight.ModeHub, "", "", "", ""); err != nil {
				t.Fatalf("wire() = %v; want nil -- a healthy-but-unwired hub location must never be refused", err)
			}

			if c.geom.AnchorRoot != loc.AnchorPath() {
				t.Errorf("geom.AnchorRoot = %q; want %q", c.geom.AnchorRoot, loc.AnchorPath())
			}
			if c.geom.WorktreeRoot != loc.AnchorPath() {
				t.Errorf("geom.WorktreeRoot = %q; want %q -- hub mode's WorktreeRoot is the anchor path", c.geom.WorktreeRoot, loc.AnchorPath())
			}
			if c.anchorRel != loc.AnchorRel {
				t.Errorf("anchorRel = %q; want %q", c.anchorRel, loc.AnchorRel)
			}
			if c.refMatcher == nil {
				t.Error("refMatcher = nil; want a non-nil matcher in hub mode")
			}
			if c.openFabric == nil {
				t.Fatal("openFabric = nil; want a wired opener closure in hub mode")
			}

			// The explicit, not-inferred-from-absence-of-error half of the laziness proof: this
			// fictional location's warp path does not exist on disk, so calling the very closure
			// wire() built -- were it ever invoked internally -- would fail loud with
			// *fabricengine.ErrMissingPath. wire() itself returned nil above despite that
			// guaranteed failure, which is only possible because wireHub never calls c.openFabric;
			// it only constructs and stores the closure.
			if _, err := c.openFabric(); err == nil {
				t.Fatal("c.openFabric() error = nil; want *fabricengine.ErrMissingPath -- this fixture's warp path does not exist, so the opener itself must fail when finally invoked by the test, proving wire() never called it")
			} else if !strings.Contains(err.Error(), "fabricengine:") {
				t.Errorf("c.openFabric() error = %v; want a fabricengine error naming the missing path", err)
			}
		})
	}
}

// TestWire_ModeStandaloneSelectsStandaloneMode covers both causes preflight.ResolveMode folds into
// ModeStandalone: a plain downloaded git repository (no hub-level directory beside it) and an
// unresolvable cwd (lyxcwd.Resolve itself failed). ResolveMode returns a nil Location for each, and
// wire cannot and must not try to tell them apart -- both land in standalone mode.
func TestWire_ModeStandaloneSelectsStandaloneMode(t *testing.T) {
	tests := []struct {
		name string
	}{
		{"PlainDownloadedRepository"},
		{"UnresolvableCwd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := t.TempDir()
			stateHome := t.TempDir()
			t.Setenv("XDG_STATE_HOME", stateHome)
			t.Setenv("LOCALAPPDATA", t.TempDir())
			t.Cleanup(func() { logger.SetDurableSinkDir("") })
			seedStandalonePlanDir(t, filepath.Join(stateHome, "lyx", hash8For(t, target), "_lyx", "plan"))

			c := &websterCLI{}
			// loc is nil regardless of which real ResolveMode cause produced ModeStandalone: both
			// carry a nil Location, and wire consults mode alone to choose the dispatch branch.
			if err := c.wire(nil, preflight.ModeStandalone, target, "", "", ""); err != nil {
				t.Fatalf("wire() = %v; want nil", err)
			}

			if c.geom.WorktreeRoot != target {
				t.Errorf("geom.WorktreeRoot = %q; want %q (the target)", c.geom.WorktreeRoot, target)
			}
			if c.anchorRel != "" {
				t.Errorf("anchorRel = %q; want empty in standalone", c.anchorRel)
			}
			if _, ok := c.refMatcher.(websterengine.NeverMatches); !ok {
				t.Errorf("refMatcher = %T; want websterengine.NeverMatches in standalone", c.refMatcher)
			}
			if c.openFabric != nil {
				t.Error("openFabric != nil; want nil in standalone -- there is no fabric repo to reach")
			}
		})
	}
}

// hash8For returns standalonestate.Derive's hash8 for target under the environment t.Setenv has
// already redirected -- the caller must have already called t.Setenv("XDG_STATE_HOME", ...) (or the
// per-OS equivalent) before calling this, exactly as it must before calling wire in standalone mode.
func hash8For(t *testing.T, target string) string {
	t.Helper()
	_, hash8, err := standalonestate.Derive(target)
	if err != nil {
		t.Fatalf("standalonestate.Derive(%q) = %v; want nil error", target, err)
	}
	return hash8
}

// TestWire_PlanDirResolution covers the plan directory's three resolutions -- the hub default, the
// standalone default, and an explicit --plan-dir override -- plus the standalone-only absent-plan-dir
// usage error naming --plan-dir.
func TestWire_PlanDirResolution(t *testing.T) {
	t.Run("HubDefault", func(t *testing.T) {
		hub := t.TempDir()
		loc := hubLocation(hub, "warp", ".")

		c := &websterCLI{}
		if err := c.wire(loc, preflight.ModeHub, "", "", "", ""); err != nil {
			t.Fatalf("wire() = %v; want nil", err)
		}
		if c.geom.PlanDir == "" {
			t.Fatal("geom.PlanDir is empty; want the hub default")
		}
	})

	t.Run("StandaloneDefault", func(t *testing.T) {
		target := t.TempDir()
		stateHome := t.TempDir()
		t.Setenv("XDG_STATE_HOME", stateHome)
		t.Setenv("LOCALAPPDATA", t.TempDir())
		t.Cleanup(func() { logger.SetDurableSinkDir("") })
		hash8 := hash8For(t, target)
		defaultPlanDir := filepath.Join(stateHome, "lyx", hash8, "_lyx", "plan")
		seedStandalonePlanDir(t, defaultPlanDir)

		c := &websterCLI{}
		if err := c.wire(nil, preflight.ModeStandalone, target, "", "", ""); err != nil {
			t.Fatalf("wire() = %v; want nil", err)
		}
		if c.geom.PlanDir != defaultPlanDir {
			t.Errorf("geom.PlanDir = %q; want the standalone default %q", c.geom.PlanDir, defaultPlanDir)
		}
	})

	t.Run("ExplicitOverride_HubMode", func(t *testing.T) {
		hub := t.TempDir()
		loc := hubLocation(hub, "warp", ".")
		override := filepath.Join(t.TempDir(), "custom-plan")

		c := &websterCLI{}
		if err := c.wire(loc, preflight.ModeHub, "", "", override, ""); err != nil {
			t.Fatalf("wire() = %v; want nil", err)
		}
		if c.geom.PlanDir != override {
			t.Errorf("geom.PlanDir = %q; want the override %q", c.geom.PlanDir, override)
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
	})

	t.Run("AbsentStandalonePlanDir_UsageError", func(t *testing.T) {
		target := t.TempDir()
		stateHome := t.TempDir()
		t.Setenv("XDG_STATE_HOME", stateHome)
		t.Setenv("LOCALAPPDATA", t.TempDir())
		t.Cleanup(func() { logger.SetDurableSinkDir("") })
		// Deliberately never seed the default plan dir: it stays absent.

		c := &websterCLI{}
		err := c.wire(nil, preflight.ModeStandalone, target, "", "", "")
		if err == nil {
			t.Fatal("wire() error = nil; want a usage error naming --plan-dir")
		}
		if !strings.Contains(err.Error(), "--plan-dir") {
			t.Errorf("wire() error = %v; want it to name --plan-dir", err)
		}
	})

	t.Run("AbsentStandalonePlanDir_HubModeUnaffected", func(t *testing.T) {
		// Hub mode's own behaviour is unchanged: no new gate, no new error, even though this
		// fictional hub location's plan directory does not exist on disk either.
		hub := t.TempDir()
		loc := hubLocation(hub, "warp", ".")

		c := &websterCLI{}
		if err := c.wire(loc, preflight.ModeHub, "", "", "", ""); err != nil {
			t.Fatalf("wire() = %v; want nil -- hub mode must not gate on plan-dir presence", err)
		}
	})
}

// TestWire_RelativeFlagDirsResolveAgainstCwd is R4-23's direct regression test. --plan-dir and
// --stencils-dir used to be stored verbatim, so a relative value reached the geometry unresolved: the
// CLI process would read it against ITS working directory while the pane webster spawns runs at the
// target (standalone) or the anchor (hub), and standalone's default-vs-override path equality could
// never match a relative spelling of the default.
//
// The standalone row proves both halves at once: it passes the default plan directory spelled
// RELATIVE to cwd, and asserts both that geom.PlanDir came out at the absolute default and that the
// run-refusal marker stayed clear -- pre-fix that same call marked the plan as moved and `run`
// refused to spawn Master over a plan sitting in the default location.
func TestWire_RelativeFlagDirsResolveAgainstCwd(t *testing.T) {
	t.Run("HubMode", func(t *testing.T) {
		hub := t.TempDir()
		loc := hubLocation(hub, "warp", ".")
		cwd := t.TempDir()

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
		defaultPlanDir := filepath.Join(stateHome, "lyx", hash8, "_lyx", "plan")
		seedStandalonePlanDir(t, defaultPlanDir)

		// cwd is the state home itself, so the default plan directory has a short relative spelling
		// from it -- exactly the shape an operator types.
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
		if c.standalonePlanDirOverridden {
			t.Error("standalonePlanDirOverridden = true; want false -- a relative spelling of the DEFAULT plan directory has not moved the plan, and run must not refuse to spawn Master over it")
		}
	})
}

// TestWire_TargetDirRefusedInHubMode proves --target-dir is refused in hub mode with an error
// explaining why, and that the refusal happens before any config is loaded (an empty hub, no
// module config on disk anywhere, still refuses on the flag alone).
func TestWire_TargetDirRefusedInHubMode(t *testing.T) {
	hub := t.TempDir()
	loc := hubLocation(hub, "warp", ".")

	c := &websterCLI{}
	err := c.wire(loc, preflight.ModeHub, "", "", "", filepath.Join(t.TempDir(), "elsewhere"))
	if err == nil {
		t.Fatal("wire() error = nil; want a refusal naming --target-dir")
	}
	if !strings.Contains(err.Error(), "--target-dir") {
		t.Errorf("wire() error = %v; want it to name --target-dir", err)
	}
}

// TestWire_StandaloneRootsResolveToTarget proves the two-roots split standalone mode exists for: the
// worktree root (also the fork-audit workdir and the {{.worktree_root}} prompt token, per
// websterengine.Geometry's own doc) resolves to target, while every _lyx/.lyx path and every module
// config base resolves under the derived state directory -- never target itself.
func TestWire_StandaloneRootsResolveToTarget(t *testing.T) {
	target := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateHome)
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Cleanup(func() { logger.SetDurableSinkDir("") })
	hash8 := hash8For(t, target)
	stateDir := filepath.Join(stateHome, "lyx", hash8)
	seedStandalonePlanDir(t, filepath.Join(stateDir, "_lyx", "plan"))

	c := &websterCLI{}
	if err := c.wire(nil, preflight.ModeStandalone, target, "", "", ""); err != nil {
		t.Fatalf("wire() = %v; want nil", err)
	}

	if c.geom.WorktreeRoot != target {
		t.Errorf("geom.WorktreeRoot = %q; want the target %q (also the audit workdir and the prompt worktree-root token)", c.geom.WorktreeRoot, target)
	}
	if c.geom.AnchorRoot != stateDir {
		t.Errorf("geom.AnchorRoot = %q; want the derived state directory %q", c.geom.AnchorRoot, stateDir)
	}
	for _, tt := range []struct {
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
		if !strings.HasPrefix(tt.got, stateDir) {
			t.Errorf("geom.%s = %q; want it to resolve under the derived state directory %q, never under the target", tt.name, tt.got, stateDir)
		}
	}
}

// TestWire_MatcherNeverNilOpenerNilOnlyInStandalone pins the two seams that must never be
// nil-or-eager: the RefMatcher is non-nil in both modes (a nil interface would panic the first time
// CheckFork/CheckParent calls Matches unguarded), and the fabric opener is nil in standalone (there is
// no fabric repo to reach) and non-nil, but never invoked by wire itself, in hub mode -- the latter
// half is covered by TestWire_ModeHubSelectsHubMode's explicit per-case check.
func TestWire_MatcherNeverNilOpenerNilOnlyInStandalone(t *testing.T) {
	t.Run("HubMode", func(t *testing.T) {
		hub := t.TempDir()
		loc := hubLocation(hub, "warp", ".")

		c := &websterCLI{}
		if err := c.wire(loc, preflight.ModeHub, "", "", "", ""); err != nil {
			t.Fatalf("wire() = %v; want nil", err)
		}
		if c.refMatcher == nil {
			t.Error("refMatcher = nil; want non-nil in hub mode")
		}
		if c.openFabric == nil {
			t.Error("openFabric = nil; want a wired (if uninvoked) opener in hub mode")
		}
	})

	t.Run("StandaloneMode", func(t *testing.T) {
		target := t.TempDir()
		stateHome := t.TempDir()
		t.Setenv("XDG_STATE_HOME", stateHome)
		t.Setenv("LOCALAPPDATA", t.TempDir())
		t.Cleanup(func() { logger.SetDurableSinkDir("") })
		hash8 := hash8For(t, target)
		seedStandalonePlanDir(t, filepath.Join(stateHome, "lyx", hash8, "_lyx", "plan"))

		c := &websterCLI{}
		if err := c.wire(nil, preflight.ModeStandalone, target, "", "", ""); err != nil {
			t.Fatalf("wire() = %v; want nil", err)
		}
		if c.refMatcher == nil {
			t.Error("refMatcher = nil; want non-nil in standalone mode too")
		}
		if c.openFabric != nil {
			t.Error("openFabric != nil; want nil in standalone mode")
		}
	})
}

// TestWireStandalone_RunnerReachesPublicEntryPointWithoutToldPathError is F16's direct regression
// test. It fails against pre-fix source, where wireStandalone constructed its runner via
// shuttleengine.NewRunner: NewRunner's containment assertion refuses standalone's deliberately
// detached anchor/worktree-root pair (the derived state directory sits outside the target
// repository), setting the runner's held toldErr, which every public entry point returns
// immediately without ever reaching reed. A runner that is merely non-nil proves nothing here --
// NewRunner and NewDetachedRunner both always return a non-nil *shuttleengine.Runner and hold
// their verdict on toldErr, surfacing it only when a verb runs -- so this test drives a public
// entry point (Interrupt, with a guid no strand will ever match) and asserts the returned error is
// the ordinary "not a shuttle strand" verdict rather than a told-path refusal.
func TestWireStandalone_RunnerReachesPublicEntryPointWithoutToldPathError(t *testing.T) {
	target := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateHome)
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Cleanup(func() { logger.SetDurableSinkDir("") })
	hash8 := hash8For(t, target)
	seedStandalonePlanDir(t, filepath.Join(stateHome, "lyx", hash8, "_lyx", "plan"))

	c := &websterCLI{}
	if err := c.wire(nil, preflight.ModeStandalone, target, "", "", ""); err != nil {
		t.Fatalf("wire() = %v; want nil", err)
	}

	err := c.runner.Interrupt("no-such-strand-guid")
	if err == nil {
		t.Fatal("runner.Interrupt() error = nil; want \"not a shuttle strand\", since no such strand exists")
	}
	if strings.Contains(err.Error(), "NewRunner") || strings.Contains(err.Error(), "NewDetachedRunner") {
		t.Fatalf("runner.Interrupt() error = %v; want the ordinary \"not a shuttle strand\" verdict, not a told-path refusal -- this is exactly the error NewRunner's containment assertion would have produced against standalone's detached anchor/worktree-root pair", err)
	}
	if !strings.Contains(err.Error(), "not a shuttle strand") {
		t.Errorf("runner.Interrupt() error = %v; want it to name \"not a shuttle strand\"", err)
	}
}

// TestWireStandalone_RedirectsDurableSinkToStandaloneLogsDir is F22's direct regression test. It
// observes the sink directory the only way this package can: by forcing a write and checking the
// filesystem, never by reading internal/logger's own state (sinkDirOverride is unexported and
// internal/logger exposes no accessor). The mechanism is that a non-empty override bypasses the
// testing.Testing() sink suppression in ensureDurableSink, so one logger.Info call after
// wireStandalone returns arms the sink at whatever directory the override names, with no
// LYX_TRACE redirect needed.
func TestWireStandalone_RedirectsDurableSinkToStandaloneLogsDir(t *testing.T) {
	target := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateHome)
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Cleanup(func() { logger.SetDurableSinkDir("") })
	stateDir := filepath.Join(stateHome, "lyx", hash8For(t, target))
	seedStandalonePlanDir(t, filepath.Join(stateDir, "_lyx", "plan"))

	c := &websterCLI{}
	if err := c.wire(nil, preflight.ModeStandalone, target, "", "", ""); err != nil {
		t.Fatalf("wire() = %v; want nil", err)
	}

	logger.Info("wiring_test: arm the sink")

	wantDir := standalonegeom.LogsDir(stateDir)
	matches, err := filepath.Glob(filepath.Join(wantDir, "trace-*.log"))
	if err != nil {
		t.Fatalf("glob %s: %v", wantDir, err)
	}
	if len(matches) == 0 {
		t.Errorf("no trace-*.log file under %s; want wireStandalone to have redirected the durable sink there", wantDir)
	}
}

// TestWireHub_LeavesDurableSinkDirUntouched guards against a later refactor quietly routing hub
// mode through the standalone sink redirect. It sets a sentinel override before calling wireHub,
// then asserts the sink still writes to that sentinel afterward -- a wireHub that had overwritten
// the override would have put the trace file somewhere else.
func TestWireHub_LeavesDurableSinkDirUntouched(t *testing.T) {
	sentinelDir := t.TempDir()
	logger.SetDurableSinkDir(sentinelDir)
	t.Cleanup(func() { logger.SetDurableSinkDir("") })

	hub := t.TempDir()
	loc := hubLocation(hub, "warp", ".")

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

// seedGitRepositoryRoot marks dir as a git repository root by creating the ".git" entry
// repositoryRootOf looks for, and returns dir. It writes no git objects and spawns no git: the walk
// this fixture feeds tests only the marker's presence, so a real repository would prove nothing extra
// and would breach the Test Tier Purity Invariant to build.
func seedGitRepositoryRoot(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir %s/.git: %v", dir, err)
	}
	return dir
}

// TestResolveStandaloneTarget_LiftsToRepositoryRoot is R4-26's direct regression test.
// preflight.ResolveMode answers ModeStandalone for a plain repository's SUBDIRECTORY too, so the
// target used to differ by where the operator happened to stand: repo/ and repo/src/ hashed to two
// different hash8 values and so derived two state directories, two reed sessions and two plan
// directories for one repository -- the second of which webster then refused outright with
// "standalone plan directory ... does not exist".
func TestResolveStandaloneTarget_LiftsToRepositoryRoot(t *testing.T) {
	t.Run("CwdInsideRepositoryLiftsToRoot", func(t *testing.T) {
		t.Parallel()
		repoRoot := seedGitRepositoryRoot(t, t.TempDir())
		subDir := filepath.Join(repoRoot, "src", "inner")
		if err := os.MkdirAll(subDir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", subDir, err)
		}

		got, err := resolveStandaloneTarget(subDir, "")
		if err != nil {
			t.Fatalf("resolveStandaloneTarget() = %v; want nil error", err)
		}
		if want := standalonestate.Normalize(repoRoot); got != want {
			t.Errorf("resolveStandaloneTarget(%q, \"\") = %q; want the repository root %q", subDir, got, want)
		}
	})

	t.Run("TargetDirInsideRepositoryLiftsToRoot", func(t *testing.T) {
		t.Parallel()
		repoRoot := seedGitRepositoryRoot(t, t.TempDir())
		if err := os.MkdirAll(filepath.Join(repoRoot, "src"), 0o755); err != nil {
			t.Fatalf("mkdir src: %v", err)
		}

		got, err := resolveStandaloneTarget(repoRoot, "src")
		if err != nil {
			t.Fatalf("resolveStandaloneTarget() = %v; want nil error", err)
		}
		if want := standalonestate.Normalize(repoRoot); got != want {
			t.Errorf("resolveStandaloneTarget(%q, \"src\") = %q; want the repository root %q", repoRoot, got, want)
		}
	})

	t.Run("NonRepositoryDirectoryUnchanged", func(t *testing.T) {
		t.Parallel()
		// standalone mode legitimately covers a plain directory that is no git repository at all --
		// preflight.ResolveMode folds that cause into the very same verdict -- so the lift must not
		// invent a root by walking to the filesystem's own top.
		plain := t.TempDir()

		got, err := resolveStandaloneTarget(plain, "")
		if err != nil {
			t.Fatalf("resolveStandaloneTarget() = %v; want nil error", err)
		}
		if want := standalonestate.Normalize(plain); got != want {
			t.Errorf("resolveStandaloneTarget(%q, \"\") = %q; want it unchanged at %q", plain, got, want)
		}
	})
}

// TestWireStandalone_SubdirectoryOfRepositoryWiresLikeItsRoot is R4-26's end-to-end half: wiring from
// a repository subdirectory must land on the SAME derived state directory, and therefore the same
// default plan directory, that wiring from the repository root lands on. Pre-fix this call failed
// with webster's own "standalone plan directory ... does not exist" refusal, because the
// subdirectory's own hash8 named a state directory nobody had ever seeded.
func TestWireStandalone_SubdirectoryOfRepositoryWiresLikeItsRoot(t *testing.T) {
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
	rootStateDir := filepath.Join(stateHome, "lyx", hash8For(t, normalizedRoot))
	seedStandalonePlanDir(t, filepath.Join(rootStateDir, "_lyx", "plan"))

	c := &websterCLI{}
	if err := c.wire(nil, preflight.ModeStandalone, subDir, "", "", ""); err != nil {
		t.Fatalf("wire() from a repository subdirectory = %v; want nil -- where the operator stands inside a repository must not change which repository webster drives", err)
	}
	if c.geom.WorktreeRoot != normalizedRoot {
		t.Errorf("geom.WorktreeRoot = %q; want the repository root %q", c.geom.WorktreeRoot, normalizedRoot)
	}
	if want := filepath.Join(rootStateDir, "_lyx", "plan"); c.geom.PlanDir != want {
		t.Errorf("geom.PlanDir = %q; want the root's own default plan directory %q", c.geom.PlanDir, want)
	}
}

// TestWireStandalone_RefusesStateDirNestedInTarget is R4-25's direct regression test. A standalone
// target that CONTAINS its own derived state directory -- a dotfiles repository rooted at the home
// directory, or an XDG_STATE_HOME pointed somewhere inside the checkout -- used to reach
// shuttleengine.NewDetachedRunner's containment assertion, which fires only once a tmux server has
// been booted and the run lock taken, blames a hub geometry that was never involved, and names no
// lever the operator can pull.
//
// The sentinel-sink assertion is what proves the refusal lands EARLY: wireStandalone redirects the
// durable trace sink to a directory under stateDir, which in this geometry sits inside the
// operator's own repository, so a trace file appearing anywhere but the sentinel would mean the
// guard ran after that redirect.
func TestWireStandalone_RefusesStateDirNestedInTarget(t *testing.T) {
	target := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(target, ".local", "state"))
	t.Setenv("LOCALAPPDATA", filepath.Join(target, "AppData", "Local"))

	sentinelDir := t.TempDir()
	logger.SetDurableSinkDir(sentinelDir)
	t.Cleanup(func() { logger.SetDurableSinkDir("") })

	c := &websterCLI{}
	err := c.wire(nil, preflight.ModeStandalone, target, "", "", "")
	if err == nil {
		t.Fatal("wire() error = nil; want a refusal naming the nested state directory")
	}
	if !strings.Contains(err.Error(), "XDG_STATE_HOME") {
		t.Errorf("wire() error = %v; want it to name XDG_STATE_HOME, the only lever an operator has here", err)
	}
	if !strings.Contains(err.Error(), target) {
		t.Errorf("wire() error = %v; want it to name the target %q", err, target)
	}
	if strings.Contains(err.Error(), "hub geometry") {
		t.Errorf("wire() error = %v; want the real cause, not NewDetachedRunner's hub-geometry guess", err)
	}
	if strings.Contains(err.Error(), "--plan-dir") {
		t.Errorf("wire() error = %v; want the nesting refusal to win over the later plan-dir gate", err)
	}

	logger.Info("wiring_test: arm the sink")
	matches, err := filepath.Glob(filepath.Join(sentinelDir, "trace-*.log"))
	if err != nil {
		t.Fatalf("glob %s: %v", sentinelDir, err)
	}
	if len(matches) == 0 {
		t.Error("no trace-*.log file under the sentinel dir; want the nesting refusal to fire before wireStandalone redirects the durable sink into the operator's own repository")
	}
}

// TestWire_ReedUpSeamPerMode is F-A1's (round fable5-high-r3) wiring pin: wireStandalone must arm
// the in-process reed bring-up seam the run verb fires before spawning Master (standalone's derived
// geometry is reachable by no CLI verb — `lyx reed up` is hub-only), and wireHub must leave it nil,
// keeping hub mode's session lifecycle the operator's (or loom's) own.
func TestWire_ReedUpSeamPerMode(t *testing.T) {
	t.Run("StandaloneArmsTheSeam", func(t *testing.T) {
		target := t.TempDir()
		stateHome := t.TempDir()
		t.Setenv("XDG_STATE_HOME", stateHome)
		t.Setenv("LOCALAPPDATA", t.TempDir())
		t.Cleanup(func() { logger.SetDurableSinkDir("") })
		seedStandalonePlanDir(t, filepath.Join(stateHome, "lyx", hash8For(t, target), "_lyx", "plan"))

		c := &websterCLI{}
		if err := c.wire(nil, preflight.ModeStandalone, target, "", "", ""); err != nil {
			t.Fatalf("wire() = %v; want nil", err)
		}
		if c.reedUp == nil {
			t.Error("wireStandalone left c.reedUp nil; want the in-process reed bring-up seam armed")
		}
	})

	t.Run("HubLeavesTheSeamNil", func(t *testing.T) {
		hub := t.TempDir()
		loc := hubLocation(hub, "warp", ".")

		c := &websterCLI{}
		if err := c.wire(loc, preflight.ModeHub, "", "", "", ""); err != nil {
			t.Fatalf("wire() = %v; want nil", err)
		}
		if c.reedUp != nil {
			t.Error("wireHub armed c.reedUp; want nil — hub mode's reed session is not run's to boot")
		}
	})
}

// TestWireStandalone_PlanDirOverrideMarksRunRefusal is F-A3's (round fable5-high-r3) wiring pin: a
// --plan-dir that moves the plan off standalone's default marks the CLI so the run verb refuses to
// spawn Master (whose flagless in-pane verbs resolve the default and could never see the moved
// plan), while the default location leaves the mark clear.
func TestWireStandalone_PlanDirOverrideMarksRunRefusal(t *testing.T) {
	t.Run("OverrideMarks", func(t *testing.T) {
		target := t.TempDir()
		t.Setenv("XDG_STATE_HOME", t.TempDir())
		t.Setenv("LOCALAPPDATA", t.TempDir())
		t.Cleanup(func() { logger.SetDurableSinkDir("") })
		override := t.TempDir()
		seedStandalonePlanDir(t, override)

		c := &websterCLI{}
		if err := c.wire(nil, preflight.ModeStandalone, target, "", override, ""); err != nil {
			t.Fatalf("wire() = %v; want nil", err)
		}
		if !c.standalonePlanDirOverridden {
			t.Error("standalonePlanDirOverridden = false; want true for a moved plan directory")
		}
		if c.standalonePlanDirDefault == "" || c.standalonePlanDirDefault == override {
			t.Errorf("standalonePlanDirDefault = %q; want the default location, distinct from the override", c.standalonePlanDirDefault)
		}
	})

	t.Run("DefaultLeavesMarkClear", func(t *testing.T) {
		target := t.TempDir()
		stateHome := t.TempDir()
		t.Setenv("XDG_STATE_HOME", stateHome)
		t.Setenv("LOCALAPPDATA", t.TempDir())
		t.Cleanup(func() { logger.SetDurableSinkDir("") })
		seedStandalonePlanDir(t, filepath.Join(stateHome, "lyx", hash8For(t, target), "_lyx", "plan"))

		c := &websterCLI{}
		if err := c.wire(nil, preflight.ModeStandalone, target, "", "", ""); err != nil {
			t.Fatalf("wire() = %v; want nil", err)
		}
		if c.standalonePlanDirOverridden {
			t.Error("standalonePlanDirOverridden = true; want false when the plan sits at the default")
		}
	})
}
