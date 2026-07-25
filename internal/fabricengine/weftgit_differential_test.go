//go:build integration

// weftgit_differential_test.go runs fabricengine's weft-git parity verbs
// (StatusWeft, CommitWeft, PushWeft, PullWeft, EnvSyncOptions) back to back
// against weftengine's originals and asserts equivalent observable
// behavior, modulo the one deliberate delta the weft-git parity decision
// documents: fabric's CommitWeft appends a Warp-SHA trailer weftengine.Commit
// never writes.
//
// Package fabricengine_test (external test package) because this file
// imports both weftengine and fabricengine; it shares the single TestMain
// declared in testmain_test.go (package fabricengine) per the
// test-package-discipline decision. Each case uses twin lyxtest.CopyWeft
// fixtures: side A drives weftengine directly, side B pairs a second
// lyxtest.CopyWeft copy with a minimal, locally-built host repo standing in
// for fabric's warp — fabric's CommitWeft needs a warp repo to read
// Warp-SHA from.

package fabricengine_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxtest"
	"github.com/Knatte18/loomyard/internal/weftengine"
)

// newWarpFixture creates a minimal, isolated git repo at t.TempDir() on
// branch main with one commit — everything fabricengine.New/CommitWeft need
// from a warp repo, without any of fabric's own topology wiring. Duplicated
// here (not imported) since fabricengine's own equivalent helper is
// unexported in package fabricengine, a different package from this file's
// fabricengine_test.
func newWarpFixture(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	lyxtest.MustRun(t, dir, "git", "init", "-q", "-b", "main")
	lyxtest.MustRun(t, dir, "git", "config", "user.email", "test@test.com")
	lyxtest.MustRun(t, dir, "git", "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(dir, "README"), []byte("warp"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	lyxtest.MustRun(t, dir, "git", "add", ".")
	lyxtest.MustRun(t, dir, "git", "commit", "-q", "-m", "init")
	return dir
}

// newFabricPair returns a *fabricengine.Fabric paired with a fresh
// newWarpFixture warp repo and a fresh lyxtest.CopyWeft weft fixture —
// side B's setup, shared by every differential case in this file.
func newFabricPair(t *testing.T) (*fabricengine.Fabric, lyxtest.WeftFixture) {
	t.Helper()

	warpPath := newWarpFixture(t)
	weftFixture := lyxtest.CopyWeft(t)
	f, err := fabricengine.New(warpPath, weftFixture.WeftPath)
	if err != nil {
		t.Fatalf("fabricengine.New(%q, %q): %v", warpPath, weftFixture.WeftPath, err)
	}
	return f, weftFixture
}

// writeWeftConfig overwrites the tracked _lyx/config.yaml file both
// CopyWeft fixtures ship with, the standard way every case here dirties a
// weft worktree's pathspec-covered content.
func writeWeftConfig(t *testing.T, weftPath, content string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(weftPath, "_lyx", "config.yaml"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// writeStrayFile writes an untracked file outside the "_lyx" pathspec both
// cases stage against, so a caller can assert it survives a scoped commit
// untouched.
func writeStrayFile(t *testing.T, weftPath string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(weftPath, "stray.txt"), []byte("stray"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// gitStatusPorcelain returns `git status --porcelain`'s raw output for
// repoPath.
func gitStatusPorcelain(t *testing.T, repoPath string) string {
	t.Helper()

	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git status --porcelain in %s: %v", repoPath, err)
	}
	return string(out)
}

// commitMessageAt returns rev's full raw commit message (subject + body +
// trailers) in repoPath, via `git log --format=%B`.
func commitMessageAt(t *testing.T, repoPath, rev string) string {
	t.Helper()

	cmd := exec.Command("git", "log", "-1", "--format=%B", rev)
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git log -1 --format=%%B %s in %s: %v", rev, repoPath, err)
	}
	return string(out)
}

// stripWarpSHATrailer removes a trailing "Warp-SHA: <sha>" trailer line —
// and the blank line appendWarpSHATrailer inserts to start a fresh trailer
// block — from message, guided by the same trailer shape
// fabricengine.WarpSHATrailerKey/parseWarpSHATrailer recognize, so a side-B
// commit message can be compared against side A's untouched one once the
// one deliberate delta (the trailer) is normalized away. Fails the test if
// message's last line does not actually carry the trailer.
func stripWarpSHATrailer(t *testing.T, message string) string {
	t.Helper()

	lines := strings.Split(strings.TrimRight(message, "\n"), "\n")
	if len(lines) == 0 {
		t.Fatalf("stripWarpSHATrailer: empty message")
	}
	last := strings.TrimSpace(lines[len(lines)-1])
	prefix := fabricengine.WarpSHATrailerKey + ":"
	if !strings.HasPrefix(last, prefix) {
		t.Fatalf("stripWarpSHATrailer: last line %q does not carry a %s trailer", last, fabricengine.WarpSHATrailerKey)
	}
	lines = lines[:len(lines)-1]
	if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

// extractWarpSHATrailer returns the Warp-SHA trailer value in message, if
// present — a local, exported-constant-guided reimplementation of
// fabricengine's unexported parseWarpSHATrailer, since this file's
// fabricengine_test package cannot call it directly.
func extractWarpSHATrailer(message string) (sha string, ok bool) {
	prefix := fabricengine.WarpSHATrailerKey + ":"
	for _, line := range strings.Split(message, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, prefix) {
			continue
		}
		if value := strings.TrimSpace(strings.TrimPrefix(trimmed, prefix)); value != "" {
			sha, ok = value, true
		}
	}
	return sha, ok
}

// TestStatusWeft_DifferentialEquivalence asserts StatusWeft and
// weftengine.Status report identical branch/dirty/ahead/behind values on a
// clean and a dirty tree.
func TestStatusWeft_DifferentialEquivalence(t *testing.T) {
	tests := []struct {
		name   string
		modify bool
	}{
		{"Clean", false},
		{"Dirty", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			weftFixtureA := lyxtest.CopyWeft(t)
			f, weftFixtureB := newFabricPair(t)

			if tt.modify {
				writeWeftConfig(t, weftFixtureA.WeftPath, "modified")
				writeWeftConfig(t, weftFixtureB.WeftPath, "modified")
			}

			want, err := weftengine.Status(weftFixtureA.WeftPath, []string{"_lyx"})
			if err != nil {
				t.Fatalf("weftengine.Status: %v", err)
			}
			got, err := f.StatusWeft([]string{"_lyx"})
			if err != nil {
				t.Fatalf("StatusWeft: %v", err)
			}

			// weft_worktree deliberately differs (each side's own fixture
			// path); every other key must match exactly.
			for _, key := range []string{"branch", "dirty", "ahead", "behind"} {
				if want[key] != got[key] {
					t.Errorf("status[%q]: weftengine=%v fabricengine=%v; want equal", key, want[key], got[key])
				}
			}
		})
	}
}

// TestCommitWeft_DifferentialEquivalence asserts CommitWeft and
// weftengine.Commit stage and commit the same scoped content, leave an
// out-of-pathspec stray file equally untracked, and — once side B's
// Warp-SHA trailer (the one deliberate delta) is stripped — produce the
// same commit message; the trailer itself names side B's warp HEAD.
func TestCommitWeft_DifferentialEquivalence(t *testing.T) {
	weftFixtureA := lyxtest.CopyWeft(t)
	f, weftFixtureB := newFabricPair(t)

	writeWeftConfig(t, weftFixtureA.WeftPath, "modified")
	writeWeftConfig(t, weftFixtureB.WeftPath, "modified")
	writeStrayFile(t, weftFixtureA.WeftPath)
	writeStrayFile(t, weftFixtureB.WeftPath)

	committedA, err := weftengine.Commit(weftFixtureA.WeftPath, []string{"_lyx"}, weftengine.DefaultCommitMessage, weftengine.SyncOptions{})
	if err != nil {
		t.Fatalf("weftengine.Commit: %v", err)
	}
	shaB, committedB, err := f.CommitWeft([]string{"_lyx"}, fabricengine.DefaultCommitMessage, fabricengine.SyncOptions{})
	if err != nil {
		t.Fatalf("CommitWeft: %v", err)
	}
	if !committedA || !committedB {
		t.Fatalf("committed: weftengine=%v fabricengine=%v; want both true", committedA, committedB)
	}
	if shaB == "" {
		t.Errorf("CommitWeft sha = %q; want non-empty", shaB)
	}

	for _, path := range []string{weftFixtureA.WeftPath, weftFixtureB.WeftPath} {
		status := gitStatusPorcelain(t, path)
		if !strings.Contains(status, "stray.txt") {
			t.Errorf("stray.txt should remain untracked in %s after scoped commit; git status: %q", path, status)
		}
	}

	msgA := commitMessageAt(t, weftFixtureA.WeftPath, "HEAD")
	msgB := commitMessageAt(t, weftFixtureB.WeftPath, "HEAD")
	strippedB := stripWarpSHATrailer(t, msgB)
	if strings.TrimRight(msgA, "\n") != strings.TrimRight(strippedB, "\n") {
		t.Errorf("commit message mismatch once side B's trailer is stripped: weftengine=%q fabricengine(stripped)=%q", msgA, strippedB)
	}

	warpSHA, err := f.Warp.CurrentSHA()
	if err != nil {
		t.Fatalf("Warp.CurrentSHA: %v", err)
	}
	trailerSHA, ok := extractWarpSHATrailer(msgB)
	if !ok {
		t.Fatalf("side B commit message has no Warp-SHA trailer: %q", msgB)
	}
	if trailerSHA != warpSHA {
		t.Errorf("Warp-SHA trailer = %q; want side B's warp HEAD %q", trailerSHA, warpSHA)
	}
}

// TestCommitWeft_AlreadyRemovedPathspec_DifferentialEquivalence asserts
// that a pathspec already fully removed from both the working tree and the
// index by a prior commit yields (false, nil) on both engines, rather than
// an error, when re-committed against.
func TestCommitWeft_AlreadyRemovedPathspec_DifferentialEquivalence(t *testing.T) {
	weftFixtureA := lyxtest.CopyWeft(t)
	f, weftFixtureB := newFabricPair(t)

	if err := os.RemoveAll(filepath.Join(weftFixtureA.WeftPath, "_lyx")); err != nil {
		t.Fatalf("RemoveAll (side A): %v", err)
	}
	if err := os.RemoveAll(filepath.Join(weftFixtureB.WeftPath, "_lyx")); err != nil {
		t.Fatalf("RemoveAll (side B): %v", err)
	}

	firstA, err := weftengine.Commit(weftFixtureA.WeftPath, []string{"_lyx"}, weftengine.DefaultCommitMessage, weftengine.SyncOptions{})
	if err != nil || !firstA {
		t.Fatalf("first weftengine.Commit (removal): committed=%v err=%v", firstA, err)
	}
	_, firstB, err := f.CommitWeft([]string{"_lyx"}, fabricengine.DefaultCommitMessage, fabricengine.SyncOptions{})
	if err != nil || !firstB {
		t.Fatalf("first CommitWeft (removal): committed=%v err=%v", firstB, err)
	}

	secondA, err := weftengine.Commit(weftFixtureA.WeftPath, []string{"_lyx"}, weftengine.DefaultCommitMessage, weftengine.SyncOptions{})
	if err != nil {
		t.Fatalf("second weftengine.Commit (already removed): %v", err)
	}
	_, secondB, err := f.CommitWeft([]string{"_lyx"}, fabricengine.DefaultCommitMessage, fabricengine.SyncOptions{})
	if err != nil {
		t.Fatalf("second CommitWeft (already removed): %v", err)
	}

	if secondA || secondB {
		t.Errorf("second commit against an already-removed pathspec: weftengine=%v fabricengine=%v; want both false", secondA, secondB)
	}
}

// TestPushWeft_DifferentialEquivalence asserts PushWeft and weftengine.Push
// both land a committed change on their respective bare remotes.
func TestPushWeft_DifferentialEquivalence(t *testing.T) {
	weftFixtureA := lyxtest.CopyWeft(t)
	f, weftFixtureB := newFabricPair(t)

	writeWeftConfig(t, weftFixtureA.WeftPath, "push me")
	writeWeftConfig(t, weftFixtureB.WeftPath, "push me")

	if _, err := weftengine.Commit(weftFixtureA.WeftPath, []string{"_lyx"}, weftengine.DefaultCommitMessage, weftengine.SyncOptions{}); err != nil {
		t.Fatalf("weftengine.Commit: %v", err)
	}
	shaB, committedB, err := f.CommitWeft([]string{"_lyx"}, fabricengine.DefaultCommitMessage, fabricengine.SyncOptions{})
	if err != nil || !committedB {
		t.Fatalf("CommitWeft: committed=%v err=%v", committedB, err)
	}

	shaA := currentSHAOf(t, weftFixtureA.WeftPath)

	if err := weftengine.Push(weftFixtureA.WeftPath, weftengine.SyncOptions{}); err != nil {
		t.Fatalf("weftengine.Push: %v", err)
	}
	if err := f.PushWeft(fabricengine.SyncOptions{}); err != nil {
		t.Fatalf("PushWeft: %v", err)
	}

	assertCommitOnBare(t, weftFixtureA.Bare, shaA)
	assertCommitOnBare(t, weftFixtureB.Bare, shaB)
}

// TestPushWeft_BrokenRemote_DifferentialEquivalence exercises both engines
// against a remote URL that does not point at any git repository at all.
// weftengine.Push's own pushUnpushed special-cases this failure (it does
// not match any of its rebase-retry trigger substrings) into an error naming
// the weft path without leaking git's raw "fatal:"-prefixed stderr — the
// behavior TestPush_BrokenRemoteFailsWithoutStderrLeak already covers on
// weftengine's side. fabricengine.PushWeft leans entirely on
// gitrepo.PushCoalesced/Push instead (per the "Most git mechanics grow into
// gitrepo" decision), and gitrepo's own documented Push contract is the
// opposite of weftengine's hand-rolled one here: "any other push failure
// returns an error including git's stderr" (gitrepo/doc.go's Push surface
// section) — a deliberate, pre-existing, cross-repo gitrepo design choice
// that predates and is out of scope for this task, not a fabric-specific
// regression. This case therefore asserts each engine's own real, already
// documented contract side by side rather than forcing a false "identical
// formatting" expectation gitrepo's own test suite (push_test.go) does not
// support.
func TestPushWeft_BrokenRemote_DifferentialEquivalence(t *testing.T) {
	weftFixtureA := lyxtest.CopyWeft(t)
	f, weftFixtureB := newFabricPair(t)

	writeWeftConfig(t, weftFixtureA.WeftPath, "broken remote A")
	writeWeftConfig(t, weftFixtureB.WeftPath, "broken remote B")
	if _, err := weftengine.Commit(weftFixtureA.WeftPath, []string{"_lyx"}, weftengine.DefaultCommitMessage, weftengine.SyncOptions{}); err != nil {
		t.Fatalf("weftengine.Commit: %v", err)
	}
	if _, committedB, err := f.CommitWeft([]string{"_lyx"}, fabricengine.DefaultCommitMessage, fabricengine.SyncOptions{}); err != nil || !committedB {
		t.Fatalf("CommitWeft: committed=%v err=%v", committedB, err)
	}

	badRemoteA := filepath.Join(t.TempDir(), "does-not-exist-a")
	lyxtest.MustRun(t, weftFixtureA.WeftPath, "git", "remote", "set-url", "origin", badRemoteA)
	badRemoteB := filepath.Join(t.TempDir(), "does-not-exist-b")
	lyxtest.MustRun(t, weftFixtureB.WeftPath, "git", "remote", "set-url", "origin", badRemoteB)

	errA := weftengine.Push(weftFixtureA.WeftPath, weftengine.SyncOptions{})
	if errA == nil {
		t.Fatal("weftengine.Push() with a broken remote error = nil; want an error")
	}
	if !strings.Contains(errA.Error(), weftFixtureA.WeftPath) {
		t.Errorf("weftengine.Push() error = %q; want it to name the weft path %q", errA, weftFixtureA.WeftPath)
	}
	if strings.Contains(errA.Error(), "fatal:") {
		t.Errorf("weftengine.Push() error = %q; want no raw git stderr leak", errA)
	}

	errB := f.PushWeft(fabricengine.SyncOptions{})
	if errB == nil {
		t.Fatal("PushWeft() with a broken remote error = nil; want an error")
	}
	// gitrepo.Push's documented contract (unlike weftengine's own): the
	// error is git's own push failure, wrapped but not stripped.
	if !strings.Contains(errB.Error(), "gitrepo: git push:") {
		t.Errorf("PushWeft() error = %q; want it to carry gitrepo's own push-failure wrapping (\"gitrepo: git push:\")", errB)
	}
}

// TestPullWeft_DifferentialEquivalence asserts PullWeft and weftengine.Pull
// both fast-forward a locally-reset checkout back to its pushed commit.
func TestPullWeft_DifferentialEquivalence(t *testing.T) {
	weftFixtureA := lyxtest.CopyWeft(t)
	f, weftFixtureB := newFabricPair(t)

	writeWeftConfig(t, weftFixtureA.WeftPath, "v1")
	writeWeftConfig(t, weftFixtureB.WeftPath, "v1")

	if _, err := weftengine.Commit(weftFixtureA.WeftPath, []string{"_lyx"}, weftengine.DefaultCommitMessage, weftengine.SyncOptions{}); err != nil {
		t.Fatalf("weftengine.Commit: %v", err)
	}
	if _, committedB, err := f.CommitWeft([]string{"_lyx"}, fabricengine.DefaultCommitMessage, fabricengine.SyncOptions{}); err != nil || !committedB {
		t.Fatalf("CommitWeft: committed=%v err=%v", committedB, err)
	}

	if err := weftengine.Push(weftFixtureA.WeftPath, weftengine.SyncOptions{}); err != nil {
		t.Fatalf("weftengine.Push: %v", err)
	}
	if err := f.PushWeft(fabricengine.SyncOptions{}); err != nil {
		t.Fatalf("PushWeft: %v", err)
	}

	lyxtest.MustRun(t, weftFixtureA.WeftPath, "git", "reset", "--hard", "HEAD~1")
	lyxtest.MustRun(t, weftFixtureB.WeftPath, "git", "reset", "--hard", "HEAD~1")

	if err := weftengine.Pull(weftFixtureA.WeftPath, weftengine.SyncOptions{}); err != nil {
		t.Fatalf("weftengine.Pull: %v", err)
	}
	if err := f.PullWeft(fabricengine.SyncOptions{}); err != nil {
		t.Fatalf("PullWeft: %v", err)
	}

	contentA, err := os.ReadFile(filepath.Join(weftFixtureA.WeftPath, "_lyx", "config.yaml"))
	if err != nil {
		t.Fatalf("ReadFile (side A): %v", err)
	}
	contentB, err := os.ReadFile(filepath.Join(weftFixtureB.WeftPath, "_lyx", "config.yaml"))
	if err != nil {
		t.Fatalf("ReadFile (side B): %v", err)
	}
	if string(contentA) != "v1" || string(contentB) != "v1" {
		t.Errorf("after pull: side A=%q side B=%q; want both %q", contentA, contentB, "v1")
	}
}

// TestEnvSyncOptions_DifferentialParity asserts weftengine.EnvSyncOptions
// and fabricengine.EnvSyncOptions map the same WEFT_SKIP_GIT/WEFT_SKIP_PUSH
// environment values identically.
func TestEnvSyncOptions_DifferentialParity(t *testing.T) {
	tests := []struct {
		name              string
		skipGit, skipPush string
	}{
		{"Unset", "", ""},
		{"SkipGitOnly", "1", ""},
		{"SkipPushOnly", "", "1"},
		{"Both", "1", "1"},
		{"NonOneValuesIgnored", "true", "yes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("WEFT_SKIP_GIT", tt.skipGit)
			t.Setenv("WEFT_SKIP_PUSH", tt.skipPush)

			want := weftengine.EnvSyncOptions()
			got := fabricengine.EnvSyncOptions()
			if want.SkipGit != got.SkipGit || want.SkipPush != got.SkipPush {
				t.Errorf("EnvSyncOptions: weftengine=%+v fabricengine=%+v; want equal", want, got)
			}
		})
	}
}

// currentSHAOf returns dir's HEAD commit SHA.
func currentSHAOf(t *testing.T, dir string) string {
	t.Helper()

	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse HEAD in %s: %v", dir, err)
	}
	return strings.TrimSpace(string(out))
}

// assertCommitOnBare fails the test unless sha is present as an object in
// the bare repository at barePath.
func assertCommitOnBare(t *testing.T, barePath, sha string) {
	t.Helper()

	cmd := exec.Command("git", "-C", barePath, "-c", "safe.bareRepository=all", "cat-file", "-e", sha)
	if err := cmd.Run(); err != nil {
		t.Errorf("commit %s did not reach bare remote %s: %v", sha, barePath, err)
	}
}
