// main_test.go covers run's subcommand dispatch and decideClone's clone/reset decisions.
// Every test here rewrites the package-level seams (devBinPath, lookPath, cloneRun, removeAll,
// launchAgent, reedDown), so none calls t.Parallel.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// stubResolveLyxProd stubs resolveLyx to return the sourceProd branch.
func stubResolveLyxProd(t *testing.T, fakeLyxPath string) func() {
	t.Helper()
	oldDevBinPath := devBinPath
	oldLookPath := lookPath
	devBinPath = func() (string, error) { return filepath.Join(t.TempDir(), "lyx"), nil }
	lookPath = func(name string) (string, error) {
		if name == "lyx" {
			return fakeLyxPath, nil
		}
		return "", fmt.Errorf("not found on PATH: %s", name)
	}
	return func() {
		devBinPath = oldDevBinPath
		lookPath = oldLookPath
	}
}

// TestDecideClone verifies decideClone's four outcomes: an absent Hub is cloned with the parent
// directory and the resolved prod lyx; an existing Hub is left alone without reset; an existing Hub
// is removed and then cloned with reset; and a cloneRun error is propagated unwrapped.
func TestDecideClone(t *testing.T) {
	const fakeLyxPath = "/fake/prod/lyx"
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

			// A run that never clones must never resolve lyx, so no resolve stub is installed for it.
			if tt.wantClone {
				defer stubResolveLyxProd(t, fakeLyxPath)()
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
			cloneRun = func(parentDir, lyxPath string) error {
				cloneRunCalled = true
				if parentDir != tmpDir {
					t.Errorf("cloneRun called with wrong parentDir: got %s, want %s", parentDir, tmpDir)
				}
				if lyxPath != fakeLyxPath {
					t.Errorf("cloneRun called with wrong lyxPath: got %s, want %s", lyxPath, fakeLyxPath)
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

// TestRun_BuildRouting verifies that a bare run and an explicit "build" token both route to the
// clone path, that -reset passed after the build token removes an existing Hub before the clone, and
// that a bare build over an existing Hub removes nothing and clones nothing -- reset must be
// explicit.
func TestRun_BuildRouting(t *testing.T) {
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

			// The no-op path must never resolve lyx at all, so no resolve stub is installed for it.
			if tt.wantClone {
				defer stubResolveLyxProd(t, "/fake/prod/lyx")()
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
			cloneRun = func(parentDir, lyxPath string) error {
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

// TestRun_SuiteSubcommands verifies, for every suite positional, that run routes it to the suite
// path and invokes launchAgent with the repo directory and the suite's default instruction; that
// -claude/-prompt flags following the positional are parsed and forwarded to launchAgent without a
// PATH lookup for claude; and that an absent Hub repo is propagated as a non-zero exit code.
func TestRun_SuiteSubcommands(t *testing.T) {
	tests := []struct {
		token string
		spec  suiteSpec
	}{
		{"suite", mainSuite},
		{"reed-suite", reedSuite},
		{"shuttle-suite", shuttleSuite},
		{"burler-suite", burlerSuite},
		{"fabric-suite", fabricSuite},
	}
	for _, tt := range tests {
		t.Run(tt.token, func(t *testing.T) {
			t.Run("routes to launch", func(t *testing.T) {
				parentDir, repoDir := makeHubRepo(t)
				fakeLyx := makeFakeLyx(t, parentDir)
				fakeClaude := filepath.Join(parentDir, "claude.exe")

				launchAgentCalled := false
				var gotInstruction string
				restore := stubSuiteSeams(t, fakeLyx, fakeClaude, func(dir, claude, instruction, binDir string) int {
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
				fakeLyx := makeFakeLyx(t, parentDir)
				customClaude := filepath.Join(parentDir, "custom-claude.exe")
				customPrompt := "Do the " + tt.token + " thing my way."

				var gotClaude, gotInstruction string
				restore := stubSuiteSeams(t, fakeLyx, "", func(dir, claude, instruction, binDir string) int {
					gotClaude = claude
					gotInstruction = instruction
					return 0
				})
				defer restore()
				lookPath = func(name string) (string, error) {
					if name == "lyx" {
						return fakeLyx, nil
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
				if code := run([]string{"-parent", t.TempDir(), tt.token}); code == 0 {
					t.Errorf("run() = 0; want non-zero when the Hub repo is absent for the %s subcommand", tt.token)
				}
			})
		})
	}
}

// TestRun_FetchReportRoutesToFetch verifies that the "fetch" positional routes to runFetch: with a
// built Hub, an on-PATH lyx, and a report, the dispatch reaches fetchReport and run returns 0.
func TestRun_FetchReportRoutesToFetch(t *testing.T) {
	tmpDir := t.TempDir()
	loomyardRoot := t.TempDir()

	// Create the Hub repo directory that runFetch requires, and drop a valid
	// report there for the fetch to pick up.
	repoDir := filepath.Join(tmpDir, hubName, repoDirName)
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("create repo dir: %v", err)
	}
	reportPath := filepath.Join(repoDir, reportFileName)
	if err := os.WriteFile(reportPath, []byte(`{"source": "sandbox-report", "items": []}`), 0o644); err != nil {
		t.Fatalf("write sandbox report: %v", err)
	}

	// Provide a real file so binaryFingerprint can stat and hash it.
	fakeLyx := filepath.Join(tmpDir, "lyx.exe")
	if err := os.WriteFile(fakeLyx, []byte("fake lyx binary"), 0o755); err != nil {
		t.Fatalf("write fake lyx: %v", err)
	}
	// No dev binary in play: resolveLyx must fall through to the lookPath
	// stub below and resolve sourceProd.
	oldDevBinPath := devBinPath
	defer func() { devBinPath = oldDevBinPath }()
	devBinPath = func() (string, error) { return filepath.Join(t.TempDir(), "lyx"), nil }

	oldLookPath := lookPath
	defer func() { lookPath = oldLookPath }()
	lookPath = func(name string) (string, error) {
		if name == "lyx" {
			return fakeLyx, nil
		}
		return "", fmt.Errorf("not found: %s", name)
	}

	code := run([]string{"-parent", tmpDir, "-loomyard", loomyardRoot, "fetch"})
	if code != 0 {
		t.Errorf("run() = %d; want 0", code)
	}
}

// TestRun_RejectsBadInvocation verifies run returns a non-zero code when -parent is missing, when
// the fetch subcommand lacks its required -loomyard flag, and for an unrecognised positional
// argument.
func TestRun_RejectsBadInvocation(t *testing.T) {
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
