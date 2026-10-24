//go:build tier0

// recordswritelock_test.go — Tier-0 (untagged, no git spawn) test for HoldRecordsWriteLock, the exported hold of the combined records write lock.

package fabricengine

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestHoldRecordsWriteLock covers a free lock being held, a second hold reading not held while the first holds it, and a hold succeeding once the first is released.
func TestHoldRecordsWriteLock(t *testing.T) {
	t.Parallel()
	recordsWorktree := t.TempDir()
	// An empty .git directory is the geometry the lock directory's exclude seeding needs.
	if err := os.Mkdir(filepath.Join(recordsWorktree, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A wait of zero reports busy before its first try, so the contended hold waits a few milliseconds.
	const contendedWait = 20 * time.Millisecond

	release, held, err := HoldRecordsWriteLock(recordsWorktree, contendedWait)
	if err != nil || !held || release == nil {
		t.Fatalf("HoldRecordsWriteLock(free) = held %v, release nil %v, err %v; want held with a release", held, release == nil, err)
	}

	secondRelease, secondHeld, err := HoldRecordsWriteLock(recordsWorktree, contendedWait)
	if err != nil || secondHeld || secondRelease != nil {
		t.Fatalf("HoldRecordsWriteLock(contended) = held %v, release nil %v, err %v; want not held, nil release", secondHeld, secondRelease == nil, err)
	}

	if err := release(); err != nil {
		t.Fatalf("release() = %v; want nil", err)
	}

	thirdRelease, thirdHeld, err := HoldRecordsWriteLock(recordsWorktree, contendedWait)
	if err != nil || !thirdHeld || thirdRelease == nil {
		t.Fatalf("HoldRecordsWriteLock(released) = held %v, release nil %v, err %v; want held with a release", thirdHeld, thirdRelease == nil, err)
	}
	if err := thirdRelease(); err != nil {
		t.Fatalf("release() = %v; want nil", err)
	}
}
