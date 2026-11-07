// gitkit.go holds the shared test fixtures: repo builders, git spawn helpers, and repo copy helpers.

package gitkit

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/lymtemp"
)

// FixtureGitEnv is the environment variable that marks a git process as fixture setup, not code under test.
// A marked child carries the value "1".
const FixtureGitEnv = "LYM_FIXTURE_GIT"

// fixtureGitEnviron returns the process environment with the fixture marker appended.
func fixtureGitEnviron() []string {
	return append(os.Environ(), FixtureGitEnv+"=1")
}

// GitStatusPorcelain returns `git status --porcelain`'s raw output for repoPath and calls tb.Fatalf on failure.
// Non-empty output means the worktree has uncommitted changes: staged, unstaged, or untracked.
func GitStatusPorcelain(tb testing.TB, repoPath string) string {
	tb.Helper()

	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = repoPath
	cmd.Env = fixtureGitEnviron()
	out, err := cmd.Output()
	if err != nil {
		tb.Fatalf("git status --porcelain in %s: %v", repoPath, err)
	}
	return string(out)
}

// MustRun runs a command in dir, calling tb.Fatalf on failure.
func MustRun(tb testing.TB, dir string, args ...string) {
	tb.Helper()

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	cmd.Env = fixtureGitEnviron()

	if output, err := cmd.CombinedOutput(); err != nil {
		tb.Fatalf("command failed: %v; output: %s", err, output)
	}
}

// Git runs `git <args>` in dir and returns its trimmed stdout, calling tb.Fatalf on a spawn error or non-zero exit.
// The query helpers spawn git only through Git and gitExit.
func Git(tb testing.TB, dir string, args ...string) string {
	tb.Helper()

	out, code, err := gitExit(dir, args...)
	if err != nil {
		tb.Fatalf("git %s in %s: %v", strings.Join(args, " "), dir, err)
	}
	if code != 0 {
		tb.Fatalf("git %s in %s: exit %d; output: %s", strings.Join(args, " "), dir, code, out)
	}
	return strings.TrimSpace(out)
}

// gitExit runs `git <args>` in dir and returns its stdout and exit code.
// A non-zero exit is reported through the code, not err; err is set only when git could not be run.
func gitExit(dir string, args ...string) (stdout string, code int, err error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = fixtureGitEnviron()
	out, runErr := cmd.Output()
	if runErr == nil {
		return string(out), 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		return string(out) + string(exitErr.Stderr), exitErr.ExitCode(), nil
	}
	return "", -1, runErr
}

// SeedConfig seeds real configuration into a git repository.
// It creates the config directory, writes each module's YAML file, and commits them.
// configByModule pairs each module name with its YAML content.
// It does not import configreg, which keeps this package a leaf.
func SeedConfig(tb testing.TB, repoDir string, configByModule map[string]string) {
	tb.Helper()

	configDir := configengine.ConfigDir(repoDir)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		tb.Fatalf("mkdir config dir: %v", err)
	}

	for module, content := range configByModule {
		configPath := configengine.ConfigFile(repoDir, module)
		if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
			tb.Fatalf("write config file %s: %v", module, err)
		}
	}

	// Stage and commit the config files so they are checked out in the worktree.
	MustRun(tb, repoDir, "git", "add", ".")
	MustRun(tb, repoDir, "git", "commit", "-m", "seed config")
}

// Template builders: cached, built once per test binary via sync.Once.

// stripHookSamples removes the *.sample files from hooksDir.
// Errors are ignored, so the removal is best-effort.
func stripHookSamples(hooksDir string) {
	pattern := filepath.Join(hooksDir, "*.sample")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return
	}
	for _, match := range matches {
		_ = os.Remove(match)
	}
}

// initRepo initializes a git repository at dir on branch main, disabling fsmonitor and auto-maintenance.
// Copies inherit .git/config, so these settings reach every copy.
// It panics on failure.
func initRepo(dir string) {
	mustGit(dir, "init", "-b", "main")
	mustGit(dir, "config", "user.email", "test@test.com")
	mustGit(dir, "config", "user.name", "Test")
	mustGit(dir, "config", "core.fsmonitor", "false")
	mustGit(dir, "config", "maintenance.auto", "false")
	mustGit(dir, "config", "gc.auto", "0")
	stripHookSamples(filepath.Join(dir, ".git", "hooks"))
}

// commitAll stages every change in dir and creates a commit (panics on failure).
func commitAll(dir, message string) {
	mustGit(dir, "add", ".")
	mustGit(dir, "commit", "-m", message)
}

// initBareRemote creates a bare git repository at dir and adds it as origin.
// It disables fsmonitor and auto-maintenance. Panics on failure.
func initBareRemote(dir, repoDir string) {
	if err := os.Mkdir(dir, 0o755); err != nil {
		panic(err)
	}
	mustGit(dir, "init", "--bare")
	mustGit(dir, "config", "core.fsmonitor", "false")
	mustGit(dir, "config", "maintenance.auto", "false")
	mustGit(dir, "config", "gc.auto", "0")
	stripHookSamples(filepath.Join(dir, "hooks"))
	mustGit(repoDir, "remote", "add", "origin", dir)
}

// mustGit runs a git subcommand in dir, panicking on non-zero exit.
func mustGit(dir string, args ...string) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = fixtureGitEnviron()
	if output, err := cmd.CombinedOutput(); err != nil {
		panic("git " + strings.Join(args, " ") + ": " + err.Error() + "; " + string(output))
	}
}

// The repo template and its bare origin are built once per test binary and cached in these variables.
var (
	repoTemplateOnce     sync.Once
	repoTemplatePath     string
	repoTemplateBarePath string
)

// buildRepoTemplate constructs the repo template: a git repo whose initial commit holds a README.
// It also adds a bare origin remote and runs once per test binary; it panics on failure.
func buildRepoTemplate() (hub, bare string) {
	repoTemplateOnce.Do(func() {
		tmpDir, err := lymtemp.MkdirTemp("gitkit-repo-*")
		if err != nil {
			panic(err)
		}

		hub := filepath.Join(tmpDir, "hub")
		if err := os.Mkdir(hub, 0o755); err != nil {
			panic(err)
		}

		initRepo(hub)

		// Write and commit README to give the repo a non-empty history.
		if err := os.WriteFile(filepath.Join(hub, "README"), []byte("test"), 0o644); err != nil {
			panic(err)
		}
		commitAll(hub, "init")

		// The bare remote stays empty: nothing is pushed to it.
		bare := filepath.Join(tmpDir, "bare")
		initBareRemote(bare, hub)

		repoTemplatePath = hub
		repoTemplateBarePath = bare
	})

	return repoTemplatePath, repoTemplateBarePath
}

// Fixture structs for public API.

// RepoFixture is an isolated copy of the repo template: a plain git repo with a bare origin, not a hub.
// Only internal/lymcwd may use it, and a test enforces that.
type RepoFixture struct {
	Repo string
	Bare string
}

// rewriteOriginURLInConfig rewrites the origin URL in .git/config as a pure text edit, with no subprocess.
// It asserts that exactly one url line exists under [remote "origin"].
func rewriteOriginURLInConfig(repoPath string, newURL string) error {
	configPath := filepath.Join(repoPath, ".git", "config")

	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read .git/config: %w", err)
	}

	lines := strings.Split(string(data), "\n")

	// Track whether we are inside the [remote "origin"] section so we only
	// replace the url line belonging to origin, not any other remote.
	inOriginSection := false
	matchCount := 0

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Detect section headers; entering a new section ends the origin block.
		if strings.HasPrefix(trimmed, "[") {
			inOriginSection = trimmed == `[remote "origin"]`
			continue
		}

		if inOriginSection && strings.HasPrefix(trimmed, "url = ") {
			// Preserve the leading whitespace from the original line.
			leading := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
			// Git config encodes backslashes as \\; use forward slashes instead,
			// which git accepts on Windows for local paths.
			forwardSlashURL := filepath.ToSlash(newURL)
			lines[i] = leading + "url = " + forwardSlashURL
			matchCount++
		}
	}

	// Exactly one url line must exist under [remote "origin"]; the template
	// invariant guarantees this. More or fewer would indicate a corrupt config.
	if matchCount != 1 {
		return fmt.Errorf("rewriteOriginURLInConfig: expected exactly 1 url line under [remote \"origin\"], found %d in %s", matchCount, configPath)
	}

	updated := strings.Join(lines, "\n")
	if err := os.WriteFile(configPath, []byte(updated), 0o644); err != nil {
		return fmt.Errorf("write .git/config: %w", err)
	}

	return nil
}

// copyDirRecursive copies the directory tree src into dest, and returns an error on a symlink.
func copyDirRecursive(src string, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}

	if err := os.Mkdir(dest, 0o755); err != nil {
		return err
	}

	// WalkDir does not follow symlinks on entry.
	// The check below fails at once on a symlink in a template, rather than leave a dangling link in the copy.
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		destPath := filepath.Join(dest, rel)

		// Refuse symlinks: template trees must be plain files/dirs only.
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("copyDirRecursive: symlink not allowed in template: %s", path)
		}

		if d.IsDir() {
			info, err := d.Info()
			if err != nil {
				return err
			}
			return os.MkdirAll(destPath, info.Mode())
		}

		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		destFile, err := os.Create(destPath)
		if err != nil {
			return err
		}
		defer destFile.Close()

		if _, err := io.Copy(destFile, srcFile); err != nil {
			return err
		}

		info, err := d.Info()
		if err != nil {
			return err
		}
		return os.Chmod(destPath, info.Mode())
	})
}

// CopyRepo returns an isolated copy of the repo template: a plain git repo with a bare origin, not a hub.
// The copy is placed in tb.TempDir(), and its origin URL points to the copied bare repository.
// Only internal/lymcwd may call it, and a test enforces that.
func CopyRepo(tb testing.TB) RepoFixture {
	tb.Helper()

	templateHub, templateBare := buildRepoTemplate()

	// Use a single temp dir so both repos share one cleanup entry.
	tempContainer := tb.TempDir()

	copiedHub := filepath.Join(tempContainer, "hub")
	if err := copyDirRecursive(templateHub, copiedHub); err != nil {
		tb.Fatalf("copyDirRecursive hub: %v", err)
	}

	copiedBare := filepath.Join(tempContainer, "bare")
	if err := copyDirRecursive(templateBare, copiedBare); err != nil {
		tb.Fatalf("copyDirRecursive bare: %v", err)
	}

	if err := rewriteOriginURLInConfig(copiedHub, copiedBare); err != nil {
		tb.Fatalf("rewriteOriginURLInConfig: %v", err)
	}

	return RepoFixture{
		Repo: copiedHub,
		Bare: copiedBare,
	}
}
