// leaf_enforcement_test.go enforces the reviewbudget leaf invariant:
// production code in internal/reviewbudget imports only the standard library and internal/configengine.

package reviewbudget

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestLeafInvariant_AllowlistOnly(t *testing.T) {
	tiergate.Require(t, 0)
	scankit.AssertImportAllowlistNoStale(t, "internal/reviewbudget", "github.com/Knatte18/loomyard/internal/configengine")
}
