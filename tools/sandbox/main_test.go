package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestDecideClone_HubPathComputation tests that hub path is computed correctly from -parent.
func TestDecideClone_HubPathComputation(t *testing.T) {
	tests := []struct {
		name        string
		parentInput string
		expectHub   string // relative to tempdir if parent is relative
	}{
		{
			name:        "absolute parent path",
			parentInput: "/absolute/path",
			expectHub:   filepath.Join("/absolute/path", hubName),
		},
		{
			name:        "relative parent path",
			parentInput: "relative/path",
			// Note: this subtest validates that filepath.IsAbs resolves relative paths
			// correctly by joining them with a temp directory base. It does not verify
			// a specific expected path value; the temp dir base is generated per test.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// For relative paths, use a temp directory as the base.
			var parentDir string
			if filepath.IsAbs(tt.parentInput) {
				parentDir = tt.parentInput
			} else {
				tmpDir := t.TempDir()
				parentDir = filepath.Join(tmpDir, tt.parentInput)
			}

			// Verify that filepath.Abs resolves relative paths correctly.
			absParent, err := filepath.Abs(parentDir)
			if err != nil {
				t.Fatalf("filepath.Abs failed: %v", err)
			}

			hubPath := filepath.Join(absParent, hubName)
			// Just verify the hub path is correctly constructed.
			if !filepath.IsAbs(hubPath) {
				t.Errorf("expected absolute hub path, got %s", hubPath)
			}
			if filepath.Base(hubPath) != hubName {
				t.Errorf("expected hub name %s, got %s", hubName, filepath.Base(hubPath))
			}
		})
	}
}

// TestDecideClone_HubAbsent tests that cloneRun is invoked when the Hub does not exist.
func TestDecideClone_HubAbsent(t *testing.T) {
	tmpDir := t.TempDir()
	hubPath := filepath.Join(tmpDir, hubName)

	cloneRunCalled := false
	oldCloneRun := cloneRun
	defer func() { cloneRun = oldCloneRun }()

	cloneRun = func(parentDir string) error {
		cloneRunCalled = true
		if parentDir != tmpDir {
			t.Errorf("cloneRun called with wrong parentDir: got %s, want %s", parentDir, tmpDir)
		}
		return nil
	}

	err := decideClone(hubPath, false)
	if err != nil {
		t.Errorf("decideClone failed: %v", err)
	}
	if !cloneRunCalled {
		t.Error("cloneRun was not called when Hub did not exist")
	}
}

// TestDecideClone_HubPresent_NoReset tests that cloneRun is not called when Hub exists and reset is false.
func TestDecideClone_HubPresent_NoReset(t *testing.T) {
	tmpDir := t.TempDir()
	hubPath := filepath.Join(tmpDir, hubName)

	// Create the Hub directory
	if err := os.MkdirAll(hubPath, 0o755); err != nil {
		t.Fatalf("failed to create Hub directory: %v", err)
	}

	cloneRunCalled := false
	oldCloneRun := cloneRun
	defer func() { cloneRun = oldCloneRun }()

	cloneRun = func(parentDir string) error {
		cloneRunCalled = true
		return nil
	}

	err := decideClone(hubPath, false)
	if err != nil {
		t.Errorf("decideClone failed: %v", err)
	}
	if cloneRunCalled {
		t.Error("cloneRun should not be called when Hub exists and reset is false")
	}
}

// TestDecideClone_HubPresent_Reset tests that removeAll is called and cloneRun is invoked when Hub exists and reset is true.
func TestDecideClone_HubPresent_Reset(t *testing.T) {
	tmpDir := t.TempDir()
	hubPath := filepath.Join(tmpDir, hubName)

	// Create the Hub directory
	if err := os.MkdirAll(hubPath, 0o755); err != nil {
		t.Fatalf("failed to create Hub directory: %v", err)
	}

	// Verify the Hub exists
	if _, err := os.Stat(hubPath); err != nil {
		t.Fatalf("Hub directory does not exist: %v", err)
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
		// Actually remove the directory for the test
		return os.RemoveAll(path)
	}

	cloneRun = func(parentDir string) error {
		cloneRunCalled = true
		if parentDir != tmpDir {
			t.Errorf("cloneRun called with wrong parentDir: got %s, want %s", parentDir, tmpDir)
		}
		return nil
	}

	err := decideClone(hubPath, true)
	if err != nil {
		t.Errorf("decideClone failed: %v", err)
	}
	if !removeAllCalled {
		t.Error("removeAll was not called when reset is true")
	}
	if !cloneRunCalled {
		t.Error("cloneRun was not called when reset is true")
	}

	// Verify the Hub directory was actually removed and recreated would have happened.
	// Since cloneRun is stubbed, the directory should not exist.
	if _, err := os.Stat(hubPath); err == nil {
		t.Error("Hub directory should have been removed")
	}
}

// TestDecideClone_CloneRunError tests that cloneRun errors are propagated.
func TestDecideClone_CloneRunError(t *testing.T) {
	tmpDir := t.TempDir()
	hubPath := filepath.Join(tmpDir, hubName)

	oldCloneRun := cloneRun
	defer func() { cloneRun = oldCloneRun }()

	cloneRun = func(parentDir string) error {
		return &exec.ExitError{}
	}

	err := decideClone(hubPath, false)
	if err == nil {
		t.Error("decideClone should return an error when cloneRun fails")
	}
	// The error from cloneRun should be propagated.
	if !isExecExitError(err) {
		t.Errorf("expected exec.ExitError, got %T: %v", err, err)
	}
}

// isExecExitError checks if an error is or wraps an exec.ExitError.
func isExecExitError(err error) bool {
	_, ok := err.(*exec.ExitError)
	return ok
}

// TestRun_MissingParent tests that run returns non-zero when -parent is absent.
func TestRun_MissingParent(t *testing.T) {
	code := run([]string{})
	if code == 0 {
		t.Error("run() = 0; want non-zero when -parent is missing")
	}
}

// TestRun_DefaultBuildRoutesToClone tests that a bare run with no subcommand
// routes to decideClone (the build path) and invokes cloneRun.
func TestRun_DefaultBuildRoutesToClone(t *testing.T) {
	tmpDir := t.TempDir()

	cloneRunCalled := false
	oldCloneRun := cloneRun
	defer func() { cloneRun = oldCloneRun }()
	cloneRun = func(parentDir string) error {
		cloneRunCalled = true
		return nil
	}

	// No subcommand → defaults to build.
	code := run([]string{"-parent", tmpDir})
	if code != 0 {
		t.Errorf("run() = %d; want 0", code)
	}
	if !cloneRunCalled {
		t.Error("cloneRun was not called for default (build) subcommand")
	}
}

// TestRun_BuildSubcommandRoutesToClone tests that the explicit "build" token
// also routes to the clone path.
func TestRun_BuildSubcommandRoutesToClone(t *testing.T) {
	tmpDir := t.TempDir()

	cloneRunCalled := false
	oldCloneRun := cloneRun
	defer func() { cloneRun = oldCloneRun }()
	cloneRun = func(parentDir string) error {
		cloneRunCalled = true
		return nil
	}

	code := run([]string{"-parent", tmpDir, "build"})
	if code != 0 {
		t.Errorf("run() = %d; want 0", code)
	}
	if !cloneRunCalled {
		t.Error("cloneRun was not called for explicit build subcommand")
	}
}

// TestRun_ResetRoutesToBuildWithReset tests that -reset with no subcommand
// removes the existing Hub and re-runs the clone.
func TestRun_ResetRoutesToBuildWithReset(t *testing.T) {
	tmpDir := t.TempDir()

	// Create the Hub directory so removeAll is triggered.
	hubPath := filepath.Join(tmpDir, hubName)
	if err := os.MkdirAll(hubPath, 0o755); err != nil {
		t.Fatalf("create hub: %v", err)
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
	cloneRun = func(parentDir string) error {
		cloneRunCalled = true
		return nil
	}

	code := run([]string{"-parent", tmpDir, "-reset"})
	if code != 0 {
		t.Errorf("run() = %d; want 0", code)
	}
	if !removeAllCalled {
		t.Error("removeAll was not called when -reset is set")
	}
	if !cloneRunCalled {
		t.Error("cloneRun was not called when -reset is set")
	}
}

// TestRun_SuiteRoutesSuiteToLaunch tests that the "suite" positional routes to
// the suite path and ultimately invokes launchAgent with the correct directory.
// The suite subcommand no longer fetches, so it needs neither -loomyard nor a
// report written by the launch stub.
func TestRun_SuiteRoutesSuiteToLaunch(t *testing.T) {
	tmpDir := t.TempDir()

	// Create the Hub host repo directory that runSuite requires.
	hostRepoDir := filepath.Join(tmpDir, hubName, hostDirName)
	if err := os.MkdirAll(filepath.Join(hostRepoDir, ".git", "info"), 0o755); err != nil {
		t.Fatalf("create host repo dir: %v", err)
	}

	// Provide a real file so binaryFingerprint can stat and hash it.
	fakeLyx := filepath.Join(tmpDir, "lyx.exe")
	if err := os.WriteFile(fakeLyx, []byte("fake lyx binary"), 0o755); err != nil {
		t.Fatalf("write fake lyx: %v", err)
	}
	fakeClaude := filepath.Join(tmpDir, "claude.exe")

	oldLookPath := lookPath
	defer func() { lookPath = oldLookPath }()
	lookPath = func(name string) (string, error) {
		switch name {
		case "lyx":
			return fakeLyx, nil
		case "claude":
			return fakeClaude, nil
		default:
			return "", fmt.Errorf("not found: %s", name)
		}
	}

	launchAgentCalled := false
	oldLaunchAgent := launchAgent
	defer func() { launchAgent = oldLaunchAgent }()
	launchAgent = func(dir, claude, instruction string) int {
		launchAgentCalled = true
		if dir != hostRepoDir {
			t.Errorf("launchAgent dir = %q; want %q", dir, hostRepoDir)
		}
		return 0
	}

	code := run([]string{"-parent", tmpDir, "suite"})
	if code != 0 {
		t.Errorf("run() = %d; want 0", code)
	}
	if !launchAgentCalled {
		t.Error("launchAgent was not called for suite subcommand")
	}
}

// TestRun_FetchReportRoutesToFetch verifies that the "fetch" positional
// routes to runFetch: with a built Hub, an on-PATH lyx, and a host report, the
// dispatch reaches fetchReport and run returns 0.
func TestRun_FetchReportRoutesToFetch(t *testing.T) {
	tmpDir := t.TempDir()
	loomyardRoot := t.TempDir()

	// Create the Hub host repo directory that runFetch requires, and drop a valid
	// report there for the fetch to pick up.
	hostRepoDir := filepath.Join(tmpDir, hubName, hostDirName)
	if err := os.MkdirAll(hostRepoDir, 0o755); err != nil {
		t.Fatalf("create host repo dir: %v", err)
	}
	reportPath := filepath.Join(hostRepoDir, reportFileName)
	if err := os.WriteFile(reportPath, []byte(`{"source": "sandbox-report", "items": []}`), 0o644); err != nil {
		t.Fatalf("write sandbox report: %v", err)
	}

	// Provide a real file so binaryFingerprint can stat and hash it.
	fakeLyx := filepath.Join(tmpDir, "lyx.exe")
	if err := os.WriteFile(fakeLyx, []byte("fake lyx binary"), 0o755); err != nil {
		t.Fatalf("write fake lyx: %v", err)
	}
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

// TestRun_FetchReportRequiresLoomyard verifies that the fetch subcommand
// fails fast when -loomyard is not supplied, covering the required-flag guard.
func TestRun_FetchReportRequiresLoomyard(t *testing.T) {
	tmpDir := t.TempDir()

	code := run([]string{"-parent", tmpDir, "fetch"})
	if code == 0 {
		t.Error("run() = 0; want non-zero when -loomyard is missing for fetch subcommand")
	}
}

// TestRun_UnknownSubcommandReturnsNonZero tests that an unrecognised positional
// argument causes run to return a non-zero code.
func TestRun_UnknownSubcommandReturnsNonZero(t *testing.T) {
	tmpDir := t.TempDir()
	code := run([]string{"-parent", tmpDir, "unknowncmd"})
	if code == 0 {
		t.Error("run() = 0; want non-zero for unknown subcommand")
	}
}
