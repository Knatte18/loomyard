// testmain_test.go runs the test binary under tmux isolation, which the integration-tagged prebuild test requires of its package.
// It carries no build tag, so it compiles into every tag set.

package main

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// TestMain runs the tests under tmuxkit.Main.
func TestMain(m *testing.M) {
	os.Exit(tmuxkit.Main(m))
}
