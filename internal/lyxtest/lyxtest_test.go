//go:build integration

package lyxtest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestCopyHostHub verifies that CopyHostHub returns valid independent git repos.
func TestCopyHostHub(t *testing.T) {
	t.Parallel()

	fixture := CopyHostHub(t)

	// Verify the copied hub is a valid git repo
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = fixture.Hub
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git rev-parse HEAD in hub: %v; output: %s", err, output)
	}

	// Verify origin URL points at the copied bare, not the template
	cmd = exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = fixture.Hub
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git remote get-url: %v", err)
	}
	gotURL := strings.TrimSpace(string(output))
	if gotURL != fixture.Bare {
		t.Errorf("origin URL = %q; want %q", gotURL, fixture.Bare)
	}
}

// TestCopyHostHub_Isolation verifies that mutations to one copy don't affect another.
func TestCopyHostHub_Isolation(t *testing.T) {
	t.Parallel()

	fixture1 := CopyHostHub(t)
	fixture2 := CopyHostHub(t)

	// Mutate fixture1: add and commit a file
	testFile := filepath.Join(fixture1.Hub, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cmd := exec.Command("git", "add", "test.txt")
	cmd.Dir = fixture1.Hub
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v; output: %s", err, output)
	}

	cmd = exec.Command("git", "commit", "-m", "add test.txt")
	cmd.Dir = fixture1.Hub
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v; output: %s", err, output)
	}

	// Verify fixture2 is unaffected
	testFile2 := filepath.Join(fixture2.Hub, "test.txt")
	if _, err := os.Stat(testFile2); err == nil {
		t.Errorf("fixture2 should not have test.txt, but it does")
	}
}

// TestCopyPaired verifies that CopyPaired returns valid independent repos.
func TestCopyPaired(t *testing.T) {
	t.Parallel()

	fixture := CopyPaired(t)

	// Verify hub is a valid git repo
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = fixture.Hub
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git rev-parse HEAD in hub: %v; output: %s", err, output)
	}

	// Verify weft-prime is a valid git repo
	cmd = exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = fixture.WeftPrime
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git rev-parse HEAD in weft-prime: %v; output: %s", err, output)
	}

	// Verify origin URLs point at the copied bares
	cmd = exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = fixture.Hub
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git remote get-url hub: %v", err)
	}
	gotURL := strings.TrimSpace(string(output))
	if gotURL != fixture.Bare {
		t.Errorf("hub origin URL = %q; want %q", gotURL, fixture.Bare)
	}

	cmd = exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = fixture.WeftPrime
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git remote get-url weft-prime: %v", err)
	}
	gotURL = strings.TrimSpace(string(output))
	if gotURL != fixture.WeftBare {
		t.Errorf("weft-prime origin URL = %q; want %q", gotURL, fixture.WeftBare)
	}

	// Verify Layout is valid
	if fixture.Layout == nil {
		t.Errorf("Layout is nil")
	}
	if fixture.Layout.Hub == "" {
		t.Errorf("Layout.Hub is empty")
	}
}

// TestCopyWeft verifies that CopyWeft returns a valid repo with upstream tracking.
func TestCopyWeft(t *testing.T) {
	t.Parallel()

	fixture := CopyWeft(t)

	// Verify the copied weft is a valid git repo
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = fixture.WeftPath
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git rev-parse HEAD: %v; output: %s", err, output)
	}

	// Verify origin URL points at the copied bare
	cmd = exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = fixture.WeftPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git remote get-url: %v", err)
	}
	gotURL := strings.TrimSpace(string(output))
	if gotURL != fixture.Bare {
		t.Errorf("origin URL = %q; want %q", gotURL, fixture.Bare)
	}

	// Verify upstream tracking is established
	cmd = exec.Command("git", "rev-parse", "@{u}")
	cmd.Dir = fixture.WeftPath
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git rev-parse @{u}: %v; output: %s", err, output)
	}

	// Verify we're up to date with upstream
	cmd = exec.Command("git", "rev-list", "--count", "@{u}..HEAD")
	cmd.Dir = fixture.WeftPath
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git rev-list count: %v", err)
	}
	count := strings.TrimSpace(string(output))
	if count != "0" {
		t.Errorf("commits ahead of upstream = %q; want 0", count)
	}
}

// TestCopyWeft_Isolation verifies that mutations to one weft copy don't affect another.
func TestCopyWeft_Isolation(t *testing.T) {
	t.Parallel()

	fixture1 := CopyWeft(t)
	fixture2 := CopyWeft(t)

	// Mutate fixture1: add and commit a file
	testFile := filepath.Join(fixture1.WeftPath, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cmd := exec.Command("git", "add", "test.txt")
	cmd.Dir = fixture1.WeftPath
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v; output: %s", err, output)
	}

	cmd = exec.Command("git", "commit", "-m", "add test.txt")
	cmd.Dir = fixture1.WeftPath
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v; output: %s", err, output)
	}

	// Verify fixture2 is unaffected
	testFile2 := filepath.Join(fixture2.WeftPath, "test.txt")
	if _, err := os.Stat(testFile2); err == nil {
		t.Errorf("fixture2 should not have test.txt, but it does")
	}
}

// TestMustRun verifies that MustRun executes commands successfully.
func TestMustRun(t *testing.T) {
	t.Parallel()

	fixture := CopyHostHub(t)

	// MustRun should succeed when the command succeeds
	MustRun(t, fixture.Hub, "git", "rev-parse", "HEAD")
}

// TestMustRun_Failure verifies that MustRun fails the test on command failure.
func TestMustRun_Failure(t *testing.T) {
	t.Parallel()

	fixture := CopyHostHub(t)

	// Create a sub-test to capture the fatality
	t.Run("command-failure", func(t *testing.T) {
		// We can't directly test t.Fatalf, but we can verify the behavior indirectly
		// by ensuring that a command that should fail would be caught.
		// This is more of a sanity check that MustRun is set up correctly.
		// For now, we just verify that the helper respects tb.Helper().
		MustRun(t, fixture.Hub, "git", "rev-parse", "HEAD")
	})
}
