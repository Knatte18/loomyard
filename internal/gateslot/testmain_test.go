// testmain_test.go runs the test binary under the hermetic git environment and tmux isolation, and doubles as the holder child the integration test re-executes.
// It carries no build tag, so it compiles into every tag set.

package gateslot

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// childSlotDirEnv, when set, turns the test binary into a child that holds the only slot of that directory until it is killed.
const childSlotDirEnv = "GATESLOT_TEST_CHILD_SLOT_DIR"

// childHeldLine is what the child prints once it holds the slot.
const childHeldLine = "held"

// TestMain runs the tests under tmuxkit.Main, or runs the holder child when childSlotDirEnv is set.
func TestMain(m *testing.M) {
	if dir := os.Getenv(childSlotDirEnv); dir != "" {
		os.Exit(holdSlotUntilKilled(dir))
	}
	gitkit.HermeticGitEnv()
	os.Exit(tmuxkit.Main(m))
}

func holdSlotUntilKilled(dir string) int {
	pool := &Pool{Dir: dir, Limits: singleSlotLimits}
	if _, err := pool.Acquire(context.Background(), Holder{Site: "child"}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println(childHeldLine)
	select {}
}

// singleSlotLimits is the fixed one-slot limit the tests share.
func singleSlotLimits() (Limits, error) {
	return Limits{Slots: 1, GoParallel: 4}, nil
}
