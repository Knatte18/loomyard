//go:build integration

package gitkit

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// TestMain wires up the hermetic git environment before any test spawns git, then runs the tests under tmuxkit.Main.
func TestMain(m *testing.M) {
	HermeticGitEnv()
	os.Exit(tmuxkit.Main(m))
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

// TestCopiedRepoScenario drives one CopyRepo fixture through the fixture and query helpers.
// The steps run serially in one order and share the fixture's repository state: the query steps start on the fixture's main branch, and the branches-and-ancestry step switches the checkout to the side branch last.
// The top-level test calls t.Parallel; no step does, because the steps share the fixture.
func TestCopiedRepoScenario(t *testing.T) {
	t.Parallel()

	fixture := CopyRepo(t)
	repo := fixture.Repo

	if !t.Run("repo and origin are the copy's own", func(t *testing.T) {
		MustRun(t, repo, "git", "rev-parse", "HEAD")

		// Origin points at the copied bare, not the template.
		// Normalize to forward slashes: git returns forward-slash paths on Windows while filepath.Join uses backslashes; both are equivalent local paths.
		gotURL := filepath.ToSlash(Git(t, repo, "remote", "get-url", "origin"))
		if gotURL != filepath.ToSlash(fixture.Bare) {
			t.Errorf("origin URL = %q; want %q", gotURL, filepath.ToSlash(fixture.Bare))
		}
	}) {
		return
	}

	// Layer A: Copy* fixtures carry quiet git settings in their own .git/config, independent of the hermetic env.
	if !t.Run("template quiet config", func(t *testing.T) {
		if got := Git(t, repo, "config", "--local", "core.fsmonitor"); got != "false" {
			t.Errorf("--local core.fsmonitor = %q; want %q", got, "false")
		}
	}) {
		return
	}

	if !t.Run("git returns trimmed stdout", func(t *testing.T) {
		if got := Git(t, repo, "rev-parse", "--abbrev-ref", "HEAD"); got != "main" {
			t.Errorf("Git rev-parse --abbrev-ref HEAD = %q; want main", got)
		}
	}) {
		return
	}

	if !t.Run("commit file", func(t *testing.T) {
		base := RevParse(t, repo, "HEAD")

		sha := CommitFile(t, repo, "dir/file.txt", "content", "add file")
		if got := RevParse(t, repo, "HEAD"); got != sha {
			t.Errorf("RevParse(HEAD) = %q; CommitFile returned %q", got, sha)
		}
		if got := RevListCount(t, repo, base+"..HEAD"); got != 1 {
			t.Errorf("RevListCount(base..HEAD) = %d; want 1", got)
		}
		if got := LsFiles(t, repo, "dir"); !slices.Equal(got, []string{"dir/file.txt"}) {
			t.Errorf("LsFiles(dir) = %v; want [dir/file.txt]", got)
		}
	}) {
		return
	}

	if !t.Run("exclude lines of a linked worktree", func(t *testing.T) {
		wt := filepath.Join(t.TempDir(), "wt")
		Git(t, repo, "worktree", "add", "-b", "linked", wt)

		infoDir := filepath.Join(repo, ".git", "info")
		if err := os.MkdirAll(infoDir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", infoDir, err)
		}
		if err := os.WriteFile(filepath.Join(infoDir, "exclude"), []byte("one\n\ntwo\n"), 0o644); err != nil {
			t.Fatalf("write exclude: %v", err)
		}

		if got := ExcludeLines(t, wt); !slices.Equal(got, []string{"one", "two"}) {
			t.Errorf("ExcludeLines(linked worktree) = %v; want [one two]", got)
		}
	}) {
		return
	}

	// Relies on the earlier steps leaving the checkout on main with the commit from "commit file".
	if !t.Run("branches and ancestry", func(t *testing.T) {
		base := RevParse(t, repo, "HEAD")

		if got := CurrentBranch(t, repo); got != "main" {
			t.Errorf("CurrentBranch = %q; want main", got)
		}
		if !BranchExists(t, repo, "main") {
			t.Error("BranchExists(main) = false; want true")
		}
		if BranchExists(t, repo, "missing") {
			t.Error("BranchExists(missing) = true; want false")
		}

		Git(t, repo, "branch", "side")
		tip := CommitFileOnBranch(t, repo, "side", "a/b.txt", "x", "side commit")

		if got := CurrentBranch(t, repo); got != "side" {
			t.Errorf("CurrentBranch after CommitFileOnBranch = %q; want side", got)
		}
		if !IsAncestor(t, repo, base, tip) {
			t.Error("IsAncestor(base, tip) = false; want true")
		}
		if IsAncestor(t, repo, tip, base) {
			t.Error("IsAncestor(tip, base) = true; want false")
		}
	}) {
		return
	}

	// A second copy never sees a file committed to the first.
	t.Run("copies are isolated", func(t *testing.T) {
		other := CopyRepo(t)
		if _, err := os.Stat(filepath.Join(other.Repo, "dir", "file.txt")); err == nil {
			t.Errorf("second copy has dir/file.txt committed to the first")
		}
	})
}

// TestFixtureGitMarker_ReachesHelperChildren verifies that git spawned by Git carries the fixture marker, read back through git's trace2 env-var events.
// It sets process environment variables through t.Setenv, so it does not call t.Parallel.
func TestFixtureGitMarker_ReachesHelperChildren(t *testing.T) {
	traceDir := t.TempDir()
	t.Setenv("GIT_TRACE2_EVENT", traceDir)
	t.Setenv("GIT_TRACE2_ENV_VARS", FixtureGitEnv)

	Git(t, t.TempDir(), "version")

	files, err := os.ReadDir(traceDir)
	if err != nil {
		t.Fatalf("read trace dir: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("trace dir holds %d event files; want 1", len(files))
	}
	events, err := os.ReadFile(filepath.Join(traceDir, files[0].Name()))
	if err != nil {
		t.Fatalf("read event file: %v", err)
	}

	var found bool
	for _, line := range strings.Split(string(events), "\n") {
		if strings.Contains(line, `"event":"def_param"`) && strings.Contains(line, `"param":"`+FixtureGitEnv+`"`) && strings.Contains(line, `"value":"1"`) {
			found = true
		}
	}
	if !found {
		t.Errorf("trace2 events carry no def_param for %s=1:\n%s", FixtureGitEnv, events)
	}
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
