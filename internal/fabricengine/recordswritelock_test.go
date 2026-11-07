// recordswritelock_test.go is the Tier-0 test for HoldRecordsWriteLock, which holds the combined records write lock.
// It spawns no git process.

package fabricengine

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestHoldRecordsWriteLock covers a free lock being held.
// A second hold reports not held while the first holds the lock.
// A hold succeeds once the first is released.
func TestHoldRecordsWriteLock(t *testing.T) {
	tiergate.Require(t, 0)
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
