//go:build integration

// gitwrap_test.go exercises headSHA and dirty against real
// scratch git repositories built fresh under t.TempDir() for each test,
// reusing the package's existing hermetic TestMain (testmain_test.go) so
// these git spawns never inherit the operator's global gitconfig.

package websterengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitexec"
)

// gitwrapNewScratchRepo initializes a fresh git repo in a t.TempDir() and
// configures a throwaway committer identity, returning its path.
func gitwrapNewScratchRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	gitwrapMustGit(t, dir, "init")
	gitwrapMustGit(t, dir, "config", "user.name", "Test User")
	gitwrapMustGit(t, dir, "config", "user.email", "test@example.com")

	return dir
}

// gitwrapMustGit runs a git command in dir via gitexec.RunGit, failing the
// test on any spawn error or non-zero exit, and returns stdout.
func gitwrapMustGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	stdout, stderr, exitCode, err := gitexec.RunGit(args, dir)
	if err != nil {
		t.Fatalf("git %v in %s: %v", args, dir, err)
	}
	if exitCode != 0 {
		t.Fatalf("git %v in %s exited %d: %s", args, dir, exitCode, stderr)
	}
	return stdout
}

// gitwrapCommitFile writes name=content into dir and commits it with
// message, returning the resulting commit SHA.
func gitwrapCommitFile(t *testing.T, dir, name, content, message string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", name, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	gitwrapMustGit(t, dir, "add", name)
	gitwrapMustGit(t, dir, "commit", "-m", message)
	return strings.TrimSpace(gitwrapMustGit(t, dir, "rev-parse", "HEAD"))
}

func TestHeadSHA_ReturnsHEAD(t *testing.T) {
	t.Parallel()

	dir := gitwrapNewScratchRepo(t)
	want := gitwrapCommitFile(t, dir, "a.txt", "one", "first")

	got, err := headSHA(dir)
	if err != nil {
		t.Fatalf("headSHA() error = %v; want nil", err)
	}
	if got != want {
		t.Errorf("headSHA() = %q; want %q", got, want)
	}
}

func TestDirty_TrueAndFalse(t *testing.T) {
	t.Parallel()

	dir := gitwrapNewScratchRepo(t)
	gitwrapCommitFile(t, dir, "a.txt", "one", "first")

	clean, err := dirty(dir)
	if err != nil {
		t.Fatalf("dirty() error = %v; want nil", err)
	}
	if clean {
		t.Errorf("dirty() = true right after a commit with no other changes; want false")
	}

	if err := os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write untracked file: %v", err)
	}

	isDirty, err := dirty(dir)
	if err != nil {
		t.Fatalf("dirty() error = %v; want nil", err)
	}
	if !isDirty {
		t.Errorf("dirty() = false with an untracked file present; want true")
	}
}
