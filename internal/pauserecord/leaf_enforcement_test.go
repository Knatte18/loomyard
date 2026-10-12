// leaf_enforcement_test.go enforces the pauserecord Leaf Invariant.
// Production code in internal/pauserecord imports only the standard library, internal/lock and internal/state.

package pauserecord

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

func TestLeafInvariant_AllowlistOnly(t *testing.T) {
	scankit.AssertImportAllowlist(t, "internal/pauserecord", "github.com/Knatte18/loomyard/internal/lock", "github.com/Knatte18/loomyard/internal/state")
}
