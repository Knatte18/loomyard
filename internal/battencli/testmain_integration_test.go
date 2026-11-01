// testmain_integration_test.go wires this package's test binary into the hermetic git
// test environment: gitkit.HermeticGitEnv() runs once before any test, since
// lifecycle_integration_test.go spawns git via hubforge fixtures (Test Tier Purity Invariant /
// Hermetic Git Test Environment Invariant), in the shape internal/landingshed's own equivalent
// already uses.
// The binary also runs under tmux isolation through tmuxkit.Main.

package battencli

import (
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// pairPushCounter counts the pair pushes spawned per worktree path, safe for the parallel tests that share the one TestMain seam.
type pairPushCounter struct {
	mutex  sync.Mutex
	counts map[string]int
}

func (counter *pairPushCounter) record(worktreePath string) {
	counter.mutex.Lock()
	defer counter.mutex.Unlock()
	if counter.counts == nil {
		counter.counts = map[string]int{}
	}
	counter.counts[worktreePath]++
}

func (counter *pairPushCounter) count(worktreePath string) int {
	counter.mutex.Lock()
	defer counter.mutex.Unlock()
	return counter.counts[worktreePath]
}

var pairPushCounts pairPushCounter

// TestMain runs HermeticGitEnv before any test spawns git.
// It then runs the tests under tmuxkit.Main.
// A pair push runs in the foreground: a detached one would fork this test binary, and the tests read the pushed branches on the remote.
// Each push is also counted per worktree path, so a parallel test asserts how often its own pair was pushed without swapping the seam.
// A run whose selection is tier0 alone installs a refusing seam instead, so a tier0 test that reaches a pair push in such a run fails rather than pushing for real.
func TestMain(m *testing.M) {
	gitkit.HermeticGitEnv()
	selection, err := tiergate.Selected()
	if err == nil && selection.Low == 0 && selection.High == 0 {
		spawnPairPush = func(*lymcwd.Location) error {
			return errors.New("unit tests push no pair")
		}
	} else {
		spawnPairPush = func(l *lymcwd.Location) error {
			pairPushCounts.record(l.WorktreePath())
			_, err := fabricengine.PushPairAnchored(l, fabricengine.EnvSyncOptions(), fabricengine.LockWaitUnbounded)
			return err
		}
	}
	os.Exit(tmuxkit.Main(m))
}
