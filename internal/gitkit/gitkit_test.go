//go:build integration

package gitkit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
)

// TestMain wires up the hermetic git environment before any test spawns git.
func TestMain(m *testing.M) {
	HermeticGitEnv()
	os.Exit(m.Run())
}

// TestHermeticGitEnv_QuietAndPinned verifies Layer B: a bare git init reads fsmonitor and branch
// from the hermetic env config, not the operator's config.
func TestHermeticGitEnv_QuietAndPinned(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	MustRun(t, dir, "git", "init")

	// The file is integration-tagged, so spawning git directly via exec.Command
	// for these two read-only assertions is legal here.
	cmd := exec.Command("git", "config", "core.fsmonitor")
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git config core.fsmonitor: %v; output: %s", err, output)
	}
	if got := strings.TrimSpace(string(output)); got != "false" {
		t.Errorf("core.fsmonitor = %q; want %q", got, "false")
	}

	cmd = exec.Command("git", "symbolic-ref", "HEAD")
	cmd.Dir = dir
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git symbolic-ref HEAD: %v; output: %s", err, output)
	}
	if got := strings.TrimSpace(string(output)); got != "refs/heads/main" {
		t.Errorf("symbolic-ref HEAD = %q; want %q", got, "refs/heads/main")
	}
}

// TestTemplateQuietConfig verifies Layer A: Copy* fixtures carry quiet git settings in their own
// .git/config, independent of the hermetic env.
func TestTemplateQuietConfig(t *testing.T) {
	t.Parallel()

	fixture := CopyRepo(t)

	cmd := exec.Command("git", "config", "--local", "core.fsmonitor")
	cmd.Dir = fixture.Repo
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git config --local core.fsmonitor: %v; output: %s", err, output)
	}
	if got := strings.TrimSpace(string(output)); got != "false" {
		t.Errorf("--local core.fsmonitor = %q; want %q", got, "false")
	}
}

// TestCopyRepo verifies that CopyRepo returns valid independent git repos.
func TestCopyRepo(t *testing.T) {
	t.Parallel()

	fixture := CopyRepo(t)

	// Verify the copied repo is a valid git repo
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = fixture.Repo
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git rev-parse HEAD in repo: %v; output: %s", err, output)
	}

	// Verify origin URL points at the copied bare, not the template.
	// Normalize to forward slashes: git returns forward-slash paths on Windows
	// while filepath.Join uses backslashes; both are equivalent local paths.
	cmd = exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = fixture.Repo
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git remote get-url: %v", err)
	}
	gotURL := filepath.ToSlash(strings.TrimSpace(string(output)))
	if gotURL != filepath.ToSlash(fixture.Bare) {
		t.Errorf("origin URL = %q; want %q", gotURL, filepath.ToSlash(fixture.Bare))
	}
}

// TestCopyRepo_Isolation verifies that fixture copies are isolated.
func TestCopyRepo_Isolation(t *testing.T) {
	t.Parallel()

	fixture1 := CopyRepo(t)
	fixture2 := CopyRepo(t)

	// Mutate fixture1: add and commit a file
	testFile := filepath.Join(fixture1.Repo, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cmd := exec.Command("git", "add", "test.txt")
	cmd.Dir = fixture1.Repo
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v; output: %s", err, output)
	}

	cmd = exec.Command("git", "commit", "-m", "add test.txt")
	cmd.Dir = fixture1.Repo
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v; output: %s", err, output)
	}

	// Verify fixture2 is unaffected
	testFile2 := filepath.Join(fixture2.Repo, "test.txt")
	if _, err := os.Stat(testFile2); err == nil {
		t.Errorf("fixture2 should not have test.txt, but it does")
	}
}

// TestMustRun verifies that MustRun executes commands successfully.
func TestMustRun(t *testing.T) {
	t.Parallel()

	fixture := CopyRepo(t)

	// MustRun should succeed when the command succeeds
	MustRun(t, fixture.Repo, "git", "rev-parse", "HEAD")
}

// TestMustRun_Failure verifies that MustRun calls tb.Fatalf on failure using the subprocess pattern
// to confirm non-zero exit.
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
	fixture := CopyRepo(t)

	// Re-invoke this test as a subprocess; the -tags flag must match the current build.
	cmd := exec.Command(os.Args[0], "-test.run=^TestMustRun_Failure$", "-test.v")
	cmd.Env = append(os.Environ(),
		"GO_TEST_SUBPROCESS=MUSTRUN_FAILURE",
		"GO_TEST_SUBPROCESS_DIR="+fixture.Repo,
	)
	err := cmd.Run()
	if err == nil {
		t.Errorf("subprocess passed; expected MustRun to call Fatalf and exit non-zero")
	}
}

// TestCopyDirRecursive_RefusesSymlinks verifies that copyDirRecursive returns an error instead of
// following or copying a symlink planted in the source tree.
func TestCopyDirRecursive_RefusesSymlinks(t *testing.T) {
	t.Parallel()

	src := t.TempDir()
	target := filepath.Join(src, "target.txt")
	if err := os.WriteFile(target, []byte("target"), 0o644); err != nil {
		t.Fatalf("WriteFile target: %v", err)
	}

	link := filepath.Join(src, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	dest := filepath.Join(t.TempDir(), "dest")
	err := copyDirRecursive(src, dest)
	if err == nil {
		t.Fatal("copyDirRecursive: expected error for symlink in source tree, got nil")
	}
	if !strings.Contains(err.Error(), "symlink not allowed") {
		t.Errorf("copyDirRecursive error = %q; want it to mention symlink refusal", err.Error())
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
	module1Path := configengine.ConfigFile(tmpDir, "module1")
	content1, err := os.ReadFile(module1Path)
	if err != nil {
		t.Fatalf("read module1.yaml: %v", err)
	}
	if string(content1) != configContent {
		t.Errorf("module1 content = %q; want %q", string(content1), configContent)
	}

	module2Path := configengine.ConfigFile(tmpDir, "module2")
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
