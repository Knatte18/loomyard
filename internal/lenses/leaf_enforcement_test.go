// leaf_enforcement_test.go pins PATTERN-leaf-packages for internal/lenses:
// production code imports only the standard library, internal/configengine and gopkg.in/yaml.v3.

package lenses

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// allowedImports are the only non-stdlib import paths production code in this package may use.
var allowedImports = []string{
	"github.com/Knatte18/loomyard/internal/configengine",
	"gopkg.in/yaml.v3",
}

func TestLeafInvariant_AllowlistOnly(t *testing.T) {
	tiergate.Require(t, 0)
	scankit.AssertImportAllowlistNoStale(t, "internal/lenses", allowedImports...)
}
