// lyxtest.go implements the shared test-fixture builders and copy helpers
// used across worktree, weft, and paths tests.

package lyxtest

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/Knatte18/loomyard/internal/paths"
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

// Fixture structs for public API.

// HostFixture represents an isolated copy of the host-hub template.
type HostFixture struct {
	Hub  string
	Bare string
}

// PairedFixture represents an isolated copy of the full paired-Add fixture
// (host hub + bare + weft-prime sibling + weft bare).
type PairedFixture struct {
	Container string
	Hub       string
	Bare      string
	WeftPrime string
	WeftBare  string
	Layout    *paths.Layout
}

// WeftFixture represents an isolated copy of the weft-only template
// (with upstream tracking established).
type WeftFixture struct {
	WeftPath string
	Bare     string
}

// Helper: rewrite origin URL in a copied repository.
// Uses git remote set-url to update the origin URL safely.
func rewriteOriginURL(repoPath string, newURL string) error {
	cmd := exec.Command("git", "remote", "set-url", "origin", newURL)
	cmd.Dir = repoPath

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git remote set-url: %v; output: %s", err, output)
	}

	return nil
}

// Helper: recursively copy a directory tree.
// The source tree is copied entirely into the destination directory,
// which must not exist beforehand.
func copyDirRecursive(src string, dest string) error {
	// Ensure destination parent exists
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}

	// Create destination
	if err := os.Mkdir(dest, 0o755); err != nil {
		return err
	}

	// Walk the source tree
	return filepath.Walk(src, func(path string, d os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		destPath := filepath.Join(dest, rel)

		if d.IsDir() {
			return os.MkdirAll(destPath, d.Mode())
		}

		// Copy file
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

		return os.Chmod(destPath, d.Mode())
	})
}

// CopyHostHub returns an isolated copy of the host-hub template.
// The copy is placed in tb.TempDir(); its origin URL is rewritten to point
// to the copied bare repository.
func CopyHostHub(tb testing.TB) HostFixture {
	tb.Helper()

	templateHub, templateBare, err := buildHostHub()
	if err != nil {
		tb.Fatalf("buildHostHub: %v", err)
	}

	// Copy template hub into temp dir
	copiedHub := filepath.Join(tb.TempDir(), "hub")
	if err := copyDirRecursive(templateHub, copiedHub); err != nil {
		tb.Fatalf("copyDirRecursive hub: %v", err)
	}

	// Copy template bare into temp dir
	copiedBare := filepath.Join(tb.TempDir(), "bare")
	if err := copyDirRecursive(templateBare, copiedBare); err != nil {
		tb.Fatalf("copyDirRecursive bare: %v", err)
	}

	// Rewrite origin URL in copied hub's config
	if err := rewriteOriginURL(copiedHub, copiedBare); err != nil {
		tb.Fatalf("rewriteOriginURL: %v", err)
	}

	return HostFixture{
		Hub:  copiedHub,
		Bare: copiedBare,
	}
}

// CopyPaired returns an isolated copy of the full paired-Add fixture.
// The copy includes hub + bare + weft-prime + weft-bare.
// All origin URLs are rewritten to point to the copied bares.
func CopyPaired(tb testing.TB) PairedFixture {
	tb.Helper()

	templateHub, templateBare, err := buildHostHub()
	if err != nil {
		tb.Fatalf("buildHostHub: %v", err)
	}

	templateWeftPrime, templateWeftBare, err := buildWeftPrime(templateHub)
	if err != nil {
		tb.Fatalf("buildWeftPrime: %v", err)
	}

	// Create a temp container
	tempContainer := tb.TempDir()

	// Copy hub
	copiedHub := filepath.Join(tempContainer, "hub")
	if err := copyDirRecursive(templateHub, copiedHub); err != nil {
		tb.Fatalf("copyDirRecursive hub: %v", err)
	}

	// Copy bare
	copiedBare := filepath.Join(tempContainer, "bare")
	if err := copyDirRecursive(templateBare, copiedBare); err != nil {
		tb.Fatalf("copyDirRecursive bare: %v", err)
	}

	// Copy weft-prime (must preserve the -weft suffix)
	base := filepath.Base(templateHub)
	copiedWeftPrime := filepath.Join(tempContainer, base+"-weft")
	if err := copyDirRecursive(templateWeftPrime, copiedWeftPrime); err != nil {
		tb.Fatalf("copyDirRecursive weftPrime: %v", err)
	}

	// Copy weft-bare
	copiedWeftBare := filepath.Join(tempContainer, base+"-weft-bare")
	if err := copyDirRecursive(templateWeftBare, copiedWeftBare); err != nil {
		tb.Fatalf("copyDirRecursive weftBare: %v", err)
	}

	// Rewrite origin URLs
	if err := rewriteOriginURL(copiedHub, copiedBare); err != nil {
		tb.Fatalf("rewriteOriginURL hub: %v", err)
	}

	if err := rewriteOriginURL(copiedWeftPrime, copiedWeftBare); err != nil {
		tb.Fatalf("rewriteOriginURL weftPrime: %v", err)
	}

	// Get layout from copied hub
	layout, err := paths.Resolve(copiedHub)
	if err != nil {
		tb.Fatalf("paths.Resolve: %v", err)
	}

	return PairedFixture{
		Container: tempContainer,
		Hub:       copiedHub,
		Bare:      copiedBare,
		WeftPrime: copiedWeftPrime,
		WeftBare:  copiedWeftBare,
		Layout:    layout,
	}
}

// CopyWeft returns an isolated copy of the weft-only template.
// The copy is placed in tb.TempDir(); its origin URL is rewritten and
// upstream tracking is already established (from the template).
func CopyWeft(tb testing.TB) WeftFixture {
	tb.Helper()

	templateWeftPath, templateBare, err := buildWeftOnly()
	if err != nil {
		tb.Fatalf("buildWeftOnly: %v", err)
	}

	// Copy template weft into temp dir
	copiedWeft := filepath.Join(tb.TempDir(), "weft")
	if err := copyDirRecursive(templateWeftPath, copiedWeft); err != nil {
		tb.Fatalf("copyDirRecursive weft: %v", err)
	}

	// Copy template bare into temp dir
	copiedBare := filepath.Join(tb.TempDir(), "bare")
	if err := copyDirRecursive(templateBare, copiedBare); err != nil {
		tb.Fatalf("copyDirRecursive bare: %v", err)
	}

	// Rewrite origin URL in copied weft's config
	if err := rewriteOriginURL(copiedWeft, copiedBare); err != nil {
		tb.Fatalf("rewriteOriginURL: %v", err)
	}

	return WeftFixture{
		WeftPath: copiedWeft,
		Bare:     copiedBare,
	}
}
