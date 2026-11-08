// testmain_test.go runs the test binary under tmux isolation through tmuxkit.Main.
// It carries no build tag, so it compiles into every tag set.

package reedengine

import (
	"flag"
	"fmt"
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// TestMain runs the tests under tmuxkit.Main.
// It first exits with status 2 on a positional argument, which no test run passes.
// A tmux job or binding that names this binary, such as `slack bar`, would otherwise run the whole suite again,
// and each session that run boots could pin the same job.
func TestMain(m *testing.M) {
	flag.Parse()
	if flag.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "reedengine.test: refusing the non-test arguments %q\n", flag.Args())
		os.Exit(2)
	}
	os.Exit(tmuxkit.Main(m))
}
