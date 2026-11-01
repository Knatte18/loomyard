// leaf_enforcement_test.go enforces the pauserecord Leaf Invariant.
// Production code in internal/pauserecord imports only the standard library, internal/state.

package pauserecord

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestLeafInvariant_AllowlistOnly(t *testing.T) {
	tiergate.Require(t, 0)
	scankit.AssertImportAllowlistNoStale(t, "internal/pauserecord", "github.com/Knatte18/loomyard/internal/state")
}
