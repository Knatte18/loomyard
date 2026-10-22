// leaf_enforcement_test.go enforces the Runproc Leaf Invariant.
// Production code in internal/runproc imports only the standard library, internal/proc, internal/state and internal/logger.

package runproc

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// allowedImports admits the process primitives, the tolerant record writer and the logger the leaf uses.
var allowedImports = []string{
	"github.com/Knatte18/loomyard/internal/logger",
	"github.com/Knatte18/loomyard/internal/proc",
	"github.com/Knatte18/loomyard/internal/state",
}

func TestLeafInvariant_AllowlistOnly(t *testing.T) {
	scankit.AssertImportAllowlist(t, "internal/runproc", allowedImports...)
}
