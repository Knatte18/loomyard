// wiring_test.go pins wire's mode truth table at tier 1: every case drives c.wire directly with a
// told preflight.Mode -- never through the real pre-run -- so no case spawns a process or resolves
// cwd.
//
// The hub-mode cases below hand wire a fictional, on-disk-absent *lyxcwd.Location: every module
// config load wire performs (shuttleengine.LoadConfig, burlerengine.LoadConfig,
// reedengine.LoadConfig) degrades to its embedded template (or, for burler.yaml, the zero Config) on
// a proven-absent _lyx/ directory, so a fictional anchor path drives the whole hub branch without
// touching disk.
//
// The standalone-mode cases reach the one call site of standalonestate.Derive that exists anywhere
// in this package: each such case redirects both XDG_STATE_HOME and LOCALAPPDATA to a t.TempDir()
// BEFORE calling wire, so both of Derive's per-OS branches land inside the test's own temp tree on
// every platform, and none of those cases is marked t.Parallel(), since t.Setenv panics under a
// parallel test.
//
// Two structural facts a later reader might otherwise try to "fix": first, no (loc non-nil,
// ModeStandalone) row exists in this file because no caller can produce one -- preflight.ResolveMode
// returns a nil Location for both standalone causes (the plain downloaded git repo and a genuine
// non-repository directory -- the plain-git-repo cause is what the design's r5 review caught).
// Second, the refuse case is deliberately absent from this file because wire never receives it -- the
// error return aborts upstream in resolvePersistentPreRun before c.wire is ever called -- and
// manufacturing one here would require driving the real pre-run, which spawns git and breaches the
// Test Tier Purity Invariant, the exact invariant wire's extraction exists to satisfy. The refusal is
// pinned instead in internal/preflight's own integration table and in
// internal/burlercli/cli_integration_test.go.
//
// What this file does NOT assert: the reed geometry's field values. burlerengine.Engine exposes only
// Run and holds its geom, cfg, and stencilsDir unexported; shuttleengine.Runner holds reed, engine,
// anchorPath, worktreeRoot, and cfg unexported with no geometry accessor. Both are different packages
// from this in-package test, so SocketKey, SessionName, LogsDir, RepoName, and HubPath are
// unreachable from here, and none of the three reporting fields (c.mode, c.stateDir, c.stencilsDir)
// encodes them. Those five values are already pinned one layer down, by TestReedGeometry in
// internal/standalonegeom/standalonegeom_test.go, which asserts all eight fields against told
// parameters. What that leaves uncovered by any test is the *linkage* -- that wireStandalone calls
// standalonegeom.ReedGeometry with this invocation's own target, stateDir, and hash8 rather than some
// other triple. No seam exposes it, so it is a review obligation rather than an assertion, recorded
// here so a later reader knows the gap is recorded rather than overlooked.

package burlercli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/preflight"
	"github.com/Knatte18/loomyard/internal/standalonegeom"
	"github.com/Knatte18/loomyard/internal/standalonestate"
)

// hubLocation returns a *lyxcwd.Location standing in for a real hub location. It performs no
// filesystem preparation of its own -- see the file header for why wire's hub branch tolerates that.
func hubLocation(hub, worktreeName, anchorRel string) *lyxcwd.Location {
	return &lyxcwd.Location{HubPath: hub, WorktreeName: worktreeName, AnchorRel: anchorRel}
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

// setStandaloneStateRoot redirects both env vars standalonestate.Derive reads to fresh t.TempDir()
// values, so a case that reaches wireStandalone stays hermetic. Not t.Parallel() -- t.Setenv panics
// under a parallel test.
// It also registers the sink-override cleanup every caller here needs, per the overview's
// sink-override-is-process-global decision: every case that calls this helper reaches wireStandalone,
// which now sets the process-global durable sink override, and a leaked override would defeat the
// testing.Testing() sink suppression for every later test in this binary.
func setStandaloneStateRoot(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Cleanup(func() { logger.SetDurableSinkDir("") })
}

// TestWire_ModeHubSelectsHubMode covers the (loc non-nil, ModeHub) row: wire must select hub mode and
// resolve c.mode/c.stateDir/c.stencilsDir to their hub-mode values.
func TestWire_ModeHubSelectsHubMode(t *testing.T) {
	t.Parallel()

	hub := t.TempDir()
	loc := hubLocation(hub, "warp", ".")

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

			c := &burlerCLI{}
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
		})
	}
}

// TestWireStandalone_NeverReadsLoc proves wireStandalone never reads loc -- it takes only cwd and the
// flags.
func TestWireStandalone_NeverReadsLoc(t *testing.T) {
	target := t.TempDir()
	setStandaloneStateRoot(t)

	c := &burlerCLI{}
	if err := c.wireStandalone(target, "", ""); err != nil {
		t.Fatalf("wireStandalone() = %v; want nil", err)
	}
	if c.mode != "standalone" {
		t.Errorf("c.mode = %q; want %q", c.mode, "standalone")
	}
}

// TestWire_StandalonePinnedValues pins every standalone value the design names: the burler
// geometry's WorktreeRoot equals the target and its AnchorPath equals stateDir, the config base is
// stateDir (proven indirectly by wire succeeding against a fictional, on-disk-absent state
// directory), and c.stencilsDir equals standalonegeom.StencilsDir(stateDir) when the flag is unset.
//
// The burler geometry's WorktreeRoot/AnchorPath are not observable from outside the constructed
// *burlerengine.Engine (its geom field is unexported), so this test asserts them indirectly: it
// derives stateDir itself via standalonestate.Derive(target) and compares c.stateDir/c.stencilsDir,
// which are the receiver's own reporting fields -- see the file header for what is and is not
// observable at this seam.
func TestWire_StandalonePinnedValues(t *testing.T) {
	target := t.TempDir()
	setStandaloneStateRoot(t)
	stateDir, hash8 := hash8AndStateDir(t, target)

	c := &burlerCLI{}
	if err := c.wire(nil, preflight.ModeStandalone, target, "", ""); err != nil {
		t.Fatalf("wire() = %v; want nil", err)
	}

	if c.stateDir != stateDir {
		t.Errorf("c.stateDir = %q; want %q", c.stateDir, stateDir)
	}
	if want := standalonegeom.StencilsDir(stateDir); c.stencilsDir != want {
		t.Errorf("c.stencilsDir = %q; want %q", c.stencilsDir, want)
	}

	// standalonegeom.ReedGeometry's own field values are pinned by TestReedGeometry in
	// internal/standalonegeom/standalonegeom_test.go; hash8 is asserted here only to prove this
	// invocation derived one at all, not to re-pin ReedGeometry's mapping.
	if hash8 == "" {
		t.Error("hash8 = \"\"; want a derived hash8")
	}
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

// TestWire_TargetDirRefusedInHubMode proves --target-dir is refused in hub mode with an error
// explaining why, and that the refusal happens before any config is loaded (an empty hub, no module
// config on disk anywhere, still refuses on the flag alone).
func TestWire_TargetDirRefusedInHubMode(t *testing.T) {
	t.Parallel()

	hub := t.TempDir()
	loc := hubLocation(hub, "warp", ".")

	c := &burlerCLI{}
	err := c.wire(loc, preflight.ModeHub, "", "", filepath.Join(t.TempDir(), "elsewhere"))
	if err == nil {
		t.Fatal("wire() error = nil; want a refusal naming --target-dir")
	}
	if !strings.Contains(err.Error(), "--target-dir") {
		t.Errorf("wire() error = %v; want it to name --target-dir", err)
	}
}

// TestWire_StencilsDirFlag covers --stencils-dir: honoured in both modes, the standalone default
// seeded on disk when the flag is unset, and an explicit --stencils-dir never written to.
func TestWire_StencilsDirFlag(t *testing.T) {
	t.Run("HonouredInHubMode", func(t *testing.T) {
		t.Parallel()
		hub := t.TempDir()
		loc := hubLocation(hub, "warp", ".")
		override := filepath.Join(t.TempDir(), "custom-stencils")

		c := &burlerCLI{}
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

		c := &burlerCLI{}
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

		c := &burlerCLI{}
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

		c := &burlerCLI{}
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
}

// TestWire_RelativeStencilsDirResolvesAgainstCwd is R4-23's direct regression test for this module.
// --stencils-dir used to be stored verbatim, so a relative value reached the engine unresolved: the
// CLI process would read it against ITS working directory while the pane burler spawns runs at the
// target (standalone) or the anchor (hub), so one string named two different directories.
func TestWire_RelativeStencilsDirResolvesAgainstCwd(t *testing.T) {
	t.Run("HubMode", func(t *testing.T) {
		t.Parallel()
		hub := t.TempDir()
		loc := hubLocation(hub, "warp", ".")
		cwd := t.TempDir()

		c := &burlerCLI{}
		if err := c.wire(loc, preflight.ModeHub, cwd, filepath.Join("custom", "stencils"), ""); err != nil {
			t.Fatalf("wire() = %v; want nil", err)
		}
		if want := filepath.Join(cwd, "custom", "stencils"); c.stencilsDir != want {
			t.Errorf("c.stencilsDir = %q; want the relative --stencils-dir resolved against cwd, %q", c.stencilsDir, want)
		}
	})

	t.Run("StandaloneMode", func(t *testing.T) {
		target := t.TempDir()
		setStandaloneStateRoot(t)
		cwd := t.TempDir()

		c := &burlerCLI{}
		if err := c.wire(nil, preflight.ModeStandalone, cwd, filepath.Join("custom", "stencils"), target); err != nil {
			t.Fatalf("wire() = %v; want nil", err)
		}
		if want := filepath.Join(cwd, "custom", "stencils"); c.stencilsDir != want {
			t.Errorf("c.stencilsDir = %q; want the relative --stencils-dir resolved against cwd, %q", c.stencilsDir, want)
		}
	})
}

// TestWireStandalone_RefusesStateDirNestedInTarget is R4-25's direct regression test for this
// module. A standalone target that CONTAINS its own derived state directory -- a dotfiles repository
// rooted at the home directory, or an XDG_STATE_HOME pointed somewhere inside the checkout -- used to
// reach shuttleengine.NewDetachedRunner's containment assertion, which fires only once a tmux server
// has been booted, blames a hub geometry that was never involved, and names no lever the operator
// can pull.
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

	c := &burlerCLI{}
	err := c.wire(nil, preflight.ModeStandalone, target, "", "")
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

	logger.Info("wiring_test: arm the sink")
	matches, err := filepath.Glob(filepath.Join(sentinelDir, "trace-*.log"))
	if err != nil {
		t.Fatalf("glob %s: %v", sentinelDir, err)
	}
	if len(matches) == 0 {
		t.Error("no trace-*.log file under the sentinel dir; want the nesting refusal to fire before wireStandalone redirects the durable sink into the operator's own repository")
	}
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

// TestWireStandalone_RunnerReachesPublicEntryPointWithoutToldPathError is F16's direct regression
// test. It fails against pre-fix source, where wireStandalone constructed its runner via
// shuttleengine.NewRunner: NewRunner's containment assertion refuses standalone's deliberately
// detached anchor/worktree-root pair (the derived state directory sits outside the target
// repository), setting the runner's held toldErr, which every public entry point returns
// immediately without ever reaching reed. A runner that is merely non-nil proves nothing here, so
// this test drives the one public entry point reachable from this package -- c.engine.Run, with a
// minimal but validate()-passing Profile -- and asserts the returned error is an ordinary reed
// "no session" verdict rather than a told-path refusal. It reaches no live reed session (none was
// ever started, so requireSessionLocked fails fast) and spawns no process: claudeengine.Prepare only
// writes prompt/settings files before AddStrand's pre-flight rejects the call.
func TestWireStandalone_RunnerReachesPublicEntryPointWithoutToldPathError(t *testing.T) {
	target := t.TempDir()
	setStandaloneStateRoot(t)

	fixture := filepath.Join(target, "fixture.md")
	if err := os.WriteFile(fixture, []byte("placeholder"), 0o644); err != nil {
		t.Fatalf("write fixture file: %v", err)
	}

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

	_, err := c.engine.Run(profile, burlerengine.RunOpts{})
	if err == nil {
		t.Fatal("engine.Run() error = nil; want a reed \"no session\" error, since no reed session was ever started")
	}
	if strings.Contains(err.Error(), "NewRunner") || strings.Contains(err.Error(), "NewDetachedRunner") {
		t.Fatalf("engine.Run() error = %v; want the ordinary reed \"no session\" verdict, not a told-path refusal -- this is exactly the error NewRunner's containment assertion would have produced against standalone's detached anchor/worktree-root pair", err)
	}
}

// TestWireStandalone_RedirectsDurableSinkToStandaloneLogsDir is F22's direct regression test. It
// observes the sink directory the only way this package can: by forcing a write and checking the
// filesystem, never by reading internal/logger's own state (sinkDirOverride is unexported and
// internal/logger exposes no accessor). The mechanism is that a non-empty override bypasses the
// testing.Testing() sink suppression in ensureDurableSink, so one logger.Info call after
// wireStandalone returns arms the sink at whatever directory the override names, with no LYX_TRACE
// redirect needed.
func TestWireStandalone_RedirectsDurableSinkToStandaloneLogsDir(t *testing.T) {
	target := t.TempDir()
	setStandaloneStateRoot(t)
	stateDir, _ := hash8AndStateDir(t, target)

	c := &burlerCLI{}
	if err := c.wire(nil, preflight.ModeStandalone, target, "", ""); err != nil {
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

// TestResolveStandaloneTarget covers resolveStandaloneTarget's three flag rows: unset returns cwd,
// absolute returns the cleaned path, relative returns the path joined onto cwd.
// Every told directory here is real but empty, so both normalizations resolveStandaloneTarget applies
// on top -- the symlink resolve and the repository-root lift -- are no-ops on this table by
// construction, leaving the flag rows alone to be asserted.
// TestResolveStandaloneTarget_LiftsToRepositoryRoot owns those two.
func TestResolveStandaloneTarget(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	cwd := filepath.Join(base, "home", "operator", "repo")
	absolute := filepath.Join(base, "elsewhere", "target")
	sibling := filepath.Join(base, "home", "operator", "sibling")
	for _, dir := range []string{cwd, absolute, sibling} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) error = %v", dir, err)
		}
	}

	tests := []struct {
		name          string
		targetDirFlag string
		want          string
	}{
		{"Unset", "", cwd},
		{"Absolute", absolute, absolute},
		{"Relative", filepath.Join("..", "sibling"), sibling},
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

// TestResolveStandaloneTarget_RefusesATargetThatIsNotAReadableDirectory pins R6-7: a --target-dir
// that does not exist, or that names a file, must be REFUSED rather than resolved. Without the
// check, standalonestate.Normalize fell back to Clean and repositoryRootOf then climbed to the
// enclosing repository, so a mistyped flag silently drove -- and, on burler's fix phase, WROTE INTO
// -- the repository the operator was standing in, indistinguishably from the no-flag invocation.
func TestResolveStandaloneTarget_RefusesATargetThatIsNotAReadableDirectory(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	seedGitRepositoryRoot(t, repo)
	file := filepath.Join(repo, "notadir.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	for _, tt := range []struct {
		name          string
		targetDirFlag string
	}{
		{"absent directory", "reposs"},
		{"a file, not a directory", "notadir.txt"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveStandaloneTarget(repo, tt.targetDirFlag)
			if err == nil {
				t.Fatalf("resolveStandaloneTarget(%q, %q) = %q, nil; want a refusal -- this silently resolves to the enclosing repository", repo, tt.targetDirFlag, got)
			}
			if got != "" {
				t.Errorf("resolveStandaloneTarget(...) target = %q; want empty alongside the refusal", got)
			}
		})
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
// different hash8 values and so derived two state directories and two reed sessions for one
// repository, while the profile's own relative target and fasit paths silently resolved against the
// subdirectory rather than the repository.
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

	t.Run("SubdirectoryWiresOntoTheRootsOwnStateDir", func(t *testing.T) {
		repoRoot := seedGitRepositoryRoot(t, t.TempDir())
		subDir := filepath.Join(repoRoot, "src")
		if err := os.MkdirAll(subDir, 0o755); err != nil {
			t.Fatalf("mkdir src: %v", err)
		}
		setStandaloneStateRoot(t)

		normalizedRoot := standalonestate.Normalize(repoRoot)
		rootStateDir, _ := hash8AndStateDir(t, normalizedRoot)

		c := &burlerCLI{}
		if err := c.wire(nil, preflight.ModeStandalone, subDir, "", ""); err != nil {
			t.Fatalf("wire() from a repository subdirectory = %v; want nil", err)
		}
		if c.stateDir != rootStateDir {
			t.Errorf("c.stateDir = %q; want the repository root's own state directory %q -- where the operator stands inside a repository must not change which repository burler reviews", c.stateDir, rootStateDir)
		}
	})
}

// TestWire_ReedUpSeamPerMode is F-A1's (round fable5-high-r3) wiring pin, mirroring
// internal/webstercli's test of the same name: wireStandalone must arm the in-process reed
// bring-up seam the run verb fires before driving a round (standalone's derived geometry is
// reachable by no CLI verb — `lyx reed up` is hub-only), and wireHub must leave it nil.
func TestWire_ReedUpSeamPerMode(t *testing.T) {
	t.Run("StandaloneArmsTheSeam", func(t *testing.T) {
		target := t.TempDir()
		setStandaloneStateRoot(t)

		c := &burlerCLI{}
		if err := c.wire(nil, preflight.ModeStandalone, target, "", ""); err != nil {
			t.Fatalf("wire() = %v; want nil", err)
		}
		if c.reedUp == nil {
			t.Error("wireStandalone left c.reedUp nil; want the in-process reed bring-up seam armed")
		}
	})

	t.Run("HubLeavesTheSeamNil", func(t *testing.T) {
		hub := t.TempDir()
		loc := hubLocation(hub, "warp", ".")

		c := &burlerCLI{}
		if err := c.wire(loc, preflight.ModeHub, "", "", ""); err != nil {
			t.Fatalf("wire() = %v; want nil", err)
		}
		if c.reedUp != nil {
			t.Error("wireHub armed c.reedUp; want nil — hub mode's reed session is not run's to boot")
		}
	})
}

// TestRefuseNestedStandaloneGeometry_SeesThroughASymlinkedStateHome is R6-15's regression test. The
// target has already been through standalonestate.Normalize with every symlink resolved, while the
// derived state directory had not, so a state home that reaches INSIDE the target only through a
// symlink read as disjoint — and lyx then wrote its state tree, run locks and trace logs into the
// operator's own checkout, which is precisely what this guard exists to prevent.
func TestRefuseNestedStandaloneGeometry_SeesThroughASymlinkedStateHome(t *testing.T) {
	t.Parallel()

	target := standalonestate.Normalize(t.TempDir())
	inside := filepath.Join(target, "state-home")
	if err := os.MkdirAll(inside, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", inside, err)
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(inside, link); err != nil {
		t.Skipf("symlinks unavailable on this host: %v", err)
	}
	// The leaf is deliberately not created: a first run's <stateHome>/lyx/<hash8> does not exist yet,
	// which is exactly when plain EvalSymlinks gives up and falls back to Clean.
	stateDir := filepath.Join(link, "lyx", "abcd1234")

	if err := refuseNestedStandaloneGeometry("burler", target, stateDir); err == nil {
		t.Errorf("refuseNestedStandaloneGeometry(%q, %q) = nil; want a refusal — the state home is nested under the target through a symlink", target, stateDir)
	}
}
