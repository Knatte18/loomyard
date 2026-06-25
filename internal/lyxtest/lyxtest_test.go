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

	// Verify origin URL points at the copied bare, not the template.
	// Normalize to forward slashes: git returns forward-slash paths on Windows
	// while filepath.Join uses backslashes; both are equivalent local paths.
	cmd = exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = fixture.Hub
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git remote get-url: %v", err)
	}
	gotURL := filepath.ToSlash(strings.TrimSpace(string(output)))
	if gotURL != filepath.ToSlash(fixture.Bare) {
		t.Errorf("origin URL = %q; want %q", gotURL, filepath.ToSlash(fixture.Bare))
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

	// Verify origin URLs point at the copied bares.
	// Normalize to forward slashes: git returns forward-slash paths on Windows
	// while filepath.Join uses backslashes; both are equivalent local paths.
	cmd = exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = fixture.Hub
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git remote get-url hub: %v", err)
	}
	gotURL := filepath.ToSlash(strings.TrimSpace(string(output)))
	if gotURL != filepath.ToSlash(fixture.Bare) {
		t.Errorf("hub origin URL = %q; want %q", gotURL, filepath.ToSlash(fixture.Bare))
	}

	cmd = exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = fixture.WeftPrime
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git remote get-url weft-prime: %v", err)
	}
	gotURL = filepath.ToSlash(strings.TrimSpace(string(output)))
	if gotURL != filepath.ToSlash(fixture.WeftBare) {
		t.Errorf("weft-prime origin URL = %q; want %q", gotURL, filepath.ToSlash(fixture.WeftBare))
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

	// Verify origin URL points at the copied bare.
	// Normalize to forward slashes: git returns forward-slash paths on Windows
	// while filepath.Join uses backslashes; both are equivalent local paths.
	cmd = exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = fixture.WeftPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git remote get-url: %v", err)
	}
	gotURL := filepath.ToSlash(strings.TrimSpace(string(output)))
	if gotURL != filepath.ToSlash(fixture.Bare) {
		t.Errorf("origin URL = %q; want %q", gotURL, filepath.ToSlash(fixture.Bare))
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

// TestMustRun_Failure verifies that MustRun calls tb.Fatalf on a non-zero exit.
// It uses the subprocess pattern: when GO_TEST_SUBPROCESS=MUSTRUN_FAILURE the test
// binary runs MustRun directly (which calls t.Fatalf and exits non-zero). The parent
// process asserts the subprocess exited non-zero, confirming Fatalf was reached.
func TestMustRun_Failure(t *testing.T) {
	t.Parallel()

	// Subprocess mode: called by the parent test; run the failing command and exit.
	// MustRun calls t.Fatalf which causes runtime.Goexit and a non-zero exit code.
	if os.Getenv("GO_TEST_SUBPROCESS") == "MUSTRUN_FAILURE" {
		dir := os.Getenv("GO_TEST_SUBPROCESS_DIR")
		MustRun(t, dir, "git", "rev-parse", "no-such-ref-xyz")
		return
	}

	// Build a fixture so the subprocess has a valid git repo to run against.
	fixture := CopyHostHub(t)

	// Re-invoke this test as a subprocess; the -tags flag must match the current build.
	cmd := exec.Command(os.Args[0], "-test.run=^TestMustRun_Failure$", "-test.v")
	cmd.Env = append(os.Environ(),
		"GO_TEST_SUBPROCESS=MUSTRUN_FAILURE",
		"GO_TEST_SUBPROCESS_DIR="+fixture.Hub,
	)
	err := cmd.Run()
	if err == nil {
		t.Errorf("subprocess passed; expected MustRun to call Fatalf and exit non-zero")
	}
}

// TestSeedConfig verifies that SeedConfig writes config files and commits them.
func TestSeedConfig(t *testing.T) {
	t.Parallel()

	// Create a temp git repo to seed
	tmpDir := t.TempDir()
	MustRun(t, tmpDir, "git", "init", "-b", "main")
	MustRun(t, tmpDir, "git", "config", "user.email", "test@test.com")
	MustRun(t, tmpDir, "git", "config", "user.name", "Test")

	// Seed config
	configContent := "test_key: test_value\n"
	SeedConfig(t, tmpDir, map[string]string{
		"module1": configContent,
		"module2": "other: value\n",
	})

	// Verify files exist with correct content
	module1Path := filepath.Join(tmpDir, "_lyx", "config", "module1.yaml")
	content1, err := os.ReadFile(module1Path)
	if err != nil {
		t.Fatalf("read module1.yaml: %v", err)
	}
	if string(content1) != configContent {
		t.Errorf("module1 content = %q; want %q", string(content1), configContent)
	}

	module2Path := filepath.Join(tmpDir, "_lyx", "config", "module2.yaml")
	content2, err := os.ReadFile(module2Path)
	if err != nil {
		t.Fatalf("read module2.yaml: %v", err)
	}
	if string(content2) != "other: value\n" {
		t.Errorf("module2 content = %q; want %q", string(content2), "other: value\n")
	}

	// Verify files are tracked in git
	cmd := exec.Command("git", "ls-files")
	cmd.Dir = tmpDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git ls-files: %v; output: %s", err, output)
	}
	lsOutput := string(output)
	if !strings.Contains(lsOutput, "_lyx/config/module1.yaml") {
		t.Errorf("module1.yaml not in git ls-files: %s", lsOutput)
	}
	if !strings.Contains(lsOutput, "_lyx/config/module2.yaml") {
		t.Errorf("module2.yaml not in git ls-files: %s", lsOutput)
	}

	// Verify working tree is clean (all committed)
	cmd = exec.Command("git", "status", "--porcelain")
	cmd.Dir = tmpDir
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git status: %v", err)
	}
	if string(output) != "" {
		t.Errorf("git status not clean after SeedConfig: %s", string(output))
	}
}

// TestCopyPaired_NeutralFixture verifies that CopyPaired produces a neutral fixture
// with a placeholder file and no real config files.
func TestCopyPaired_NeutralFixture(t *testing.T) {
	t.Parallel()

	fixture := CopyPaired(t)

	// Verify the weft-prime contains _lyx/config/placeholder
	placeholderPath := filepath.Join(fixture.WeftPrime, "_lyx", "config", "placeholder")
	placeholderContent, err := os.ReadFile(placeholderPath)
	if err != nil {
		t.Fatalf("read placeholder: %v", err)
	}
	if string(placeholderContent) != "weft config" {
		t.Errorf("placeholder content = %q; want %q", string(placeholderContent), "weft config")
	}

	// Verify the weft-prime does NOT contain real config files (e.g., weft.yaml)
	weftConfigPath := filepath.Join(fixture.WeftPrime, "_lyx", "config", "weft.yaml")
	if _, err := os.Stat(weftConfigPath); !os.IsNotExist(err) {
		if err == nil {
			t.Errorf("weft.yaml should not exist in neutral fixture, but it does")
		} else {
			t.Errorf("unexpected error checking weft.yaml: %v", err)
		}
	}
}
