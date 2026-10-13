// leaf_enforcement_test.go enforces the Selfexec Leaf Invariant.
// Production code in internal/selfexec imports only the standard library and internal/logger.

package selfexec

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// allowedImports admits internal/logger, which the helper uses to log its stat, exec and replace outcomes.
var allowedImports = []string{"github.com/Knatte18/loomyard/internal/logger"}

func TestLeafInvariant_AllowlistOnly(t *testing.T) {
	scankit.AssertImportAllowlist(t, "internal/selfexec", allowedImports...)
}
