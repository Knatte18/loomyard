// main_test.go covers run's subcommand dispatch and decideClone's clone/reset decisions.
// Every test here rewrites the package-level seams (devBinPath, lookPath, cloneRun, removeAll, launchAgent, reedDown).
// Because they rewrite package-level state, none of these tests calls t.Parallel.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// stubResolveLymProd makes resolveLym resolve to the sourceProd branch.
// It returns a function that restores the stubbed seams.
func stubResolveLymProd(t *testing.T, fakeLymPath string) func() {
	t.Helper()
	oldDevBinPath := devBinPath
	oldLookPath := lookPath
	devBinPath = func() (string, error) { return filepath.Join(t.TempDir(), "lym"), nil }
	lookPath = func(name string) (string, error) {
		if name == "lym" {
			return fakeLymPath, nil
		}
		return "", fmt.Errorf("not found on PATH: %s", name)
	}
	return func() {
		devBinPath = oldDevBinPath
		lookPath = oldLookPath
	}
}

// TestDecideClone verifies decideClone's four outcomes:
// an absent Hub is cloned with the parent directory and the resolved prod lym;
// an existing Hub is left alone without reset;
// an existing Hub is removed and then cloned with reset;
// and a cloneRun error is propagated unwrapped.
func TestDecideClone(t *testing.T) {
	tiergate.Require(t, 0)
	const fakeLymPath = "/fake/prod/lym"
	tests := []struct {
		name          string
		hubExists     bool
		reset         bool
		cloneErr      error
		wantRemoveAll bool
		wantClone     bool
	}{
		{name: "hub absent is cloned", wantClone: true},
		{name: "hub present without reset is left alone", hubExists: true},
		{name: "hub present with reset is removed then cloned", hubExists: true, reset: true, wantRemoveAll: true, wantClone: true},
		{name: "clone error is propagated", cloneErr: &exec.ExitError{}, wantClone: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			hubPath := filepath.Join(tmpDir, hubName)
			if tt.hubExists {
				if err := os.MkdirAll(hubPath, 0o755); err != nil {
					t.Fatalf("failed to create Hub directory: %v", err)
				}
			}

			// A run that never clones must never resolve lym, so no resolve stub is installed for it.
			if tt.wantClone {
				defer stubResolveLymProd(t, fakeLymPath)()
			}

			removeAllCalled := false
			cloneRunCalled := false
			oldRemoveAll := removeAll
			oldCloneRun := cloneRun
			defer func() {
				removeAll = oldRemoveAll
				cloneRun = oldCloneRun
			}()
			removeAll = func(path string) error {
				removeAllCalled = true
				if path != hubPath {
					t.Errorf("removeAll called with wrong path: got %s, want %s", path, hubPath)
				}
				return os.RemoveAll(path)
			}
			cloneRun = func(parentDir, lymPath string) error {
				cloneRunCalled = true
				if parentDir != tmpDir {
					t.Errorf("cloneRun called with wrong parentDir: got %s, want %s", parentDir, tmpDir)
				}
				if lymPath != fakeLymPath {
					t.Errorf("cloneRun called with wrong lymPath: got %s, want %s", lymPath, fakeLymPath)
				}
				return tt.cloneErr
			}

			err := decideClone(hubPath, tt.reset)
			if tt.cloneErr == nil && err != nil {
				t.Errorf("decideClone failed: %v", err)
			}
			if tt.cloneErr != nil {
				if err == nil {
					t.Fatal("decideClone should return an error when cloneRun fails")
				}
				if _, ok := err.(*exec.ExitError); !ok {
					t.Errorf("expected exec.ExitError, got %T: %v", err, err)
				}
			}
			if removeAllCalled != tt.wantRemoveAll {
				t.Errorf("removeAll called = %v; want %v", removeAllCalled, tt.wantRemoveAll)
			}
			if cloneRunCalled != tt.wantClone {
				t.Errorf("cloneRun called = %v; want %v", cloneRunCalled, tt.wantClone)
			}
			// cloneRun is stubbed, so a reset leaves the Hub directory removed and not recreated.
			if tt.wantRemoveAll {
				if _, err := os.Stat(hubPath); err == nil {
					t.Error("Hub directory should have been removed")
				}
			}
		})
	}
}

// TestRun_BuildRouting verifies that a bare run and an explicit "build" token both route to the clone path;
// that -reset passed after the build token removes an existing Hub before the clone;
// and that a bare build over an existing Hub removes nothing and clones nothing, so reset must be explicit.
func TestRun_BuildRouting(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name          string
		subcommand    []string
		hubExists     bool
		wantRemoveAll bool
		wantClone     bool
	}{
		{name: "no subcommand defaults to build", wantClone: true},
		{name: "explicit build token", subcommand: []string{"build"}, wantClone: true},
		{name: "-reset after the build token", subcommand: []string{"build", "-reset"}, hubExists: true, wantRemoveAll: true, wantClone: true},
		{name: "build over an existing Hub without -reset", subcommand: []string{"build"}, hubExists: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			if tt.hubExists {
				if err := os.MkdirAll(filepath.Join(tmpDir, hubName), 0o755); err != nil {
					t.Fatalf("create hub: %v", err)
				}
			}

			// The no-op path must never resolve lym at all, so no resolve stub is installed for it.
			if tt.wantClone {
				defer stubResolveLymProd(t, "/fake/prod/lym")()
			}

			removeAllCalled := false
			cloneRunCalled := false
			oldRemoveAll := removeAll
			oldCloneRun := cloneRun
			defer func() {
				removeAll = oldRemoveAll
				cloneRun = oldCloneRun
			}()
			removeAll = func(path string) error {
				removeAllCalled = true
				return os.RemoveAll(path)
			}
			cloneRun = func(parentDir, lymPath string) error {
				cloneRunCalled = true
				return nil
			}

			if code := run(append([]string{"-parent", tmpDir}, tt.subcommand...)); code != 0 {
				t.Errorf("run() = %d; want 0", code)
			}
			if removeAllCalled != tt.wantRemoveAll {
				t.Errorf("removeAll called = %v; want %v", removeAllCalled, tt.wantRemoveAll)
			}
			if cloneRunCalled != tt.wantClone {
				t.Errorf("cloneRun called = %v; want %v", cloneRunCalled, tt.wantClone)
			}
		})
	}
}

// TestRun_SuiteSubcommands verifies, for every suite positional:
// that run routes it to the suite path;
// that it invokes launchAgent with the repo directory and the suite's default instruction;
// that -claude and -prompt flags following the positional are parsed and forwarded to launchAgent;
// that claude is never looked up on PATH;
// that an absent Hub repo exits non-zero, naming sandbox/build.cmd, without launching the agent;
// and that an unknown flag exits non-zero without launching it.
// The subtests "absent Hub exits non-zero" and "unknown flag exits non-zero" cover the last two.
func TestRun_SuiteSubcommands(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		token string
		spec  suiteSpec
	}{
		{"suite", mainSuite},
		{"reed-suite", reedSuite},
		{"shuttle-suite", shuttleSuite},
		{"creel-suite", creelSuite},
		{"burler-suite", burlerSuite},
		{"fabric-suite", fabricSuite},
	}
	for _, tt := range tests {
		t.Run(tt.token, func(t *testing.T) {
			t.Run("routes to launch", func(t *testing.T) {
				parentDir, repoDir := makeHubRepo(t)
				fakeLym := makeFakeLym(t, parentDir)
				fakeClaude := filepath.Join(parentDir, "claude.exe")

				launchAgentCalled := false
				var gotInstruction string
				restore := stubSuiteSeams(t, fakeLym, fakeClaude, func(dir, claude, instruction, binDir string) int {
					launchAgentCalled = true
					gotInstruction = instruction
					if dir != repoDir {
						t.Errorf("launchAgent dir = %q; want %q", dir, repoDir)
					}
					return 0
				})
				defer restore()

				if code := run([]string{"-parent", parentDir, tt.token}); code != 0 {
					t.Errorf("run() = %d; want 0", code)
				}
				if !launchAgentCalled {
					t.Errorf("launchAgent was not called for the %s subcommand", tt.token)
				}
				if gotInstruction != tt.spec.instruction {
					t.Errorf("launchAgent instruction = %q; want %q", gotInstruction, tt.spec.instruction)
				}
			})

			t.Run("flags routed after the token", func(t *testing.T) {
				parentDir, _ := makeHubRepo(t)
				fakeLym := makeFakeLym(t, parentDir)
				customClaude := filepath.Join(parentDir, "custom-claude.exe")
				customPrompt := "Do the " + tt.token + " thing my way."

				var gotClaude, gotInstruction string
				restore := stubSuiteSeams(t, fakeLym, "", func(dir, claude, instruction, binDir string) int {
					gotClaude = claude
					gotInstruction = instruction
					return 0
				})
				defer restore()
				lookPath = func(name string) (string, error) {
					if name == "lym" {
						return fakeLym, nil
					}
					t.Errorf("unexpected lookPath call for %q; claude override should skip PATH lookup", name)
					return "", fmt.Errorf("not found: %s", name)
				}

				code := run([]string{"-parent", parentDir, tt.token, "-claude", customClaude, "-prompt", customPrompt})
				if code != 0 {
					t.Errorf("run() = %d; want 0", code)
				}
				if gotClaude != customClaude {
					t.Errorf("launchAgent claude = %q; want %q", gotClaude, customClaude)
				}
				if gotInstruction != customPrompt {
					t.Errorf("launchAgent instruction = %q; want %q", gotInstruction, customPrompt)
				}
			})

			t.Run("absent Hub exits non-zero", func(t *testing.T) {
				restore := stubSuiteSeams(t, "", "", func(dir, claude, instruction, binDir string) int {
					t.Errorf("launchAgent should not be called when the Hub is absent for the %s subcommand", tt.token)
					return 1
				})
				defer restore()

				var code int
				stderr := captureStderr(t, func() {
					code = run([]string{"-parent", t.TempDir(), tt.token})
				})
				if code == 0 {
					t.Errorf("run() = 0; want non-zero when the Hub repo is absent for the %s subcommand", tt.token)
				}
				if !strings.Contains(stderr, "sandbox/build.cmd") {
					t.Errorf("stderr = %q; want it to contain %q", stderr, "sandbox/build.cmd")
				}
			})

			t.Run("unknown flag exits non-zero", func(t *testing.T) {
				parentDir, _ := makeHubRepo(t)
				fakeLym := makeFakeLym(t, parentDir)
				fakeClaude := filepath.Join(parentDir, "claude.exe")

				restore := stubSuiteSeams(t, fakeLym, fakeClaude, func(dir, claude, instruction, binDir string) int {
					t.Errorf("launchAgent should not be called after a flag-parse failure for the %s subcommand", tt.token)
					return 1
				})
				defer restore()

				var code int
				captureStderr(t, func() {
					code = run([]string{"-parent", parentDir, tt.token, "-bogus"})
				})
				if code == 0 {
					t.Errorf("run() = 0; want non-zero for an unknown flag on the %s subcommand", tt.token)
				}
			})
		})
	}
}

// TestRun_FetchReportRoutesToFetch verifies that the "fetch" positional routes to runFetch:
// with a built Hub, an on-PATH lym, and a report, the dispatch reaches fetchReport and run returns 0.
func TestRun_FetchReportRoutesToFetch(t *testing.T) {
	tiergate.Require(t, 0)
	tmpDir := t.TempDir()
	loomyardRoot := t.TempDir()

	// Create the Hub repo directory that runFetch requires, and drop a valid report there for the fetch to pick up.
	repoDir := filepath.Join(tmpDir, hubName, repoDirName)
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("create repo dir: %v", err)
	}
	reportPath := filepath.Join(repoDir, reportFileName)
	if err := os.WriteFile(reportPath, []byte(`{"source": "sandbox-report", "items": []}`), 0o644); err != nil {
		t.Fatalf("write sandbox report: %v", err)
	}

	// Provide a real file so binaryFingerprint can stat and hash it.
	fakeLym := filepath.Join(tmpDir, "lym.exe")
	if err := os.WriteFile(fakeLym, []byte("fake lym binary"), 0o755); err != nil {
		t.Fatalf("write fake lym: %v", err)
	}
	// No dev binary is in play, so resolveLym falls through to the lookPath stub below and resolves sourceProd.
	oldDevBinPath := devBinPath
	defer func() { devBinPath = oldDevBinPath }()
	devBinPath = func() (string, error) { return filepath.Join(t.TempDir(), "lym"), nil }

	oldLookPath := lookPath
	defer func() { lookPath = oldLookPath }()
	lookPath = func(name string) (string, error) {
		if name == "lym" {
			return fakeLym, nil
		}
		return "", fmt.Errorf("not found: %s", name)
	}

	code := run([]string{"-parent", tmpDir, "-loomyard", loomyardRoot, "fetch"})
	if code != 0 {
		t.Errorf("run() = %d; want 0", code)
	}
}

// TestRun_RejectsBadInvocation verifies that run returns a non-zero code for three bad invocations:
// a missing -parent, a fetch subcommand without its required -loomyard flag, and an unrecognised positional argument.
func TestRun_RejectsBadInvocation(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name string
		args func(parentDir string) []string
	}{
		{"missing -parent", func(string) []string { return []string{} }},
		{"fetch without -loomyard", func(parentDir string) []string { return []string{"-parent", parentDir, "fetch"} }},
		{"unknown subcommand", func(parentDir string) []string { return []string{"-parent", parentDir, "unknowncmd"} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := tt.args(t.TempDir())
			if code := run(args); code == 0 {
				t.Errorf("run(%v) = 0; want non-zero", args)
			}
		})
	}
}
