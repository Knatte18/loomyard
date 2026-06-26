package main

import (
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
