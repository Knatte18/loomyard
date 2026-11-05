// leaf_enforcement_test.go enforces the Raddleparser Sole-Parser Invariant's import half: production code in internal/raddleparser imports the standard library and internal/discussionparser only.

package raddleparser

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestLeafInvariant_AllowlistOnly(t *testing.T) {
	tiergate.Require(t, 0)
	scankit.AssertImportAllowlist(t, "internal/raddleparser", "github.com/Knatte18/loomyard/internal/discussionparser")
}
