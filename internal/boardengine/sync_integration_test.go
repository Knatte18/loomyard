//go:build integration

// sync_integration_test.go covers boardengine.Sync's parity after delegating
// its absorbing-lock coalescing loop to fabricengine.CoalescePush: a dirty
// board still commits and pushes, .gitignore is still seeded with the
// lock/manifest patterns, the absorbing push lock still lives at the
// unchanged board.push.lock path, and skipPush still commits locally without
// advancing the remote. The board+bare-origin fixture is built inline via
// gitkit.MustRun, mirroring the fixture shape of
// internal/gitrepo/push_test.go's newBareRemote/newRepoWithRemote —
// reimplemented locally since those helpers live in a different test
// package (gitrepo_test) and are not importable across that boundary.

package boardengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/lock"
)

// newBareRemote creates a bare git repository for testing.
func newBareRemote(t *testing.T, dir string) string {
	t.Helper()

	bare := filepath.Join(dir, "remote.git")
	if err := os.Mkdir(bare, 0o755); err != nil {
		t.Fatalf("mkdir bare remote: %v", err)
	}
	gitkit.MustRun(t, bare, "git", "init", "--bare", "-b", "main")
	return bare
}

// newBoardRepo creates a fresh git repository in state before its very first sync.
func newBoardRepo(t *testing.T, dir, name, bareRemote string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", name, err)
	}
	gitkit.MustRun(t, path, "git", "init", "-b", "main")
	gitkit.MustRun(t, path, "git", "remote", "add", "origin", bareRemote)
	return path
}

// writeBoardFile creates or overwrites a file to dirty the working tree for Sync.
func writeBoardFile(t *testing.T, dir, name, content string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// bareRemoteHead returns the bare remote's main branch SHA, or "" if nothing was pushed.
func bareRemoteHead(t *testing.T, bareRemote string) string {
	t.Helper()

	stdout, _, code, err := gitexec.RunGit([]string{"rev-parse", "main"}, bareRemote)
	if err != nil {
		t.Fatalf("git rev-parse main (bare) error = %v", err)
	}
	if code != 0 {
		return ""
	}
	return strings.TrimSpace(stdout)
}

// syncFixture is the one bare remote and board repository the steps of TestSync_Scenario share.
type syncFixture struct {
	bareRemote string
	boardPath  string
}

// TestSync_Scenario runs Sync's contract against one board repository and bare remote, in the order below.
// It calls t.Parallel and no step does: the steps share the one repository, so they run serially.
// The skip-push step runs first because it asserts the remote is still empty,
// each later step relies on the repository the earlier steps left behind,
// and the retire-legacy step runs last because it replaces the legacy files the earlier steps wrote.
func TestSync_Scenario(t *testing.T) {
	t.Parallel()
	container := t.TempDir()
	bareRemote := newBareRemote(t, container)
	fixture := &syncFixture{bareRemote: bareRemote, boardPath: newBoardRepo(t, container, "board", bareRemote)}

	steps := []struct {
		name string
		run  func(t *testing.T, f *syncFixture)
	}{
		{"SkipPush_CommitsLocallyButDoesNotPush", stepSyncSkipPushCommitsLocallyButDoesNotPush},
		{"DirtyBoard_CommitsAndPushes", stepSyncDirtyBoardCommitsAndPushes},
		{"SeedsGitignoreWithLockAndManifestPatterns", stepSyncSeedsGitignoreWithLockAndManifestPatterns},
		{"ConcurrentCallSerializesOnBoardPushLock", stepSyncConcurrentCallSerializesOnBoardPushLock},
		{"RetireLegacy_CommitsBoardJSONAndLegacyDeletions", stepSyncRetireLegacyCommitsBoardJSONAndLegacyDeletions},
	}
	for _, step := range steps {
		if !t.Run(step.name, func(t *testing.T) { step.run(t, fixture) }) {
			return
		}
	}
}

// stepSyncSkipPushCommitsLocallyButDoesNotPush asserts skipPush commits locally but doesn't push.
func stepSyncSkipPushCommitsLocallyButDoesNotPush(t *testing.T, f *syncFixture) {
	writeBoardFile(t, f.boardPath, "tasks.json", `{"tasks":[]}`)

	if err := Sync(f.boardPath, false, true); err != nil {
		t.Fatalf("Sync() (skipPush) error = %v; want nil", err)
	}

	if got := bareRemoteHead(t, f.bareRemote); got != "" {
		t.Errorf("bare remote main after skipPush Sync() = %q; want \"\" (nothing pushed)", got)
	}

	stdout, stderr, code, err := gitexec.RunGit([]string{"log", "--oneline"}, f.boardPath)
	if err != nil {
		t.Fatalf("git log error = %v", err)
	}
	if code != 0 {
		t.Fatalf("git log exited %d: %s", code, stderr)
	}
	if strings.TrimSpace(stdout) == "" {
		t.Fatal("git log --oneline after skipPush Sync() = \"\"; want a local commit")
	}
}

// stepSyncDirtyBoardCommitsAndPushes asserts a dirty board commits and pushes.
func stepSyncDirtyBoardCommitsAndPushes(t *testing.T, f *syncFixture) {
	writeBoardFile(t, f.boardPath, "tasks.json", `{"tasks":["dirty"]}`)

	if err := Sync(f.boardPath, false, false); err != nil {
		t.Fatalf("Sync() error = %v; want nil", err)
	}

	if got := bareRemoteHead(t, f.bareRemote); got == "" {
		t.Fatal("bare remote main after Sync() = \"\"; want the dirty commit pushed")
	}
}

// stepSyncSeedsGitignoreWithLockAndManifestPatterns asserts Sync seeded .gitignore with lock and
// manifest patterns.
func stepSyncSeedsGitignoreWithLockAndManifestPatterns(t *testing.T, f *syncFixture) {
	got, err := os.ReadFile(filepath.Join(f.boardPath, ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	for _, pat := range []string{"*.lock", "*.swaplock", renderManifestFile} {
		if !strings.Contains(string(got), pat) {
			t.Errorf(".gitignore = %q; want it to contain %q", got, pat)
		}
	}
}

// stepSyncConcurrentCallSerializesOnBoardPushLock asserts concurrent Sync calls serialize on
// board.push.lock.
func stepSyncConcurrentCallSerializesOnBoardPushLock(t *testing.T, f *syncFixture) {
	held, err := lock.AcquireWriteLock(filepath.Join(f.boardPath, "board.push.lock"))
	if err != nil {
		t.Fatalf("AcquireWriteLock() error = %v", err)
	}

	done := make(chan error, 1)
	go func() {
		writeBoardFile(t, f.boardPath, "tasks.json", `{"tasks":["one"]}`)
		done <- Sync(f.boardPath, false, false)
	}()

	select {
	case err := <-done:
		_ = held.Release()
		t.Fatalf("Sync() returned (err=%v) while board.push.lock was externally held; want it to block", err)
	case <-time.After(200 * time.Millisecond):
	}

	if err := held.Release(); err != nil {
		t.Fatalf("Release() error = %v", err)
	}

	if err := <-done; err != nil {
		t.Fatalf("Sync() error = %v; want nil once the external hold releases", err)
	}
}

// stepSyncRetireLegacyCommitsBoardJSONAndLegacyDeletions asserts that after RetireLegacy on a synced board that tracked the legacy files, Sync commits board.json and the deletions and leaves a clean tree.
func stepSyncRetireLegacyCommitsBoardJSONAndLegacyDeletions(t *testing.T, f *syncFixture) {
	writeBoardFile(t, f.boardPath, legacyTasksFile, `[{"id":0,"slug":"alpha","title":"Alpha","depends_on":[]}]`)
	writeBoardFile(t, f.boardPath, legacyNotesFile, `[]`)
	if err := Sync(f.boardPath, false, false); err != nil {
		t.Fatalf("initial Sync() error = %v", err)
	}

	b := New(Config{Path: f.boardPath, Readme: "Home.md", DesignPrefix: "proposal-", SkipGit: true})
	if err := b.RetireLegacy(); err != nil {
		t.Fatalf("RetireLegacy() error = %v", err)
	}
	if err := Sync(f.boardPath, false, false); err != nil {
		t.Fatalf("Sync() after retire error = %v", err)
	}

	tracked, _, _, err := gitexec.RunGit([]string{"ls-files"}, f.boardPath)
	if err != nil {
		t.Fatalf("git ls-files error = %v", err)
	}
	if !strings.Contains(tracked, boardFile) {
		t.Errorf("tracked files = %q; want %s committed", tracked, boardFile)
	}
	for _, name := range []string{legacyTasksFile, legacyNotesFile} {
		if strings.Contains(tracked, name) {
			t.Errorf("tracked files = %q; want %s deletion committed", tracked, name)
		}
	}
	status, _, _, err := gitexec.RunGit([]string{"status", "--porcelain"}, f.boardPath)
	if err != nil {
		t.Fatalf("git status error = %v", err)
	}
	if strings.TrimSpace(status) != "" {
		t.Errorf("working tree not clean after Sync: %q", status)
	}
}
