// wiring_test.go pins wire's mode truth table at tier 0.
// Every case drives c.wire directly with a told preflight.Mode, never through the real pre-run,
// so no case spawns a process or resolves cwd.
//
// The hub-mode cases hand wire a fictional, on-disk-absent *lymcwd.Location.
// Every module config load wire performs degrades to its embedded template on a proven-absent _lym/ directory:
// shuttleengine.LoadConfig, lenses.Load, and reedengine.LoadConfig all do so.
// A fictional anchor path therefore drives the whole hub branch without touching disk.
// The one entry each hub case creates is the worktree's .git directory;
// hubgeom.ReedGeometry stats it to tell the prime from a task worktree.
//
// The standalone-mode cases reach standalonestate.Derive through internal/cliwire's ResolveStandalone.
// Each such case redirects both XDG_STATE_HOME and LOCALAPPDATA to a t.TempDir() before calling wire,
// so both of Derive's per-OS branches land inside the test's own temp tree on every platform.
// None of those cases is marked t.Parallel(), since t.Setenv panics under a parallel test.
//
// Two structural facts a later reader might otherwise try to "fix".
// First, no (loc non-nil, ModeStandalone) row exists in this file, because no caller can produce one:
// preflight.ResolveMode returns a nil Location for both standalone causes:
// a plain downloaded git repo, and a directory outside any repository.
// Second, the refuse case is absent from this file, because wire never receives it:
// the error return aborts upstream in resolvePersistentPreRun before c.wire is called.
// Manufacturing one here would require driving the real pre-run;
// that spawns git and breaches the Test Tier Purity Invariant, which wire's extraction exists to satisfy.
// The refusal is pinned instead in internal/preflight's own integration table and in
// internal/burlercli/cli_integration_test.go.
//
// This file does not assert the reed geometry's field values.
// burlerengine.Engine exposes only Run and holds its geom, cfg, and stencilsDir unexported;
// shuttleengine.Runner holds reed, engine, anchorPath, worktreeRoot, and cfg unexported with no geometry accessor.
// Both live in other packages, so SocketKey, SessionName, LogsDir and HubPath are unreachable from this test.
// None of the three reporting fields (c.mode, c.stateDir, c.stencilsDir) encodes them.
// TestReedGeometry in internal/standalonegeom/standalonegeom_test.go pins those values one layer down.
// It asserts the geometry's fields against told parameters.
// Left uncovered is the linkage:
// wireStandalone calls standalonegeom.ReedGeometry with this invocation's own target, stateDir, and hash8.
// No seam exposes it, so it is a review obligation, recorded here so the gap is not mistaken for an oversight.

package burlercli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/cliwire"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lenses"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/preflight"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/standalonegeom"
	"github.com/Knatte18/loomyard/internal/standalonestate"
	"github.com/Knatte18/loomyard/internal/testkit/shuttlefake"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// hubLocation returns a *lymcwd.Location standing in for a real hub's prime.
// It creates only the worktree's .git directory; wire's hub branch tolerates the rest being absent.
func hubLocation(t *testing.T, hub, worktreeName, anchorRel string) *lymcwd.Location {
	t.Helper()
	loc := &lymcwd.Location{HubPath: hub, WorktreeName: worktreeName, AnchorRel: anchorRel}
	if err := os.MkdirAll(filepath.Join(loc.WorktreePath(), ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.git) = %v; want nil", err)
	}
	return loc
}

// setStandaloneStateRoot points the environment variables standalonestate.Derive reads at fresh t.TempDir() values,
// so a case that reaches wireStandalone stays hermetic.
// It must not run under t.Parallel, since t.Setenv panics there.
// It also registers a cleanup that clears the durable sink override:
// wireStandalone sets that process-global override,
// and a leaked one would defeat the testing.Testing() sink suppression for every later test in the binary.
func setStandaloneStateRoot(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Cleanup(func() { logger.SetDurableSinkDir("") })
}

// stateDirFor derives the state directory for target under the environment t.Setenv has already redirected.
func stateDirFor(t *testing.T, target string) string {
	t.Helper()
	stateDir, _, err := standalonestate.Derive(target)
	if err != nil {
		t.Fatalf("standalonestate.Derive(%q) = %v; want nil error", target, err)
	}
	return stateDir
}

// TestWire_SelectsModeAndWiresItsFields covers wire's dispatch on the told mode.
// Hub mode (loc non-nil) reports mode "hub", an empty stateDir and the hub stencils directory.
// It leaves the reed bring-up seam nil, since hub mode's reed session is not run's to boot.
// Standalone mode (loc nil) reports mode "standalone" with the derived state directory and its stencils directory.
// It also arms the in-process reed bring-up seam.
// A nil loc covers both a plain downloaded repository and a non-repository directory, which wire cannot tell apart.
// The standalone subtest sets the process environment, so the test is not t.Parallel;
// the hub subtest touches no global state and runs in parallel.
//
//testtiming:keep pins the mode, stateDir, stencilsDir and reed-seam values per mode, which the stencils-dir and runner tests covering its blocks do not assert
func TestWire_SelectsModeAndWiresItsFields(t *testing.T) {
	tiergate.Require(t, 0)
	t.Run("Hub", func(t *testing.T) {
		t.Parallel()
		hub := t.TempDir()
		loc := hubLocation(t, hub, "code", ".")

		c := &burlerCLI{}
		if err := c.wire(loc, preflight.ModeHub, "", "", ""); err != nil {
			t.Fatalf("wire() = %v; want nil", err)
		}

		if c.mode != "hub" {
			t.Errorf("c.mode = %q; want %q", c.mode, "hub")
		}
		if c.stateDir != "" {
			t.Errorf("c.stateDir = %q; want empty in hub mode", c.stateDir)
		}
		if want := fabricengine.StencilsDir(loc.HubPath); c.stencilsDir != want {
			t.Errorf("c.stencilsDir = %q; want %q", c.stencilsDir, want)
		}
		if c.engine == nil {
			t.Fatal("c.engine = nil; want a constructed *burlerengine.Engine")
		}
		if c.reedUp != nil {
			t.Error("wireHub armed c.reedUp; want nil -- hub mode's reed session is not run's to boot")
		}
	})

	writeHubLanguageFile := func(t *testing.T, hub, contents string) {
		t.Helper()
		configDir := filepath.Join(fabricengine.BoardDir(hub), "_lym", "config")
		if err := os.MkdirAll(configDir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) = %v; want nil", configDir, err)
		}
		if err := os.WriteFile(filepath.Join(configDir, "language.yaml"), []byte(contents), 0o644); err != nil {
			t.Fatalf("WriteFile(language.yaml) = %v; want nil", err)
		}
	}

	t.Run("HubWithLanguageList", func(t *testing.T) {
		t.Parallel()
		hub := t.TempDir()
		loc := hubLocation(t, hub, "code", ".")
		writeHubLanguageFile(t, hub, "skills:\n  - scribe:golang-testing\n")

		c := &burlerCLI{}
		if err := c.wire(loc, preflight.ModeHub, "", "", ""); err != nil {
			t.Fatalf("wire() = %v; want nil", err)
		}

		if want := []string{"scribe:golang-testing"}; !slices.Equal(c.languageSkills, want) {
			t.Errorf("c.languageSkills = %v; want %v", c.languageSkills, want)
		}
	})

	t.Run("HubWithInvalidLanguageList", func(t *testing.T) {
		t.Parallel()
		hub := t.TempDir()
		loc := hubLocation(t, hub, "code", ".")
		writeHubLanguageFile(t, hub, "skills:\n  - \"\"\n")

		c := &burlerCLI{}
		err := c.wire(loc, preflight.ModeHub, "", "", "")

		if err == nil {
			t.Fatal("wire() = nil; want a refusal of the empty skills entry")
		}
		for _, want := range []string{"skills", "lym config language"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("wire() error = %q; want it to contain %q", err, want)
			}
		}
	})

	t.Run("Standalone", func(t *testing.T) {
		target := t.TempDir()
		setStandaloneStateRoot(t)

		c := &burlerCLI{}
		if err := c.wire(nil, preflight.ModeStandalone, target, "", ""); err != nil {
			t.Fatalf("wire() = %v; want nil", err)
		}

		if c.mode != "standalone" {
			t.Errorf("c.mode = %q; want %q", c.mode, "standalone")
		}
		stateDir := stateDirFor(t, target)
		if c.stateDir != stateDir {
			t.Errorf("c.stateDir = %q; want %q", c.stateDir, stateDir)
		}
		if want := standalonegeom.StencilsDir(stateDir); c.stencilsDir != want {
			t.Errorf("c.stencilsDir = %q; want %q", c.stencilsDir, want)
		}
		if c.engine == nil {
			t.Fatal("c.engine = nil; want a constructed *burlerengine.Engine")
		}
		if c.reedUp == nil {
			t.Error("wireStandalone left c.reedUp nil; want the in-process reed bring-up seam armed")
		}
		if len(c.languageSkills) != 0 {
			t.Errorf("c.languageSkills = %v; want empty in standalone, which loads no language module", c.languageSkills)
		}
	})
}

// TestWire_TargetDirRefusedInHubMode proves that --target-dir is refused in hub mode, with an error naming the flag.
// The refusal happens before any config loads, so it holds on an empty hub with no module config on disk.
func TestWire_TargetDirRefusedInHubMode(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	hub := t.TempDir()
	loc := hubLocation(t, hub, "code", ".")

	c := &burlerCLI{}
	err := c.wire(loc, preflight.ModeHub, "", "", filepath.Join(t.TempDir(), "elsewhere"))
	if err == nil {
		t.Fatal("wire() error = nil; want a refusal naming --target-dir")
	}
	if !strings.Contains(err.Error(), "--target-dir") {
		t.Errorf("wire() error = %v; want it to name --target-dir", err)
	}
}

// TestWire_StencilsDirFlag covers --stencils-dir in both modes.
// An absolute value is honoured as given, and a relative value resolves against cwd.
// Stored as an absolute path, one string names one directory for the CLI and for the pane burler.
// The told directory must exist on disk, since wiring stats it at the boundary.
// The standalone subtests set the process environment and so are not t.Parallel.
func TestWire_StencilsDirFlag(t *testing.T) {
	tiergate.Require(t, 0)
	t.Run("AbsoluteHonouredInHubMode", func(t *testing.T) {
		t.Parallel()
		hub := t.TempDir()
		loc := hubLocation(t, hub, "code", ".")
		override := t.TempDir()

		c := &burlerCLI{}
		if err := c.wire(loc, preflight.ModeHub, "", override, ""); err != nil {
			t.Fatalf("wire() = %v; want nil", err)
		}
		if c.stencilsDir != override {
			t.Errorf("c.stencilsDir = %q; want the override %q", c.stencilsDir, override)
		}
	})

	t.Run("AbsoluteHonouredInStandaloneMode", func(t *testing.T) {
		target := t.TempDir()
		setStandaloneStateRoot(t)
		override := t.TempDir()

		c := &burlerCLI{}
		if err := c.wire(nil, preflight.ModeStandalone, target, override, ""); err != nil {
			t.Fatalf("wire() = %v; want nil", err)
		}
		if c.stencilsDir != override {
			t.Errorf("c.stencilsDir = %q; want the override %q", c.stencilsDir, override)
		}
	})

	t.Run("RelativeResolvesAgainstCwdInHubMode", func(t *testing.T) {
		t.Parallel()
		hub := t.TempDir()
		loc := hubLocation(t, hub, "code", ".")
		cwd := t.TempDir()
		if err := os.MkdirAll(filepath.Join(cwd, "custom", "stencils"), 0o755); err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}

		c := &burlerCLI{}
		if err := c.wire(loc, preflight.ModeHub, cwd, filepath.Join("custom", "stencils"), ""); err != nil {
			t.Fatalf("wire() = %v; want nil", err)
		}
		if want := filepath.Join(cwd, "custom", "stencils"); c.stencilsDir != want {
			t.Errorf("c.stencilsDir = %q; want the relative --stencils-dir resolved against cwd, %q", c.stencilsDir, want)
		}
	})

	t.Run("RelativeResolvesAgainstCwdInStandaloneMode", func(t *testing.T) {
		target := t.TempDir()
		setStandaloneStateRoot(t)
		cwd := t.TempDir()
		if err := os.MkdirAll(filepath.Join(cwd, "custom", "stencils"), 0o755); err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}

		c := &burlerCLI{}
		if err := c.wire(nil, preflight.ModeStandalone, cwd, filepath.Join("custom", "stencils"), target); err != nil {
			t.Fatalf("wire() = %v; want nil", err)
		}
		if want := filepath.Join(cwd, "custom", "stencils"); c.stencilsDir != want {
			t.Errorf("c.stencilsDir = %q; want the relative --stencils-dir resolved against cwd, %q", c.stencilsDir, want)
		}
	})
}

// TestWireHub_AbsentStencilsDirIsRefused proves that hub mode refuses a --stencils-dir that does not exist.
// Without this check, a typo'd directory fails only at the first instruction render.
// That happens after the run lock and substrate boot.
// The boundary stat goes through the same cliwire.Module method standalone's prologue uses,
// so the two modes cannot drift apart on it.
func TestWireHub_AbsentStencilsDirIsRefused(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	hub := t.TempDir()
	loc := hubLocation(t, hub, "code", ".")
	told := filepath.Join(t.TempDir(), "no-such-stencils")

	c := &burlerCLI{}
	err := c.wire(loc, preflight.ModeHub, "", told, "")
	if err == nil {
		t.Fatal("wire() = nil; want the unreadable --stencils-dir refusal")
	}
	if !strings.Contains(err.Error(), "--stencils-dir") || !strings.Contains(err.Error(), told) {
		t.Errorf("wire() error = %q; want it to name --stencils-dir and the told directory %q", err.Error(), told)
	}
}

// TestWireStandalone_RunnerReachesPublicEntryPointWithoutToldPathError checks a standalone runner reaches reed.
// NewRunner's containment assertion refuses the anchor and worktree-root pair that standalone detaches.
// Standalone detaches the pair because the derived state directory sits outside the target repository.
// The refusal sets the runner's held toldErr, which every public entry point returns at once, without reaching reed.
// A runner that is merely non-nil proves nothing.
// The test therefore drives c.engine.Review with a minimal validateReview-passing Profile,
// and it asserts the returned error is non-nil and is not a told-path refusal.
//
// AddStrand self-heals a cold worktree instead of failing fast, so a bare wire would spawn a real tmux server.
// The test points its configured multiplexer binary at a path that is never created.
// A first wire call resolves the standalone state directory onto the receiver and spawns nothing.
// wireStandalone only assigns its boot closure and never executes it.
// A reed config is then seeded under that state directory with the tmux key set to the missing path.
// The session probe of sessionSubstrateLocked then fails at process lookup, so nothing is spawned.
// A fresh CLI receiver wires again to pick up the seeded config and drives the same entry point.
// This keeps the tier0 test inside the Test Tier Purity Invariant.
func TestWireStandalone_RunnerReachesPublicEntryPointWithoutToldPathError(t *testing.T) {
	tiergate.Require(t, 0)
	target := t.TempDir()
	setStandaloneStateRoot(t)

	fixture := filepath.Join(target, "fixture.md")
	if err := os.WriteFile(fixture, []byte("placeholder"), 0o644); err != nil {
		t.Fatalf("write fixture file: %v", err)
	}

	// The first wire call only resolves and assigns the standalone state directory onto the receiver.
	// wireStandalone assigns its c.reedUp boot closure without executing it, so this call spawns nothing.
	primer := &burlerCLI{}
	if err := primer.wire(nil, preflight.ModeStandalone, target, "", ""); err != nil {
		t.Fatalf("wire() = %v; want nil", err)
	}

	// Seed a reed config under the state directory this invocation derived.
	// Its multiplexer key points at a path that is never created, so no process is ever spawned.
	// MkdirAll, not os.Mkdir: the first wire call has already created this state directory's _lym tree,
	// so a single-level Mkdir would fail before the config is written.
	configDir := configengine.ConfigDir(primer.stateDir)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", configDir, err)
	}
	noSuchTmux := filepath.Join(t.TempDir(), "no-such-tmux-binary")
	rawTemplate := reedengine.ConfigTemplate()
	lines := strings.SplitN(rawTemplate, "\n", 2)
	// The whole template is written, not a single-key fragment.
	// The degrading config loader resolves a present file rather than merging it over the template.
	seededConfig := fmt.Sprintf("tmux: %s\n%s", noSuchTmux, lines[1])
	configFile := configengine.ConfigFile(primer.stateDir, "reed")
	if err := os.WriteFile(configFile, []byte(seededConfig), 0o644); err != nil {
		t.Fatalf("write reed config: %v", err)
	}

	// Wire again on a fresh receiver so this second call's engine picks up the seeded config.
	c := &burlerCLI{}
	if err := c.wire(nil, preflight.ModeStandalone, target, "", ""); err != nil {
		t.Fatalf("wire() = %v; want nil", err)
	}

	profile := burlerengine.Profile{
		Target:          burlerengine.FileSet{Paths: []string{fixture}},
		Fasit:           burlerengine.FileSet{Paths: []string{fixture}},
		Rubric:          "placeholder rubric",
		FixScope:        burlerengine.FixScopeOverlay,
		ReviewPath:      "review.md",
		FixerReportPath: "fixer.md",
	}

	_, err := c.engine.Review(profile, burlerengine.RunOpts{})
	if err == nil {
		t.Fatal("engine.Review() error = nil; want a non-nil error, since the pinned-out tmux binary makes sessionSubstrateLocked's probe fail")
	}
	if strings.Contains(err.Error(), "NewRunner") || strings.Contains(err.Error(), "NewDetachedRunner") {
		t.Fatalf("engine.Review() error = %v; want the exec-lookup error from the pinned-out tmux binary, not a told-path refusal -- this is exactly the error NewRunner's containment assertion would have produced against standalone's detached anchor/worktree-root pair", err)
	}
}

// TestWireHub_LeavesDurableSinkDirUntouched guards against hub mode being routed through the standalone sink redirect.
// It sets a sentinel override before calling wireHub, then asserts the sink still writes to that sentinel.
// A wireHub that overwrote the override would send the trace file somewhere else.
//
//testtiming:keep pins that wireHub leaves the durable sink override untouched, which the mode test covering its blocks never reads back
func TestWireHub_LeavesDurableSinkDirUntouched(t *testing.T) {
	tiergate.Require(t, 0)
	sentinelDir := t.TempDir()
	logger.SetDurableSinkDir(sentinelDir)
	t.Cleanup(func() { logger.SetDurableSinkDir("") })

	hub := t.TempDir()
	loc := hubLocation(t, hub, "code", ".")

	c := &burlerCLI{}
	if err := c.wire(loc, preflight.ModeHub, "", "", ""); err != nil {
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
// It writes no git objects and spawns no git.
// A real repository would prove nothing extra, and building one would breach the Test Tier Purity Invariant.
func seedGitRepositoryRoot(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir %s/.git: %v", dir, err)
	}
	return dir
}

// TestWireStandalone_SubdirectoryOfRepositoryWiresLikeItsRoot checks wiring from a repository subdirectory.
// It must land on the same derived state directory that wiring from the repository root lands on.
func TestWireStandalone_SubdirectoryOfRepositoryWiresLikeItsRoot(t *testing.T) {
	tiergate.Require(t, 0)
	repoRoot := seedGitRepositoryRoot(t, t.TempDir())
	subDir := filepath.Join(repoRoot, "src")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	setStandaloneStateRoot(t)

	normalizedRoot := standalonestate.Normalize(repoRoot)
	rootStateDir := stateDirFor(t, normalizedRoot)

	c := &burlerCLI{}
	if err := c.wire(nil, preflight.ModeStandalone, subDir, "", ""); err != nil {
		t.Fatalf("wire() from a repository subdirectory = %v; want nil", err)
	}
	if c.stateDir != rootStateDir {
		t.Errorf("c.stateDir = %q; want the repository root's own state directory %q -- where the operator stands inside a repository must not change which repository burler reviews", c.stateDir, rootStateDir)
	}
}

// TestWireModule_DescriptorIsVerbatim pins burler's own wireModule descriptor field values.
// The shared wiring package asserts only its local fixtures, so this test is the check on burler's descriptor text.
func TestWireModule_DescriptorIsVerbatim(t *testing.T) {
	tiergate.Require(t, 0)
	if wireModule.Name != "burler" {
		t.Errorf("wireModule.Name = %q; want %q", wireModule.Name, "burler")
	}
	if want := "instruction files, shuttle run directories and trace logs"; wireModule.StateArtifacts != want {
		t.Errorf("wireModule.StateArtifacts = %q; want %q", wireModule.StateArtifacts, want)
	}
	if want := "the repository it reviews"; wireModule.TargetRole != want {
		t.Errorf("wireModule.TargetRole = %q; want %q", wireModule.TargetRole, want)
	}
	if want := "Review a target"; wireModule.TargetRecourse != want {
		t.Errorf("wireModule.TargetRecourse = %q; want %q", wireModule.TargetRecourse, want)
	}
	if want := "the anchor path is already the target"; wireModule.HubTargetSubject != want {
		t.Errorf("wireModule.HubTargetSubject = %q; want %q", wireModule.HubTargetSubject, want)
	}
	if wireModule.Plan != nil {
		t.Error("wireModule.Plan != nil; want nil -- burler parses no plan, and the nil is what makes ResolveStandalone skip plan resolution for it")
	}

	t.Run("RefuseTargetDirInHubMode", func(t *testing.T) {
		flag := filepath.Join(t.TempDir(), "elsewhere")
		err := wireModule.RefuseTargetDirInHubMode(flag)
		if err == nil {
			t.Fatal("RefuseTargetDirInHubMode() = nil; want a refusal for a non-empty flag")
		}
		want := "burler: --target-dir is not honoured in hub mode: the anchor path is already the target, and honouring any other value would strand its artifacts outside fabric's positive-only commit pathspec"
		if err.Error() != want {
			t.Errorf("RefuseTargetDirInHubMode() error = %q; want %q", err.Error(), want)
		}
		if err := wireModule.RefuseTargetDirInHubMode(""); err != nil {
			t.Errorf("RefuseTargetDirInHubMode(\"\") = %v; want nil", err)
		}
	})

	// This case composes Name, StateArtifacts and TargetRole into a real message, not bare fields:
	// the derived state directory nests inside the target.
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
		want := fmt.Sprintf("burler: the derived state directory %s lies inside the standalone target %s: standalone mode keeps its %s strictly outside %s, so the two must be disjoint. The state home is nested under the target -- a repository rooted at your home directory is the usual cause. Point XDG_STATE_HOME (LOCALAPPDATA on Windows) at a directory outside %s and re-run", normalizedStateDir, normalizedTarget, wireModule.StateArtifacts, wireModule.TargetRole, normalizedTarget)
		if err.Error() != want {
			t.Errorf("ResolveStandalone() error = %q; want %q", err.Error(), want)
		}
	})
}

const (
	// The strand roles that the review half's and the fix half's chair seats run under.
	reviewChairRole = "burler-review"
	fixChairRole    = "burler-fix"
)

// halfShuttle is a seatengine.Shuttle double whose started seats finish at once.
// A reviewer writes review to its output file and is done, or dies when review is empty;
// a fixer writes a fixer report to its output file and is done.
// It records every spec it starts.
type halfShuttle struct {
	review string

	// mu guards specs, which the seats' goroutines append to.
	mu    sync.Mutex
	specs []shuttleengine.Spec
}

func (s *halfShuttle) StartGated(spec shuttleengine.Spec, _ shuttleengine.GateSpec) (seatengine.Handle, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.specs = append(s.specs, spec)
	return halfHandle{spec: spec, review: s.review}, nil
}

// ProbeGated finds no live run, since a halfShuttle only starts seats.
func (s *halfShuttle) ProbeGated(shuttleengine.Spec, shuttleengine.GateSpec) (seatengine.Handle, bool, error) {
	return nil, false, nil
}

// startedRoles returns the roles of the seats started so far, in start order.
func (s *halfShuttle) startedRoles() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var roles []string
	for _, spec := range s.specs {
		roles = append(roles, spec.Role)
	}
	return roles
}

// halfHandle is the started seat of a halfShuttle; it names its strand as the seats engine expects.
type halfHandle struct {
	spec   shuttleengine.Spec
	review string
}

func (h halfHandle) StrandGUID() string { return h.spec.Role + "-guid" }

func (h halfHandle) StrandName() string { return "tst:slug:" + h.spec.Role }

func (h halfHandle) RunDir() string { return "/kept/" + h.spec.Role }

func (h halfHandle) SessionState() (shuttleengine.RunSessionState, error) {
	return shuttleengine.RunSessionState{}, nil
}

func (h halfHandle) Send(string) error { return nil }

func (h halfHandle) Wait() (shuttleengine.Result, error) {
	result := shuttleengine.Result{Outcome: shuttleengine.OutcomeDied, StrandGUID: h.StrandGUID(), RunDir: h.RunDir()}
	content := "fixer report\n"
	if h.spec.Role == reviewChairRole {
		content = h.review
	}
	if content == "" {
		return result, nil
	}
	output := h.spec.OutputFiles[0]
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return result, err
	}
	if err := os.WriteFile(output, []byte(content), 0o644); err != nil {
		return result, err
	}
	result.Outcome = shuttleengine.OutcomeDone
	return result, nil
}

// Stop is a no-op; the half is already over.
func (h halfHandle) Stop() error { return nil }

// TestRunCmd drives burler's run verb through its own RunE with seams in place of the live substrate.
// One row proves it calls c.reedUp once, through a recording fake that fails;
// the call then stops there, with c.engine nil.
// One row proves a review with findings runs the fixer and maps the four model flags onto the two halves;
// it also reports each half's identity.
// One row proves a review with no findings runs no fixer and reports fixSkipped beside the Go-written fixer report.
// One row proves a reviewer that died runs no fixer and reports the death as the outcome.
// One row proves it rejects the retired --model flag.
//
//testtiming:keep the round-driving rows pin the run verb's flag mapping, the fixer's gating on the review and the envelope, which no other test reaches
func TestRunCmd(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const (
		profileYAML = `target:
  paths: [target.txt]
fasit:
  paths: [fasit.txt]
rubric: placeholder
fix-scope: source
review-path: reviews/review.md
fixer-report-path: reviews/fixer-report.md
`
		approvedReview = "---\nverdict: APPROVED\n---\nlooks good\n"
		blockingReview = "---\nverdict: BLOCKING\nfindings:\n" +
			"  - id: F1\n    severity: BLOCKING\n    class: design\n    location: target.txt:1\n    summary: colors do not match\n    fix: change the color in target.txt:1\n" +
			"---\n### [BLOCKING] colors do not match\n**Location:** target.txt:1\n**Issue:** found a mismatch\n"
	)

	// newRoundWith builds a CLI over an engine on a halfShuttle whose reviewer writes review;
	// the root is a fresh directory holding profile, and the engine takes cfg.
	newRoundWith := func(t *testing.T, review, profile string, cfg lenses.Config) (c *burlerCLI, shuttle *halfShuttle, profilePath, root string) {
		t.Helper()
		root = t.TempDir()
		for _, name := range []string{"target.txt", "fasit.txt"} {
			if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0o644); err != nil {
				t.Fatalf("write %s: %v", name, err)
			}
		}
		profilePath = filepath.Join(root, "profile.yaml")
		if err := os.WriteFile(profilePath, []byte(profile), 0o644); err != nil {
			t.Fatalf("write profile: %v", err)
		}
		shuttle = &halfShuttle{review: review}
		engine := burlerengine.New(shuttle, burlerengine.Geometry{WorktreeRoot: root, AnchorPath: root, Shortname: "tst", Slug: "slug"}, cfg, stencilkit.Seed(t), "", nil)
		c = &burlerCLI{cwd: root, engine: engine, mode: "standalone"}
		return c, shuttle, profilePath, root
	}

	// newRound builds a round whose reviewer writes review, over the plain profile and an empty lenses.Config.
	newRound := func(t *testing.T, review string) (c *burlerCLI, shuttle *halfShuttle, profilePath, root string) {
		t.Helper()
		return newRoundWith(t, review, profileYAML, lenses.Config{})
	}

	// runEnvelope runs the verb over args and returns its decoded envelope.
	runEnvelope := func(t *testing.T, c *burlerCLI, args ...string) map[string]any {
		t.Helper()
		var out bytes.Buffer
		if exitCode := clihelp.Execute(c.runCmd(), &out, args); exitCode != 0 {
			t.Fatalf("run = exit %d, output: %s", exitCode, out.String())
		}
		var envelope map[string]any
		if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
			t.Fatalf("envelope %q is not JSON: %v", out.String(), err)
		}
		return envelope
	}

	t.Run("calls the reed bring-up once", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		profilePath := filepath.Join(dir, "profile.yaml")
		if err := os.WriteFile(profilePath, []byte("rubric: placeholder\nreview-path: review.md\n"), 0o644); err != nil {
			t.Fatalf("write profile: %v", err)
		}

		c := &burlerCLI{cwd: dir}
		var bringUps int
		c.reedUp = func() error {
			bringUps++
			return errors.New("no tmux server available in this test")
		}

		var out bytes.Buffer
		exitCode := clihelp.Execute(c.runCmd(), &out, []string{"--profile", profilePath})

		if bringUps != 1 {
			t.Fatalf("c.reedUp calls = %d; want exactly 1", bringUps)
		}
		if exitCode != 1 {
			t.Fatalf("run with a failing reed bring-up = %d; want 1, output: %s", exitCode, out.String())
		}
	})

	t.Run("a review with findings runs the fixer and maps the four model flags onto the halves", func(t *testing.T) {
		t.Parallel()

		c, shuttle, profilePath, _ := newRound(t, blockingReview)
		envelope := runEnvelope(t, c,
			"--profile", profilePath,
			"--review-model", "rm", "--review-effort", "re", "--fix-model", "fm", "--fix-effort", "fe",
		)

		if envelope["outcome"] != "done" || envelope["verdict"] != "BLOCKING" || envelope["fixSkipped"] != false {
			t.Errorf("envelope outcome/verdict/fixSkipped = %v/%v/%v; want done/BLOCKING/false", envelope["outcome"], envelope["verdict"], envelope["fixSkipped"])
		}
		for key, role := range map[string]string{"review": reviewChairRole, "fix": fixChairRole} {
			half, ok := envelope[key].(map[string]any)
			if !ok || half["strandGuid"] != role+"-guid" {
				t.Errorf("envelope %s = %v; want the %s half with strand %q", key, envelope[key], role, role+"-guid")
			}
		}
		if want := []string{reviewChairRole, fixChairRole}; !slices.Equal(shuttle.startedRoles(), want) {
			t.Fatalf("started roles = %v; want the reviewer, then the fixer %v", shuttle.startedRoles(), want)
		}

		got := map[string][2]string{}
		for _, spec := range shuttle.specs {
			got[spec.Role] = [2]string{spec.Model, spec.Effort}
			if spec.Engine != "" {
				t.Errorf("%s spec engine = %q; want empty, the default engine", spec.Role, spec.Engine)
			}
			if spec.ForkSubagents {
				t.Errorf("%s spec ForkSubagents = true; want false for a profile naming no fix-fan", spec.Role)
			}
		}
		want := map[string][2]string{reviewChairRole: {"rm", "re"}, fixChairRole: {"fm", "fe"}}
		if len(got) != len(want) || got[reviewChairRole] != want[reviewChairRole] || got[fixChairRole] != want[fixChairRole] {
			t.Errorf("started halves' (model, effort) = %v; want %v", got, want)
		}
	})

	t.Run("a profile naming a fix-fan runs the twill fixer with forks", func(t *testing.T) {
		t.Parallel()

		cfg := lenses.Config{
			Lenses: map[string]string{"correctness": "check the fix is correct"},
			Fans:   map[string][]string{"fix-plan": {"correctness"}},
		}
		c, shuttle, profilePath, _ := newRoundWith(t, blockingReview, profileYAML+"fix-fan: fix-plan\n", cfg)
		envelope := runEnvelope(t, c, "--profile", profilePath)

		if envelope["outcome"] != "done" {
			t.Errorf("envelope outcome = %v; want done", envelope["outcome"])
		}
		forks := map[string]bool{}
		for _, spec := range shuttle.specs {
			forks[spec.Role] = spec.ForkSubagents
		}
		if want := (map[string]bool{reviewChairRole: false, fixChairRole: true}); !maps.Equal(forks, want) {
			t.Errorf("started halves' ForkSubagents = %v; want %v", forks, want)
		}
	})

	t.Run("a review with no findings runs no fixer", func(t *testing.T) {
		t.Parallel()

		c, shuttle, profilePath, root := newRound(t, approvedReview)
		envelope := runEnvelope(t, c, "--profile", profilePath)

		if envelope["outcome"] != "done" || envelope["verdict"] != "APPROVED" || envelope["fixSkipped"] != true {
			t.Errorf("envelope outcome/verdict/fixSkipped = %v/%v/%v; want done/APPROVED/true", envelope["outcome"], envelope["verdict"], envelope["fixSkipped"])
		}
		if want := []string{reviewChairRole}; !slices.Equal(shuttle.startedRoles(), want) {
			t.Errorf("started roles = %v; want only the reviewer %v", shuttle.startedRoles(), want)
		}
		if report, err := os.ReadFile(filepath.Join(root, "reviews", "fixer-report.md")); err != nil || len(report) == 0 {
			t.Errorf("fixer report = %q, %v; want the Go-written report, non-empty", report, err)
		}
	})

	t.Run("a reviewer that died runs no fixer", func(t *testing.T) {
		t.Parallel()

		c, shuttle, profilePath, _ := newRound(t, "")
		envelope := runEnvelope(t, c, "--profile", profilePath)

		if envelope["outcome"] != "died" || envelope["fixSkipped"] != false {
			t.Errorf("envelope outcome/fixSkipped = %v/%v; want died/false", envelope["outcome"], envelope["fixSkipped"])
		}
		if want := []string{reviewChairRole}; !slices.Equal(shuttle.startedRoles(), want) {
			t.Errorf("started roles = %v; want only the reviewer %v", shuttle.startedRoles(), want)
		}
	})

	t.Run("rejects the retired --model flag", func(t *testing.T) {
		t.Parallel()

		c, shuttle, profilePath, _ := newRound(t, approvedReview)
		var out bytes.Buffer
		exitCode := clihelp.Execute(c.runCmd(), &out, []string{"--profile", profilePath, "--model", "m"})

		if exitCode == 0 || !strings.Contains(out.String(), "--model") {
			t.Errorf("run --model = exit %d, output %q; want a non-zero exit naming the unknown flag", exitCode, out.String())
		}
		if len(shuttle.startedRoles()) != 0 {
			t.Errorf("started halves = %v; want none for a rejected flag", shuttle.startedRoles())
		}
	})

	t.Run("a cluster-fan reviewer on an engine without forks is refused naming the engine: way forward", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		for _, name := range []string{"target.txt", "fasit.txt"} {
			if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0o644); err != nil {
				t.Fatalf("write %s: %v", name, err)
			}
		}
		profilePath := filepath.Join(root, "profile.yaml")
		if err := os.WriteFile(profilePath, []byte(profileYAML+"cluster-fan: standard\n"), 0o644); err != nil {
			t.Fatalf("write profile: %v", err)
		}
		reed := &shuttlefake.Reed{}
		runner := shuttleengine.NewRunner(reed, shuttlefake.EngineSet(&shuttlefake.Engine{}), root, root, shuttleengine.Config{RunDir: t.TempDir(), RunTimeoutMin: 60, StartupTimeoutS: 30})
		engine := burlerengine.New(seatengine.RunnerShuttle(runner), burlerengine.Geometry{WorktreeRoot: root, AnchorPath: root, Shortname: "tst", Slug: "slug"}, lenses.Config{
			Lenses: map[string]string{"style": "style prose"},
			Fans:   map[string][]string{"standard": {"style"}},
		}, stencilkit.Seed(t), "", nil)
		c := &burlerCLI{cwd: root, engine: engine, mode: "standalone"}

		var out bytes.Buffer
		exitCode := clihelp.Execute(c.runCmd(), &out, []string{"--profile", profilePath, "--review-model", "rm"})

		if exitCode == 0 {
			t.Fatalf("run = exit 0, output: %s; want the requirements refusal", out.String())
		}
		for _, want := range []string{`engine \"fake\"`, "set engine: in shuttle.yaml to an engine that meets them"} {
			if !strings.Contains(out.String(), want) {
				t.Errorf("run output = %s; want it to contain %q", out.String(), want)
			}
		}
		if len(reed.AddedSpecs) != 0 {
			t.Errorf("reed AddStrand calls = %d; want none, the refusal comes before any pane", len(reed.AddedSpecs))
		}
	})
}

// TestProductionFiles_NeverReferenceHubWatchdogMechanism proves standalone never touches the hub watchdog.
// The scan forbids three things that belong to hub mode alone: the hub lock path (fabricengine.HubScratchDir).
// It also forbids the daemon spawn verb ("reed watchdog").
// It also forbids the seam that owns the detached spawn (reedengine.SpawnWatchdog).
//
// The scan also covers internal/standalonegeom, because reedengine is also standalone's engine,
// so this package's own files alone do not show the property.
// This test spawns nothing and stays tier0.
//
//testtiming:keep internal/loomcli/cli_test.go names this test
func TestProductionFiles_NeverReferenceHubWatchdogMechanism(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	matches, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob *.go: %v", err)
	}
	standaloneGeomMatches, err := filepath.Glob(filepath.Join("..", "standalonegeom", "*.go"))
	if err != nil {
		t.Fatalf("glob ../standalonegeom/*.go: %v", err)
	}
	matches = append(matches, standaloneGeomMatches...)

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
		if strings.Contains(content, "reedengine.SpawnWatchdog") {
			t.Errorf("%s references reedengine.SpawnWatchdog; standalone must never call the hub watchdog seam", path)
		}
	}
}
