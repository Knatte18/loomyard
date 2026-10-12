// testmain_test.go runs the package's test binary under tmuxkit.Main.
// It is an external test package so tmuxkit may itself import lyxtemp without a cycle.

package lyxtemp_test

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

func TestMain(m *testing.M) {
	os.Exit(tmuxkit.Main(m))
}
