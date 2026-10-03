//go:build !integration

// testmain_test.go runs the untagged test binary under tmux isolation through tmuxkit.Main.
// No untagged test here spawns git, so it needs no hermetic git call.
//
// The `!integration` constraint is load-bearing: testmain_integration_test.go declares its own TestMain under the `integration` tag,
// and an untagged file always compiles, so without this negation both would collide under `go test -tags integration`.

package shedcli

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// TestMain runs the tests under tmuxkit.Main.
func TestMain(m *testing.M) {
	os.Exit(tmuxkit.Main(m))
}
