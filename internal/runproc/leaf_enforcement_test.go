// leaf_enforcement_test.go enforces the Runproc Leaf Invariant.
// Production code in internal/runproc imports only the standard library and these internal packages:
// internal/proc, internal/state, internal/logger and internal/lymcwd.

package runproc

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// allowedImports admits the leaf's process primitives, tolerant record writer, logger and Bolt directory name.
var allowedImports = []string{
	"github.com/Knatte18/loomyard/internal/logger",
	"github.com/Knatte18/loomyard/internal/lymcwd",
	"github.com/Knatte18/loomyard/internal/proc",
	"github.com/Knatte18/loomyard/internal/state",
}

func TestLeafInvariant_AllowlistOnly(t *testing.T) {
	tiergate.Require(t, 0)
	scankit.AssertImportAllowlist(t, "internal/runproc", allowedImports...)
}
