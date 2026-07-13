//go:build integration

// gitquery_test.go exercises HeadSHA, ChangedFiles, Dirty, and ResetHard
// against a real scratch git repo (Tier 2 — see docs/benchmarks/running-
// tests.md), per the discussion's "gitexec test pattern": t.TempDir(),
// `git init`, committer identity configured, commits made via
// gitexec.RunGit.

package builderengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/builderengine"
	"github.com/Knatte18/loomyard/internal/gitexec"
)

// newScratchRepo initializes a fresh git repo in a t.TempDir() and
// configures a throwaway committer identity, returning its path. Every
// gitquery/chain test needing real git history builds on this rather than
// lyxtest's weft/host fixtures, since these helpers exercise a plain
// worktree path, not lyx's junction-paired geometry.
func newScratchRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	mustGit(t, dir, "init")
	mustGit(t, dir, "config", "user.name", "Test User")
	mustGit(t, dir, "config", "user.email", "test@example.com")

	return dir
}

// mustGit runs a git command in dir via gitexec.RunGit, failing the test on
// any spawn error or non-zero exit, and returns stdout.
func mustGit(t *testing.T, dir string, args ...string) string {
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

// commitFile writes name=content into dir and commits it with message,
// returning the resulting commit SHA.
func commitFile(t *testing.T, dir, name, content, message string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", name, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	mustGit(t, dir, "add", name)
	mustGit(t, dir, "commit", "-m", message)
	return strings.TrimSpace(mustGit(t, dir, "rev-parse", "HEAD"))
}

func TestHeadSHA(t *testing.T) {
	t.Parallel()

	dir := newScratchRepo(t)
	want := commitFile(t, dir, "a.txt", "one", "first")

	got, err := builderengine.HeadSHA(dir)
	if err != nil {
		t.Fatalf("HeadSHA() error = %v; want nil", err)
	}
	if got != want {
		t.Errorf("HeadSHA() = %q; want %q", got, want)
	}
}

func TestChangedFiles(t *testing.T) {
	t.Parallel()

	dir := newScratchRepo(t)
	sinceSHA := commitFile(t, dir, "base.txt", "base", "base commit")
	commitFile(t, dir, "b.txt", "b", "add b")
	commitFile(t, dir, filepath.Join("sub", "c.txt"), "c", "add sub/c")

	got, err := builderengine.ChangedFiles(dir, sinceSHA)
	if err != nil {
		t.Fatalf("ChangedFiles() error = %v; want nil", err)
	}

	want := []string{"b.txt", "sub/c.txt"}
	if len(got) != len(want) {
		t.Fatalf("ChangedFiles() = %v; want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("ChangedFiles()[%d] = %q; want %q", i, got[i], want[i])
		}
	}
}

func TestDirty(t *testing.T) {
	t.Parallel()

	dir := newScratchRepo(t)
	commitFile(t, dir, "a.txt", "one", "first")

	clean, err := builderengine.Dirty(dir)
	if err != nil {
		t.Fatalf("Dirty() error = %v; want nil", err)
	}
	if clean {
		t.Errorf("Dirty() = true right after a commit with no other changes; want false")
	}

	if err := os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write untracked file: %v", err)
	}

	dirty, err := builderengine.Dirty(dir)
	if err != nil {
		t.Fatalf("Dirty() error = %v; want nil", err)
	}
	if !dirty {
		t.Errorf("Dirty() = false with an untracked file present; want true")
	}
}

func TestResetHard(t *testing.T) {
	t.Parallel()

	dir := newScratchRepo(t)
	anchor := commitFile(t, dir, "a.txt", "one", "first")
	commitFile(t, dir, "b.txt", "two", "second")

	if err := builderengine.ResetHard(dir, anchor); err != nil {
		t.Fatalf("ResetHard() error = %v; want nil", err)
	}

	head, err := builderengine.HeadSHA(dir)
	if err != nil {
		t.Fatalf("HeadSHA() error = %v; want nil", err)
	}
	if head != anchor {
		t.Errorf("HeadSHA() after ResetHard = %q; want anchor %q", head, anchor)
	}
	if _, err := os.Stat(filepath.Join(dir, "b.txt")); !os.IsNotExist(err) {
		t.Errorf("b.txt still present after ResetHard to before its commit")
	}
}
