// lock_test.go verifies withOpLock's per-worktree lock path, that two calls serialize (the second
// blocks until the first releases), that a released lock can be re-acquired with no stale-lock
// residue, and Engine's Socket()/SessionName() accessor strings.
// newTestEngine is the shared fixture every reedengine test in this package builds on; it builds a
// Geometry struct literal directly rather than calling hubgeom.ReedGeometry, since hubgeom imports
// reedengine and an in-package test importing it would close an import cycle.

package reedengine

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
)

// newTestEngine builds an Engine rooted at a fresh t.TempDir(), suitable
// for any reedengine test that needs a *Engine but must never actually
// shell out to tmux: cfg.Tmux/cfg.Shell point at paths that do not exist,
// so a stray real invocation fails fast with "file not found" instead of
// hanging or silently succeeding against some unrelated running server.
// The Geometry fields are distinct values derived from one t.TempDir() — a
// synthetic hub, a worktree root under it, and an anchor path under that —
// so a field mix-up inside the engine surfaces instead of passing silently.
// Only the worktree root is created on disk. AnchorPath and PaneCwd are
// deliberately left uncreated: leaving them absent is what makes a field
// mix-up between them and WorktreeRoot surface, and it makes this shared
// fixture stand in for the standalone shape, where the anchor is a state
// directory the engine itself materializes on first use.
func newTestEngine(t *testing.T) *Engine {
	t.Helper()
	hub := t.TempDir()
	worktreeRoot := filepath.Join(hub, "worktree")
	if err := os.MkdirAll(worktreeRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll worktree root: %v", err)
	}
	anchorPath := filepath.Join(worktreeRoot, "anchor")
	// PaneCwd is deliberately set to a directory distinct from AnchorPath, so
	// a spawn site that regresses to reading AnchorPath instead of PaneCwd
	// cannot pass by coincidence (see lifecycle_test.go's split-window assertion).
	geom := Geometry{
		SocketKey:    ServerName(hub),
		SessionName:  SessionName(worktreeRoot),
		AnchorPath:   anchorPath,
		PaneCwd:      filepath.Join(hub, "pane"),
		WorktreeRoot: worktreeRoot,
		LogsDir:      filepath.Join(hub, "logs"),
		RepoName:     "test-repo",
		HubPath:      hub,
	}
	cfg := Config{
		Tmux:               filepath.Join(hub, "does-not-exist-tmux.exe"),
		Shell:              filepath.Join(hub, "does-not-exist-shell.exe"),
		Width:              100,
		Height:             21,
		CollapsedStripRows: 2,
		MinFullRows:        3,
		StrandName:         "<ROLE>:<ROUND>:<SHORT_GUID>",
		// A valid value so every caller of this shared fixture that reaches
		// ensureServerAndSessionLocked (watchdogOption's boot-path validation)
		// does not start failing on "invalid watchdog value" instead of on
		// what it actually asserts. A test that wants an invalid value
		// overrides e.cfg.Watchdog itself.
		Watchdog: "on",
	}
	return New(cfg, geom)
}

func TestWithOpLock_PathIsUnderDotLyx(t *testing.T) {
	// The anchor path is a real subpath of the worktree root here so
	// stateDir's AnchorPath anchoring (as opposed to WorktreeRoot) is
	// actually observable.
	hub := t.TempDir()
	worktreeRoot := filepath.Join(hub, "worktree")
	if err := os.MkdirAll(worktreeRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll worktree root: %v", err)
	}
	anchorPath := filepath.Join(worktreeRoot, "sub", "dir")
	geom := Geometry{
		SocketKey:    ServerName(hub),
		SessionName:  SessionName(worktreeRoot),
		AnchorPath:   anchorPath,
		PaneCwd:      anchorPath,
		WorktreeRoot: worktreeRoot,
		LogsDir:      filepath.Join(hub, "logs"),
		RepoName:     "test-repo",
		HubPath:      hub,
	}
	cfg := Config{
		Tmux:               filepath.Join(hub, "does-not-exist-tmux.exe"),
		Shell:              filepath.Join(hub, "does-not-exist-shell.exe"),
		Width:              100,
		Height:             21,
		CollapsedStripRows: 2,
		MinFullRows:        3,
		StrandName:         "<ROLE>:<ROUND>:<SHORT_GUID>",
	}
	e := New(cfg, geom)

	var sawPath string
	err := e.withOpLock(func() error {
		sawPath = filepath.Join(e.stateDir(), reedLockFileName)
		if _, statErr := os.Stat(sawPath); statErr != nil {
			t.Errorf("lock file not present while held: %v", statErr)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("withOpLock: %v", err)
	}

	dotLyx := filepath.Join(e.geom.AnchorPath, ".lyx")
	if filepath.Dir(sawPath) != dotLyx {
		t.Errorf("lock path = %q, want under %q (per-worktree, not shared across worktrees)", sawPath, dotLyx)
	}
	if filepath.Dir(sawPath) == filepath.Join(e.geom.WorktreeRoot, ".lyx") {
		t.Errorf("lock path = %q, want it to differ from the WorktreeRoot-based path for a subpath-anchored fixture", sawPath)
	}
}

func TestWithOpLock_SerializesConcurrentCalls(t *testing.T) {
	e := newTestEngine(t)

	started := make(chan struct{})
	release := make(chan struct{})
	firstErr := make(chan error, 1)

	go func() {
		firstErr <- e.withOpLock(func() error {
			close(started)
			<-release
			return nil
		})
	}()

	select {
	case <-started:
	case err := <-firstErr:
		t.Fatalf("first withOpLock returned before fn ran: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("first withOpLock never entered fn (lock acquisition hung)")
	}

	secondStarted := make(chan struct{})
	secondErr := make(chan error, 1)
	go func() {
		secondErr <- e.withOpLock(func() error {
			close(secondStarted)
			return nil
		})
	}()

	select {
	case <-secondStarted:
		t.Fatal("second withOpLock ran while the first still held the lock")
	case err := <-secondErr:
		t.Fatalf("second withOpLock returned before the first released: %v", err)
	case <-time.After(150 * time.Millisecond):
		// Expected: the second call is blocked behind the first.
	}

	close(release)
	if err := <-firstErr; err != nil {
		t.Fatalf("first withOpLock: %v", err)
	}

	select {
	case err := <-secondErr:
		if err != nil {
			t.Fatalf("second withOpLock: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second withOpLock did not proceed after the first released")
	}
}

func TestWithOpLock_ReacquireAfterReleaseSucceeds(t *testing.T) {
	e := newTestEngine(t)

	if err := e.withOpLock(func() error { return nil }); err != nil {
		t.Fatalf("first withOpLock: %v", err)
	}
	// No stale lock should remain from the first, already-released
	// acquisition — a second acquisition on the same path must succeed
	// immediately rather than block or error.
	if err := e.withOpLock(func() error { return nil }); err != nil {
		t.Fatalf("second withOpLock after release: %v", err)
	}
}

func TestEngine_SocketAndSessionName(t *testing.T) {
	hub := t.TempDir()
	worktreeRoot := filepath.Join(hub, "worktree")
	geom := Geometry{
		SocketKey:    ServerName(hub),
		SessionName:  SessionName(worktreeRoot),
		AnchorPath:   worktreeRoot,
		PaneCwd:      worktreeRoot,
		WorktreeRoot: worktreeRoot,
		LogsDir:      filepath.Join(hub, "logs"),
		RepoName:     "test-repo",
		HubPath:      hub,
	}
	e := New(Config{}, geom)

	wantSocket := ServerName(hub)
	if got := e.Socket(); got != wantSocket {
		t.Errorf("Socket() = %q, want %q", got, wantSocket)
	}

	wantSession := filepath.Base(worktreeRoot)
	if got := e.SessionName(); got != wantSession {
		t.Errorf("SessionName() = %q, want %q", got, wantSession)
	}
}

// TestWithOpLock_ReportsALockFileRemovedMidOperation is the regression guard for the R5 review's
// R5-F6.
//
// A flock is held on an inode, not on a name, so deleting .lyx while an op holds reed.lock unlinks
// that inode without disturbing the lock — and the next op's MkdirAll + O_CREATE mints a fresh one
// whose lock is granted immediately. Measured live: a second `lyx reed status` blocked 11027ms with
// the lock genuinely held, and was granted in 107ms once .lyx was deleted mid-hold, so two reed
// processes then drove the same session and the same reed.json concurrently.
// Reed cannot make the second op wait (an unlinked inode is unreachable by name), so what it must
// not do is return success over state it did not actually have exclusive access to.
func TestWithOpLock_ReportsALockFileRemovedMidOperation(t *testing.T) {
	e := newTestEngine(t)

	ran := false
	err := e.withOpLock(func() error {
		ran = true
		// Exactly what `rm -rf .lyx` / `git clean -xdf` does to a held lock.
		return os.RemoveAll(e.stateDir())
	})
	if !ran {
		t.Fatal("withOpLock did not run the operation body")
	}
	if err == nil {
		t.Fatal("withOpLock = nil after its lock file was removed mid-operation; want an error — the operation ran without the exclusion it claimed")
	}
	for _, want := range []string{reedLockFileName, "did not exclude other reed processes"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("withOpLock error = %v; want it to contain %q", err, want)
		}
	}
}

// TestWithOpLock_ReportsBothFailuresWhenTheOperationAlsoFailed pins that the lock-compromise report
// never swallows the operation's own error: an operator debugging a removed .lyx still needs to know
// what the operation itself hit.
func TestWithOpLock_ReportsBothFailuresWhenTheOperationAlsoFailed(t *testing.T) {
	e := newTestEngine(t)

	opFailure := errors.New("the operation's own failure")
	err := e.withOpLock(func() error {
		if rmErr := os.RemoveAll(e.stateDir()); rmErr != nil {
			t.Fatalf("remove state dir: %v", rmErr)
		}
		return opFailure
	})
	if err == nil {
		t.Fatal("withOpLock = nil; want an error")
	}
	if !errors.Is(err, opFailure) {
		t.Errorf("withOpLock error = %v; want it to wrap the operation's own error", err)
	}
	if !strings.Contains(err.Error(), "did not exclude other reed processes") {
		t.Errorf("withOpLock error = %v; want it to also report the compromised lock", err)
	}
}

// TestWithOpLock_QuietWhenTheLockFileSurvives pins the other half: an ordinary operation must not
// pay for this check with a spurious failure.
func TestWithOpLock_QuietWhenTheLockFileSurvives(t *testing.T) {
	e := newTestEngine(t)

	if err := e.withOpLock(func() error { return nil }); err != nil {
		t.Errorf("withOpLock on an undisturbed lock file = %v; want nil", err)
	}
}

// TestWithTryOpLock_RunsFnOnAFreeLock pins the acquired path: fn runs and (true, nil) is reported.
func TestWithTryOpLock_RunsFnOnAFreeLock(t *testing.T) {
	e := newTestEngine(t)

	ran := false
	acquired, err := e.withTryOpLock(func() error {
		ran = true
		return nil
	})
	if !acquired {
		t.Error("withTryOpLock() acquired = false, want true (lock was free)")
	}
	if err != nil {
		t.Errorf("withTryOpLock() err = %v, want nil", err)
	}
	if !ran {
		t.Error("withTryOpLock() did not run fn")
	}
}

// TestWithTryOpLock_DefersWithoutTouchingTmuxWhenAlreadyHeld pins the deferral contract: with
// reed.lock already held by a second acquisition, withTryOpLock reports (false, nil), never calls
// fn, and issues no tmux call — deferral is not a failure.
func TestWithTryOpLock_DefersWithoutTouchingTmuxWhenAlreadyHeld(t *testing.T) {
	e := newTestEngine(t)

	dotLyx := e.stateDir()
	if err := os.MkdirAll(dotLyx, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	lockPath := filepath.Join(dotLyx, reedLockFileName)
	held, err := lock.AcquireWriteLock(lockPath)
	if err != nil {
		t.Fatalf("AcquireWriteLock: %v", err)
	}
	defer held.Release()

	fnCalled := false
	acquired, err := e.withTryOpLock(func() error {
		fnCalled = true
		return nil
	})
	if acquired {
		t.Error("withTryOpLock() acquired = true, want false (lock already held)")
	}
	if err != nil {
		t.Errorf("withTryOpLock() err = %v, want nil (a deferral is not a failure)", err)
	}
	if fnCalled {
		t.Error("withTryOpLock() called fn while the lock was held, want it never called")
	}
}

// TestWithTryOpLock_PropagatesFnErrorWithAcquiredTrue pins that a real acquisition still reports
// acquired == true even when fn itself fails.
func TestWithTryOpLock_PropagatesFnErrorWithAcquiredTrue(t *testing.T) {
	e := newTestEngine(t)

	fnErr := errors.New("fn's own failure")
	acquired, err := e.withTryOpLock(func() error { return fnErr })
	if !acquired {
		t.Error("withTryOpLock() acquired = false, want true")
	}
	if !errors.Is(err, fnErr) {
		t.Errorf("withTryOpLock() err = %v, want it to wrap fn's error", err)
	}
}

// TestWithTryOpLock_ToldGeometryValidationFailureLeavesTheLockFileUntouched pins that a
// told-geometry pre-flight failure reports (false, err) without ever touching the lock file.
func TestWithTryOpLock_ToldGeometryValidationFailureLeavesTheLockFileUntouched(t *testing.T) {
	hub := t.TempDir()
	worktreeRoot := filepath.Join(hub, "worktree")
	geom := Geometry{
		SocketKey:    ServerName(hub),
		SessionName:  "", // empty session name fails validateToldTmuxIdentity
		AnchorPath:   filepath.Join(worktreeRoot, "anchor"),
		PaneCwd:      filepath.Join(hub, "pane"),
		WorktreeRoot: worktreeRoot,
		LogsDir:      filepath.Join(hub, "logs"),
		RepoName:     "test-repo",
		HubPath:      hub,
	}
	e := New(Config{
		Tmux:  filepath.Join(hub, "does-not-exist-tmux.exe"),
		Shell: filepath.Join(hub, "does-not-exist-shell.exe"),
	}, geom)

	acquired, err := e.withTryOpLock(func() error {
		t.Fatal("withTryOpLock() called fn despite a told-geometry validation failure")
		return nil
	})
	if acquired {
		t.Error("withTryOpLock() acquired = true, want false")
	}
	if err == nil {
		t.Fatal("withTryOpLock() err = nil, want a validation error")
	}

	lockPath := filepath.Join(e.stateDir(), reedLockFileName)
	if _, statErr := os.Stat(lockPath); !os.IsNotExist(statErr) {
		t.Errorf("lock file stat err = %v, want IsNotExist (validation must fail before the lock file is ever touched)", statErr)
	}
}
