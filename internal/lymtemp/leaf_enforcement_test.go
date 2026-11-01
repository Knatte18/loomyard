// leaf_enforcement_test.go enforces the lymtemp Leaf Invariant.
// Production code in internal/lymtemp imports only the standard library and internal/lock.

package lymtemp

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestLeafInvariant_AllowlistOnly(t *testing.T) {
	tiergate.Require(t, 0)
	scankit.AssertImportAllowlist(t, "internal/lymtemp", "github.com/Knatte18/loomyard/internal/lock")
}
