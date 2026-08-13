// gitkit.go implements the shared test-fixture builders and copy helpers used across worktree,
// weft, and paths tests.

package gitkit

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/weftname"
)

// MustRun runs a command in the specified directory, calling tb.Fatalf on failure.
func MustRun(tb testing.TB, dir string, args ...string) {
	tb.Helper()

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir

	if output, err := cmd.CombinedOutput(); err != nil {
		tb.Fatalf("command failed: %v; output: %s", err, output)
	}
}

// SeedConfig seeds real configuration into a git repository: creates _lyx/config, writes each
// module's YAML file, and commits them.
// The map parameter maps module names to YAML content.
// This preserves the leaf invariant by avoiding configreg imports.
func SeedConfig(tb testing.TB, repoDir string, configByModule map[string]string) {
	tb.Helper()

	// Create config directory if it doesn't exist.
	configDir := configengine.ConfigDir(repoDir)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		tb.Fatalf("mkdir config dir: %v", err)
	}

	// Write each module's config file.
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

// stripHookSamples removes all *.sample files from the given hooks directory
// (best-effort; errors are ignored).
func stripHookSamples(hooksDir string) {
	pattern := filepath.Join(hooksDir, "*.sample")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		// Ignore glob errors (best-effort).
		return
	}
	for _, match := range matches {
		_ = os.Remove(match)
	}
}

// initRepo initializes a git repository at dir on branch main, disabling
// fsmonitor and auto-maintenance (copies inherit .git/config). Panics on failure.
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
	if output, err := cmd.CombinedOutput(); err != nil {
		panic("git " + strings.Join(args, " ") + ": " + err.Error() + "; " + string(output))
	}
}

// warpHubTemplate caches the warp-hub template (git repo with bare origin, left empty).
var (
	warpHubOnce     sync.Once
	warpHubPath     string
	warpHubBarePath string
)

// buildWarpHub constructs the warp-hub template: a git repo with origin bare remote,
// populated with a README and initial commit (called once per test binary; panics on failure).
func buildWarpHub() (hub, bare string) {
	warpHubOnce.Do(func() {
		tmpDir, err := os.MkdirTemp("", "gitkit-repo-*")
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

		// Create bare remote and add it as origin (left empty; no push).
		bare := filepath.Join(tmpDir, "bare")
		initBareRemote(bare, hub)

		warpHubPath = hub
		warpHubBarePath = bare
	})

	return warpHubPath, warpHubBarePath
}

// weftPrimeTemplate caches the weft-prime template.
var (
	weftPrimeOnce     sync.Once
	weftPrimePath     string
	weftPrimeBarePath string
)

// buildWeftPrime constructs the weft-prime template: a sibling weft worktree
// at <hub>-weft with _lyx/config/placeholder and a bare remote (panics on failure).
func buildWeftPrime() (weftPrime, weftBare string) {
	weftPrimeOnce.Do(func() {
		// Derive the base name from the already-cached warp hub path so the naming
		// is stable across repeated calls (sync.Once skips the body on reuse).
		base := filepath.Base(warpHubPath)
		tmpDir, err := os.MkdirTemp("", "gitkit-weftprime-*")
		if err != nil {
			panic(err)
		}

		weftPrime := weftname.SiblingPath(tmpDir, base)
		if err := os.Mkdir(weftPrime, 0o755); err != nil {
			panic(err)
		}

		initRepo(weftPrime)

		// Create _lyx/config with neutral placeholder (no real config files).
		// Tests needing real config seed it via SeedConfig.
		lyxConfigDir := configengine.ConfigDir(weftPrime)
		if err := os.MkdirAll(lyxConfigDir, 0o755); err != nil {
			panic(err)
		}

		// Write a placeholder file to mark the config dir as initialized.
		placeholderPath := filepath.Join(lyxConfigDir, "placeholder")
		if err := os.WriteFile(placeholderPath, []byte("weft config"), 0o644); err != nil {
			panic(fmt.Sprintf("write placeholder: %v", err))
		}
		commitAll(weftPrime, "init")

		// Create bare remote and add it as origin (left empty; no push).
		weftBare := weftname.BareSiblingPath(tmpDir, base)
		initBareRemote(weftBare, weftPrime)

		weftPrimePath = weftPrime
		weftPrimeBarePath = weftBare
	})

	return weftPrimePath, weftPrimeBarePath
}

// weftOnlyTemplate caches the weft-only template (with upstream tracking).
var (
	weftOnlyOnce sync.Once
	weftOnlyPath string
	weftOnlyBare string
)

// buildWeftOnly constructs the weft-only template: a weft worktree with
// _lyx/config.yaml and upstream tracking (the only template that needs this; panics on failure).
func buildWeftOnly() (weftPath, bare string) {
	weftOnlyOnce.Do(func() {
		tmpDir, err := os.MkdirTemp("", "gitkit-weftonly-*")
		if err != nil {
			panic(err)
		}

		weftPath := tmpDir

		initRepo(weftPath)

		// Create _lyx/config.yaml — a single tracked file under _lyx so that
		// TestPushIntegration can commit the "_lyx" pathspec. This fixture only
		// needs some tracked file under _lyx, not a real config layout; tests that
		// need real config call SeedConfig after CopyWeft.
		lyxDir := filepath.Join(weftPath, lyxdirs.LyxDirName)
		if err := os.MkdirAll(lyxDir, 0o755); err != nil {
			panic(err)
		}

		if err := os.WriteFile(filepath.Join(lyxDir, "config.yaml"), []byte("test"), 0o644); err != nil {
			panic(err)
		}
		commitAll(weftPath, "init")

		// Create bare remote, add it as origin, then push with -u to establish
		// upstream tracking — this is the only fixture that needs the tracking branch.
		bare := filepath.Join(tmpDir, "bare")
		initBareRemote(bare, weftPath)
		mustGit(weftPath, "push", "-u", "origin", "main")

		weftOnlyPath = weftPath
		weftOnlyBare = bare
	})

	return weftOnlyPath, weftOnlyBare
}

// Fixture structs for public API.

// RepoFixture represents an isolated copy of the warp-hub template: a plain git repo with a bare
// origin, never a hub.
// It is the primitive repo fixture and is callable from internal/lyxcwd alone —
// every other package takes a real hub from internal/hubforge instead;
// see TestCopyRepoCallerSet_LyxcwdOnly.
type RepoFixture struct {
	Repo string
	Bare string
}

// WarpFixture represents an isolated copy of the warp-hub template (hub + bare).
//
// Deprecated: WarpFixture is scheduled for deletion once the hub-shaped CopyWarpHub call sites
// migrate to internal/hubforge.
type WarpFixture struct {
	Hub  string
	Bare string
}

// PairedFixture represents an isolated copy of the paired-Add fixture (warp hub + bare + weft-prime
// + weft-bare).
type PairedFixture struct {
	Container string
	Hub       string
	Bare      string
	WeftPrime string
	WeftBare  string
	Layout    *lyxcwd.Location
}

// WeftFixture represents an isolated copy of the weft-only template (with upstream tracking).
type WeftFixture struct {
	WeftPath string
	Bare     string
}

// rewriteOriginURLInConfig rewrites the origin URL in .git/config as a pure text edit
// (no subprocess). It asserts that exactly one url line exists under [remote "origin"].
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

// copyDirRecursive recursively copies a directory tree from src to dest (panics on symlinks).
func copyDirRecursive(src string, dest string) error {
	// Ensure destination parent exists.
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}

	// Create destination root.
	if err := os.Mkdir(dest, 0o755); err != nil {
		return err
	}

	// WalkDir does not follow symlinks on entry; we detect them explicitly so
	// that a symlink in a template causes an immediate, clear error rather than
	// silently producing a dangling link in the copy.
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

		// Copy regular file.
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

// CopyRepo returns an isolated copy of the warp-hub template: a plain git repo with a bare origin,
// never a hub.
// The copy is placed in tb.TempDir();
// its origin URL is rewritten to point to the copied bare repository.
// It is the primitive repo fixture and is callable from internal/lyxcwd alone;
// every other package takes a real hub from internal/hubforge instead.
func CopyRepo(tb testing.TB) RepoFixture {
	tb.Helper()

	templateHub, templateBare := buildWarpHub()

	// Use a single temp dir so both repos share one cleanup entry (matches CopyPaired).
	tempContainer := tb.TempDir()

	// Copy template hub into temp dir
	copiedHub := filepath.Join(tempContainer, "hub")
	if err := copyDirRecursive(templateHub, copiedHub); err != nil {
		tb.Fatalf("copyDirRecursive hub: %v", err)
	}

	// Copy template bare into the same temp dir
	copiedBare := filepath.Join(tempContainer, "bare")
	if err := copyDirRecursive(templateBare, copiedBare); err != nil {
		tb.Fatalf("copyDirRecursive bare: %v", err)
	}

	// Rewrite origin URL in copied hub's config
	if err := rewriteOriginURLInConfig(copiedHub, copiedBare); err != nil {
		tb.Fatalf("rewriteOriginURLInConfig: %v", err)
	}

	return RepoFixture{
		Repo: copiedHub,
		Bare: copiedBare,
	}
}

// CopyWarpHub returns an isolated copy of the warp-hub template, mapped onto the legacy WarpFixture
// shape.
//
// Deprecated: use CopyRepo instead;
// CopyWarpHub is scheduled for deletion once its hub-shaped call sites migrate to
// internal/hubforge.
func CopyWarpHub(tb testing.TB) WarpFixture {
	tb.Helper()

	fixture := CopyRepo(tb)
	return WarpFixture{
		Hub:  fixture.Repo,
		Bare: fixture.Bare,
	}
}

// CopyPaired returns an isolated copy of the full paired-Add fixture.
// The copy includes hub + bare + weft-prime + weft-bare.
// All origin URLs are rewritten to point to the copied bares.
func CopyPaired(tb testing.TB) PairedFixture {
	tb.Helper()

	templateHub, templateBare := buildWarpHub()
	templateWeftPrime, templateWeftBare := buildWeftPrime()

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
	copiedWeftPrime := weftname.SiblingPath(tempContainer, base)
	if err := copyDirRecursive(templateWeftPrime, copiedWeftPrime); err != nil {
		tb.Fatalf("copyDirRecursive weftPrime: %v", err)
	}

	// Copy weft-bare
	copiedWeftBare := weftname.BareSiblingPath(tempContainer, base)
	if err := copyDirRecursive(templateWeftBare, copiedWeftBare); err != nil {
		tb.Fatalf("copyDirRecursive weftBare: %v", err)
	}

	// Rewrite origin URLs
	if err := rewriteOriginURLInConfig(copiedHub, copiedBare); err != nil {
		tb.Fatalf("rewriteOriginURLInConfig hub: %v", err)
	}

	if err := rewriteOriginURLInConfig(copiedWeftPrime, copiedWeftBare); err != nil {
		tb.Fatalf("rewriteOriginURLInConfig weftPrime: %v", err)
	}

	// Get layout from copied hub
	layout, err := lyxcwd.Resolve(copiedHub)
	if err != nil {
		tb.Fatalf("lyxcwd.Resolve: %v", err)
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

// CopyPairedLocal returns an isolated copy of the paired-Add fixture optimized for SkipPush:true
// tests.
// It copies only the warp hub, warp bare, and weft-prime, omitting the weft-bare (unused when the
// weft push is suppressed).
// This reduces per-test filesystem-copy + Defender cost by ~25%.
// The returned fixture has Container, Hub, Bare, WeftPrime, and Layout populated, but WeftBare is
// left empty.
// Pushing the weft branch against this fixture is unsupported;
// use CopyPaired instead if the test exercises the weft-bare as a live push target.
func CopyPairedLocal(tb testing.TB) PairedFixture {
	tb.Helper()

	templateHub, templateBare := buildWarpHub()
	templateWeftPrime, _ := buildWeftPrime()

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

	// Copy weft-prime (must preserve the -weft suffix); omit weft-bare
	base := filepath.Base(templateHub)
	copiedWeftPrime := weftname.SiblingPath(tempContainer, base)
	if err := copyDirRecursive(templateWeftPrime, copiedWeftPrime); err != nil {
		tb.Fatalf("copyDirRecursive weftPrime: %v", err)
	}

	// Rewrite warp origin URL; do not rewrite weft-prime's origin URL
	// (it points at the shared template weft-bare and is never reached under SkipPush:true)
	if err := rewriteOriginURLInConfig(copiedHub, copiedBare); err != nil {
		tb.Fatalf("rewriteOriginURLInConfig hub: %v", err)
	}

	// Get layout from copied hub
	layout, err := lyxcwd.Resolve(copiedHub)
	if err != nil {
		tb.Fatalf("lyxcwd.Resolve: %v", err)
	}

	return PairedFixture{
		Container: tempContainer,
		Hub:       copiedHub,
		Bare:      copiedBare,
		WeftPrime: copiedWeftPrime,
		WeftBare:  "",
		Layout:    layout,
	}
}

// CopyWeft returns an isolated copy of the weft-only template.
// The copy is placed in tb.TempDir();
// its origin URL is rewritten and upstream tracking is already established (from the template).
func CopyWeft(tb testing.TB) WeftFixture {
	tb.Helper()

	templateWeftPath, templateBare := buildWeftOnly()

	// Use a single temp dir so both repos share one cleanup entry (matches CopyPaired).
	tempContainer := tb.TempDir()

	// Copy template weft into temp dir
	copiedWeft := filepath.Join(tempContainer, "weft")
	if err := copyDirRecursive(templateWeftPath, copiedWeft); err != nil {
		tb.Fatalf("copyDirRecursive weft: %v", err)
	}

	// Copy template bare into the same temp dir
	copiedBare := filepath.Join(tempContainer, "bare")
	if err := copyDirRecursive(templateBare, copiedBare); err != nil {
		tb.Fatalf("copyDirRecursive bare: %v", err)
	}

	// Rewrite origin URL in copied weft's config
	if err := rewriteOriginURLInConfig(copiedWeft, copiedBare); err != nil {
		tb.Fatalf("rewriteOriginURLInConfig: %v", err)
	}

	return WeftFixture{
		WeftPath: copiedWeft,
		Bare:     copiedBare,
	}
}
