// leaf_enforcement_test.go enforces the lyxtemp Leaf Invariant.
// Production code in internal/lyxtemp imports only the standard library and internal/lock.

package lyxtemp

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

func TestLeafInvariant_AllowlistOnly(t *testing.T) {
	scankit.AssertImportAllowlist(t, "internal/lyxtemp", "github.com/Knatte18/loomyard/internal/lock")
}
