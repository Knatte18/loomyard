//go:build tier0

// lock_test.go verifies withOpLock's per-worktree lock path, that two calls serialize (the second
// blocks until the first releases), that a released lock can be re-acquired with no stale-lock
// residue, and Engine's Socket()/SessionName() accessor strings.

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

//testtiming:keep pins the lock file living under the anchor path's .lym, not the worktree root's, for a subpath-anchored fixture, and a second acquisition succeeding after release; its covering tests run this code without asserting it
func TestWithOpLock_PathIsUnderDotLym(t *testing.T) {
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
		HubPath:      hub,
	}
	cfg := Config{
		Tmux:   filepath.Join(hub, "does-not-exist-tmux.exe"),
		Shell:  filepath.Join(hub, "does-not-exist-shell.exe"),
		Width:  100,
		Height: 21,
	}
	e := New(cfg, geom, testScrub)

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

	dotLym := filepath.Join(e.geom.AnchorPath, ".lym")
	if filepath.Dir(sawPath) != dotLym {
		t.Errorf("lock path = %q, want under %q (per-worktree, not shared across worktrees)", sawPath, dotLym)
	}
	if filepath.Dir(sawPath) == filepath.Join(e.geom.WorktreeRoot, ".lym") {
		t.Errorf("lock path = %q, want it to differ from the WorktreeRoot-based path for a subpath-anchored fixture", sawPath)
	}
	// No stale lock remains from the released acquisition: a second one on the same path succeeds immediately rather than blocking or erroring.
	if err := e.withOpLock(func() error { return nil }); err != nil {
		t.Errorf("second withOpLock after release: %v", err)
	}
}

//testtiming:keep pins a second withOpLock blocking until the first releases and then proceeding; its covering tests run this code without asserting it
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

//testtiming:keep pins Socket() returning the told socket key and SessionName() the worktree basename; its covering tests run this code without asserting it
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
		HubPath:      hub,
	}
	e := New(Config{}, geom, testScrub)

	wantSocket := ServerName(hub)
	if got := e.Socket(); got != wantSocket {
		t.Errorf("Socket() = %q, want %q", got, wantSocket)
	}

	wantSession := filepath.Base(worktreeRoot)
	if got := e.SessionName(); got != wantSession {
		t.Errorf("SessionName() = %q, want %q", got, wantSession)
	}

	defer func() {
		if recover() == nil {
			t.Error("New over an empty EnvScrub did not panic")
		}
	}()
	New(Config{}, geom, EnvScrub{})
}

// TestWithOpLock_ReportsALockFileRemovedMidOperation is the regression guard for the R5 review's
// R5-F6.
//
// A flock is held on an inode, not on a name, so deleting .lym while an op holds reed.lock unlinks
// that inode without disturbing the lock — and the next op's MkdirAll + O_CREATE mints a fresh one
// whose lock is granted immediately. Measured live: a second `lym reed status` blocked 11027ms with
// the lock genuinely held, and was granted in 107ms once .lym was deleted mid-hold, so two reed
// processes then drove the same session and the same reed.json concurrently.
// Reed cannot make the second op wait (an unlinked inode is unreachable by name), so what it must
// not do is return success over state it did not actually have exclusive access to.
func TestWithOpLock_ReportsALockFileRemovedMidOperation(t *testing.T) {
	e := newTestEngine(t)

	ran := false
	err := e.withOpLock(func() error {
		ran = true
		// Exactly what `rm -rf .lym` / `git clean -xdf` does to a held lock.
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
// never swallows the operation's own error: an operator debugging a removed .lym still needs to know
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

// TestWithTryOpLock pins the three outcomes: on a free lock fn runs and (true, nil) is reported,
// an fn error is wrapped while acquired stays true, and with reed.lock already held by a second
// acquisition it reports (false, nil) and never calls fn (a deferral is not a failure).
//
//testtiming:keep pins withTryOpLock running fn on a free lock with acquired true, wrapping fn's error with acquired still true, and deferring with (false, nil) and no fn call or tmux call while the lock is held; its covering tests run this code without asserting it
func TestWithTryOpLock(t *testing.T) {
	fnErr := errors.New("fn's own failure")
	tests := []struct {
		name         string
		hold         bool
		fnErr        error
		wantAcquired bool
		wantFnRan    bool
	}{
		{name: "RunsFnOnAFreeLock", wantAcquired: true, wantFnRan: true},
		{name: "PropagatesFnErrorWithAcquiredTrue", fnErr: fnErr, wantAcquired: true, wantFnRan: true},
		{name: "DefersWhenAlreadyHeld", hold: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestEngine(t)
			if tt.hold {
				if err := os.MkdirAll(e.stateDir(), 0o755); err != nil {
					t.Fatalf("MkdirAll: %v", err)
				}
				held, err := lock.AcquireWriteLock(filepath.Join(e.stateDir(), reedLockFileName))
				if err != nil {
					t.Fatalf("AcquireWriteLock: %v", err)
				}
				defer held.Release()
			}
			fake := installFakeTmux(t, e)

			ran := false
			acquired, err := e.withTryOpLock(func() error {
				ran = true
				return tt.fnErr
			})

			if acquired != tt.wantAcquired {
				t.Errorf("withTryOpLock() acquired = %v, want %v", acquired, tt.wantAcquired)
			}
			if ran != tt.wantFnRan {
				t.Errorf("withTryOpLock() ran fn = %v, want %v", ran, tt.wantFnRan)
			}
			if tt.fnErr == nil && err != nil {
				t.Errorf("withTryOpLock() err = %v, want nil", err)
			}
			if tt.fnErr != nil && !errors.Is(err, tt.fnErr) {
				t.Errorf("withTryOpLock() err = %v, want it to wrap fn's error", err)
			}
			if calls := fake.Calls(); len(calls) != 0 {
				t.Errorf("withTryOpLock() issued tmux calls %v, want none", calls)
			}
		})
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
		HubPath:      hub,
	}
	e := New(Config{
		Tmux:  filepath.Join(hub, "does-not-exist-tmux.exe"),
		Shell: filepath.Join(hub, "does-not-exist-shell.exe"),
	}, geom, testScrub)

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
