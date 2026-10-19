// lockwait_test.go pins that the weft write lock's wait signal reaches only the path a test registered for.

package fabricengine

import (
	"path/filepath"
	"testing"
)

func TestAcquireWeftWriteLock_SignalsOnlyItsOwnPath(t *testing.T) {
	t.Parallel()
	first, second, third := t.TempDir(), t.TempDir(), t.TempDir()
	firstSignal := AwaitWeftLockWaitForTest(t, filepath.Join(first, weftWriteLockFile))
	secondSignal := AwaitWeftLockWaitForTest(t, filepath.Join(second, weftWriteLockFile))

	held, err := acquireWeftWriteLock(first)
	if err != nil {
		t.Fatalf("acquireWeftWriteLock(first) error = %v", err)
	}
	if err := held.Release(); err != nil {
		t.Fatalf("Release() error = %v", err)
	}
	select {
	case <-firstSignal:
	default:
		t.Error("first path's channel is open after acquiring its lock; want it closed")
	}
	select {
	case <-secondSignal:
		t.Error("second path's channel closed by the first path's acquisition; want it open")
	default:
	}

	for _, dir := range []string{first, third} {
		again, err := acquireWeftWriteLock(dir)
		if err != nil {
			t.Fatalf("acquireWeftWriteLock(%q) error = %v", dir, err)
		}
		if err := again.Release(); err != nil {
			t.Fatalf("Release() error = %v", err)
		}
	}
	select {
	case <-secondSignal:
		t.Error("second path's channel closed by a repeat or unregistered acquisition; want it open")
	default:
	}
}
