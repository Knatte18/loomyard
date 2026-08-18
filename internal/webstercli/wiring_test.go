// wiring_test.go pins wire's mode truth table at tier 1: every case drives c.wire directly with a
// told preflight.HubPresent result -- never through the real pre-run -- so no case spawns a process
// or resolves cwd. websterCLI itself carries no *lyxcwd.Location field (see cli.go's struct): that is
// a compile-time property of this package, not something a test can additionally assert at runtime.
//
// The hub-mode cases below hand wire fictional, on-disk-absent *lyxcwd.Location values: every module
// config load wire performs (shuttleengine.LoadConfig, reedengine.LoadConfig, websterengine.LoadConfig,
// batcher.Active, modelspec.LoadRegistry) degrades to its embedded template on a proven-absent _lyx/
// directory, so a fictional anchor path drives the whole hub branch without touching disk.
//
// The standalone-mode cases reach the one call site of standalonestate.Derive that exists anywhere in
// this codebase: each such case redirects XDG_STATE_HOME to a t.TempDir() BEFORE calling wire, and
// none of those cases is marked t.Parallel(), since t.Setenv panics under a parallel test.

package webstercli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
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

// TestWire_HubPresentTrueSelectsHubMode covers the three healthy-but-unwired locations that run
// webster verbs today under hub mode: an ordinary worktree standing in for <hub>/_board, an unpaired
// sibling, and a worktree whose paired sibling has been removed. A Wired-keyed implementation would
// silently send every one of these to standalone mode; none of them may be refused, and none of them
// may land in standalone.
func TestWire_HubPresentTrueSelectsHubMode(t *testing.T) {
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
			if err := c.wire(loc, true, "", "", "", ""); err != nil {
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

// TestWire_HubPresentFalseSelectsStandaloneMode covers both cases preflight.HubPresent folds into
// "false": a plain downloaded git repository (no hub-level directory beside it) and an unresolvable
// cwd (lyxcwd.Resolve itself failed). HubPresent returns a nil Location for each, and wire cannot and
// must not try to tell them apart -- both land in standalone mode.
func TestWire_HubPresentFalseSelectsStandaloneMode(t *testing.T) {
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
			seedStandalonePlanDir(t, filepath.Join(stateHome, "lyx", hash8For(t, target), "_lyx", "plan"))

			c := &websterCLI{}
			// loc is nil regardless of which real HubPresent branch produced the false: both
			// carry a nil Location, and wire consults hubPresent (the bool) alone to choose mode.
			if err := c.wire(nil, false, target, "", "", ""); err != nil {
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
		if err := c.wire(loc, true, "", "", "", ""); err != nil {
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
		hash8 := hash8For(t, target)
		defaultPlanDir := filepath.Join(stateHome, "lyx", hash8, "_lyx", "plan")
		seedStandalonePlanDir(t, defaultPlanDir)

		c := &websterCLI{}
		if err := c.wire(nil, false, target, "", "", ""); err != nil {
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
		if err := c.wire(loc, true, "", "", override, ""); err != nil {
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
		override := t.TempDir()
		seedStandalonePlanDir(t, override)

		c := &websterCLI{}
		if err := c.wire(nil, false, target, "", override, ""); err != nil {
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
		// Deliberately never seed the default plan dir: it stays absent.

		c := &websterCLI{}
		err := c.wire(nil, false, target, "", "", "")
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
		if err := c.wire(loc, true, "", "", "", ""); err != nil {
			t.Fatalf("wire() = %v; want nil -- hub mode must not gate on plan-dir presence", err)
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
	err := c.wire(loc, true, "", "", "", filepath.Join(t.TempDir(), "elsewhere"))
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
	hash8 := hash8For(t, target)
	stateDir := filepath.Join(stateHome, "lyx", hash8)
	seedStandalonePlanDir(t, filepath.Join(stateDir, "_lyx", "plan"))

	c := &websterCLI{}
	if err := c.wire(nil, false, target, "", "", ""); err != nil {
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
// half is covered by TestWire_HubPresentTrueSelectsHubMode's explicit per-case check.
func TestWire_MatcherNeverNilOpenerNilOnlyInStandalone(t *testing.T) {
	t.Run("HubMode", func(t *testing.T) {
		hub := t.TempDir()
		loc := hubLocation(hub, "warp", ".")

		c := &websterCLI{}
		if err := c.wire(loc, true, "", "", "", ""); err != nil {
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
		hash8 := hash8For(t, target)
		seedStandalonePlanDir(t, filepath.Join(stateHome, "lyx", hash8, "_lyx", "plan"))

		c := &websterCLI{}
		if err := c.wire(nil, false, target, "", "", ""); err != nil {
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
