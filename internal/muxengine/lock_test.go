// lock_test.go verifies withOpLock's per-worktree lock path, that two calls
// serialize (the second blocks until the first releases), that a released
// lock can be re-acquired with no stale-lock residue, and Engine's
// Socket()/SessionName() accessor strings. newTestEngine is the shared
// fixture every muxengine test in this package builds on.

package muxengine

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
)

// newTestEngine builds an Engine rooted at a fresh t.TempDir(), suitable
// for any muxengine test that needs a *Engine but must never actually
// shell out to psmux: cfg.Psmux/cfg.Pwsh point at paths that do not exist,
// so a stray real invocation fails fast with "file not found" instead of
// hanging or silently succeeding against some unrelated running server.
func newTestEngine(t *testing.T) *Engine {
	t.Helper()
	root := t.TempDir()
	layout := &hubgeometry.Layout{
		Cwd:          root,
		WorktreeRoot: root,
		Hub:          filepath.Dir(root),
	}
	cfg := Config{
		Psmux:              filepath.Join(root, "does-not-exist-psmux.exe"),
		Pwsh:               filepath.Join(root, "does-not-exist-pwsh.exe"),
		Width:              100,
		Height:             21,
		CollapsedStripRows: 2,
		MinFullRows:        3,
		StrandName:         "<ROLE>:<ROUND>:<SHORT_GUID>",
	}
	return New(cfg, layout)
}

func TestWithOpLock_PathIsUnderDotLyx(t *testing.T) {
	e := newTestEngine(t)

	var sawPath string
	err := e.withOpLock(func() error {
		sawPath = filepath.Join(e.layout.DotLyxDir(), muxLockFileName)
		if _, statErr := os.Stat(sawPath); statErr != nil {
			t.Errorf("lock file not present while held: %v", statErr)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("withOpLock: %v", err)
	}

	dotLyx := filepath.Join(e.layout.Cwd, ".lyx")
	if filepath.Dir(sawPath) != dotLyx {
		t.Errorf("lock path = %q, want under %q (per-worktree, not shared across worktrees)", sawPath, dotLyx)
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
	e := newTestEngine(t)

	wantSocket := ServerName(e.layout.Hub)
	if got := e.Socket(); got != wantSocket {
		t.Errorf("Socket() = %q, want %q", got, wantSocket)
	}

	wantSession := filepath.Base(e.layout.WorktreeRoot)
	if got := e.SessionName(); got != wantSession {
		t.Errorf("SessionName() = %q, want %q", got, wantSession)
	}
}
