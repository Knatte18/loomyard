// lyxtest.go implements the shared test-fixture builders and copy helpers
// used across worktree, weft, and paths tests.

package lyxtest

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

// MustRun runs a command with the given arguments in the specified directory.
// It calls tb.Fatalf if the command returns a non-zero exit code.
// Call tb.Helper() is delegated to the caller.
func MustRun(tb testing.TB, dir string, args ...string) {
	tb.Helper()

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir

	if output, err := cmd.CombinedOutput(); err != nil {
		tb.Fatalf("command failed: %v; output: %s", err, output)
	}
}

// Template builders: cached, built once per test binary via sync.Once.

// hostHubTemplate caches the host-hub template (git repo with bare origin, left empty).
var (
	hostHubOnce     sync.Once
	hostHubPath     string
	hostHubBarePath string
)

// buildHostHub constructs the host-hub template: a git repo with origin bare remote,
// populated with a README and initial commit. The bare remote is left empty
// (not pushed to), matching the worktree "AddOptions{SkipPush:true}" semantics.
// This is called once per test binary via sync.Once; subsequent calls return the cached path.
func buildHostHub() (hub, bare string, err error) {
	hostHubOnce.Do(func() {
		tmpDir, err := os.MkdirTemp("", "lyxtest-hosthub-*")
		if err != nil {
			panic(err)
		}

		hub := filepath.Join(tmpDir, "hub")
		if err := os.Mkdir(hub, 0o755); err != nil {
			panic(err)
		}

		// Initialize git repo
		cmd := exec.Command("git", "init", "-b", "main")
		cmd.Dir = hub
		if output, err := cmd.CombinedOutput(); err != nil {
			panic("git init: " + err.Error() + "; " + string(output))
		}

		// Configure git user
		cmd = exec.Command("git", "config", "user.email", "test@test.com")
		cmd.Dir = hub
		if output, err := cmd.CombinedOutput(); err != nil {
			panic("git config user.email: " + err.Error() + "; " + string(output))
		}

		cmd = exec.Command("git", "config", "user.name", "Test")
		cmd.Dir = hub
		if output, err := cmd.CombinedOutput(); err != nil {
			panic("git config user.name: " + err.Error() + "; " + string(output))
		}

		// Write and commit README
		if err := os.WriteFile(filepath.Join(hub, "README"), []byte("test"), 0o644); err != nil {
			panic(err)
		}

		cmd = exec.Command("git", "add", ".")
		cmd.Dir = hub
		if output, err := cmd.CombinedOutput(); err != nil {
			panic("git add: " + err.Error() + "; " + string(output))
		}

		cmd = exec.Command("git", "commit", "-m", "init")
		cmd.Dir = hub
		if output, err := cmd.CombinedOutput(); err != nil {
			panic("git commit: " + err.Error() + "; " + string(output))
		}

		// Create bare remote
		bare := filepath.Join(tmpDir, "bare")
		if err := os.Mkdir(bare, 0o755); err != nil {
			panic(err)
		}

		cmd = exec.Command("git", "init", "--bare")
		cmd.Dir = bare
		if output, err := cmd.CombinedOutput(); err != nil {
			panic("git init --bare: " + err.Error() + "; " + string(output))
		}

		// Add remote to hub (leave bare empty)
		cmd = exec.Command("git", "remote", "add", "origin", bare)
		cmd.Dir = hub
		if output, err := cmd.CombinedOutput(); err != nil {
			panic("git remote add: " + err.Error() + "; " + string(output))
		}

		hostHubPath = hub
		hostHubBarePath = bare
	})

	return hostHubPath, hostHubBarePath, nil
}

// weftPrimeTemplate caches the weft-prime template.
var (
	weftPrimeOnce     sync.Once
	weftPrimePath     string
	weftPrimeBarePath string
)

// buildWeftPrime constructs the weft-prime template: a sibling weft worktree
// at <base>-weft with _lyx/config/placeholder, plus a bare remote left empty.
func buildWeftPrime(hubPath string) (weftPrime, weftBare string, err error) {
	weftPrimeOnce.Do(func() {
		container := filepath.Dir(hubPath)
		base := filepath.Base(hubPath)
		tmpDir, err := os.MkdirTemp("", "lyxtest-weftprime-*")
		if err != nil {
			panic(err)
		}

		weftPrime := filepath.Join(tmpDir, base+"-weft")
		if err := os.Mkdir(weftPrime, 0o755); err != nil {
			panic(err)
		}

		// Initialize git repo
		cmd := exec.Command("git", "init", "-b", "main")
		cmd.Dir = weftPrime
		if output, err := cmd.CombinedOutput(); err != nil {
			panic("git init: " + err.Error() + "; " + string(output))
		}

		// Configure git user
		cmd = exec.Command("git", "config", "user.email", "test@test.com")
		cmd.Dir = weftPrime
		if output, err := cmd.CombinedOutput(); err != nil {
			panic("git config user.email: " + err.Error() + "; " + string(output))
		}

		cmd = exec.Command("git", "config", "user.name", "Test")
		cmd.Dir = weftPrime
		if output, err := cmd.CombinedOutput(); err != nil {
			panic("git config user.name: " + err.Error() + "; " + string(output))
		}

		// Create _lyx/config/placeholder structure
		lyxConfigDir := filepath.Join(weftPrime, "_lyx", "config")
		if err := os.MkdirAll(lyxConfigDir, 0o755); err != nil {
			panic(err)
		}

		if err := os.WriteFile(filepath.Join(lyxConfigDir, "placeholder"), []byte("weft config"), 0o644); err != nil {
			panic(err)
		}

		// Commit
		cmd = exec.Command("git", "add", ".")
		cmd.Dir = weftPrime
		if output, err := cmd.CombinedOutput(); err != nil {
			panic("git add: " + err.Error() + "; " + string(output))
		}

		cmd = exec.Command("git", "commit", "-m", "init")
		cmd.Dir = weftPrime
		if output, err := cmd.CombinedOutput(); err != nil {
			panic("git commit: " + err.Error() + "; " + string(output))
		}

		// Create bare remote
		weftBare := filepath.Join(tmpDir, base+"-weft-bare")
		if err := os.Mkdir(weftBare, 0o755); err != nil {
			panic(err)
		}

		cmd = exec.Command("git", "init", "--bare")
		cmd.Dir = weftBare
		if output, err := cmd.CombinedOutput(); err != nil {
			panic("git init --bare: " + err.Error() + "; " + string(output))
		}

		// Add remote (leave empty)
		cmd = exec.Command("git", "remote", "add", "origin", weftBare)
		cmd.Dir = weftPrime
		if output, err := cmd.CombinedOutput(); err != nil {
			panic("git remote add: " + err.Error() + "; " + string(output))
		}

		weftPrimePath = weftPrime
		weftPrimeBarePath = weftBare
	})

	return weftPrimePath, weftPrimeBarePath, nil
}

// weftOnlyTemplate caches the weft-only template (with upstream tracking).
var (
	weftOnlyOnce sync.Once
	weftOnlyPath string
	weftOnlyBare string
)

// buildWeftOnly constructs the weft-only template: a weft worktree with
// _lyx/config.yaml and upstream tracking (push -u origin main).
// This is the only template that needs upstream tracking.
func buildWeftOnly() (weftPath, bare string, err error) {
	weftOnlyOnce.Do(func() {
		tmpDir, err := os.MkdirTemp("", "lyxtest-weftonly-*")
		if err != nil {
			panic(err)
		}

		weftPath := tmpDir

		// Initialize git repo
		cmd := exec.Command("git", "init", "-b", "main")
		cmd.Dir = weftPath
		if output, err := cmd.CombinedOutput(); err != nil {
			panic("git init: " + err.Error() + "; " + string(output))
		}

		// Configure git user
		cmd = exec.Command("git", "config", "user.email", "test@test.com")
		cmd.Dir = weftPath
		if output, err := cmd.CombinedOutput(); err != nil {
			panic("git config user.email: " + err.Error() + "; " + string(output))
		}

		cmd = exec.Command("git", "config", "user.name", "Test")
		cmd.Dir = weftPath
		if output, err := cmd.CombinedOutput(); err != nil {
			panic("git config user.name: " + err.Error() + "; " + string(output))
		}

		// Create _lyx/config.yaml
		lyxDir := filepath.Join(weftPath, "_lyx")
		if err := os.MkdirAll(lyxDir, 0o755); err != nil {
			panic(err)
		}

		if err := os.WriteFile(filepath.Join(lyxDir, "config.yaml"), []byte("test"), 0o644); err != nil {
			panic(err)
		}

		// Commit
		cmd = exec.Command("git", "add", ".")
		cmd.Dir = weftPath
		if output, err := cmd.CombinedOutput(); err != nil {
			panic("git add: " + err.Error() + "; " + string(output))
		}

		cmd = exec.Command("git", "commit", "-m", "init")
		cmd.Dir = weftPath
		if output, err := cmd.CombinedOutput(); err != nil {
			panic("git commit: " + err.Error() + "; " + string(output))
		}

		// Create bare remote
		bare := filepath.Join(tmpDir, "bare")
		if err := os.Mkdir(bare, 0o755); err != nil {
			panic(err)
		}

		cmd = exec.Command("git", "init", "--bare")
		cmd.Dir = bare
		if output, err := cmd.CombinedOutput(); err != nil {
			panic("git init --bare: " + err.Error() + "; " + string(output))
		}

		// Add remote
		cmd = exec.Command("git", "remote", "add", "origin", bare)
		cmd.Dir = weftPath
		if output, err := cmd.CombinedOutput(); err != nil {
			panic("git remote add: " + err.Error() + "; " + string(output))
		}

		// Push main with -u to establish upstream tracking
		cmd = exec.Command("git", "push", "-u", "origin", "main")
		cmd.Dir = weftPath
		if output, err := cmd.CombinedOutput(); err != nil {
			panic("git push -u: " + err.Error() + "; " + string(output))
		}

		weftOnlyPath = weftPath
		weftOnlyBare = bare
	})

	return weftOnlyPath, weftOnlyBare, nil
}
