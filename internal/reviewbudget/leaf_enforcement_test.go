// leaf_enforcement_test.go enforces the reviewbudget leaf invariant: production code in internal/reviewbudget imports only the standard library and internal/configengine.

package reviewbudget

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

func TestLeafInvariant_AllowlistOnly(t *testing.T) {
	scankit.AssertImportAllowlistNoStale(t, "internal/reviewbudget", "github.com/Knatte18/loomyard/internal/configengine")
}
