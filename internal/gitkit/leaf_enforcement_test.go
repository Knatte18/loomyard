// leaf_enforcement_test.go enforces the gitkit Leaf Invariant.
// Production code in internal/gitkit imports only the standard library and internal/configengine, internal/lyxcwd, internal/weftname, internal/lyxdirs and internal/lyxtemp.
// It never imports internal/configreg or any feature package (boardengine/boardcli, ideengine/idecli, selfreportengine/selfreportcli, fabricengine/fabriccli).
// Tests that need real config seed it via SeedConfig with a configreg-free map[string]string (never configreg types).

package gitkit

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// allowedImports are the only non-stdlib import paths production code in this package may use.
var allowedImports = []string{
	"github.com/Knatte18/loomyard/internal/configengine",
	"github.com/Knatte18/loomyard/internal/lyxcwd",
	"github.com/Knatte18/loomyard/internal/lyxdirs",
	"github.com/Knatte18/loomyard/internal/lyxtemp",
	"github.com/Knatte18/loomyard/internal/weftname",
}

func TestLeafInvariant_AllowlistOnly(t *testing.T) {
	scankit.AssertImportAllowlist(t, "internal/gitkit", allowedImports...)
}
