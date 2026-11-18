// testmain_test.go runs the package's tests under tmuxkit.Main;
// their temp files live in the process run directory and go with it.

package shuttleengines

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine/codexengine"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// TestMain points both engines' lym binary at a stand-in path, since the test binary is no lym.
func TestMain(m *testing.M) {
	standIn := func() (string, error) { return "/stub/lym", nil }
	claudeengine.LymBinary = standIn
	codexengine.LymBinary = standIn
	os.Exit(tmuxkit.Main(m))
}
