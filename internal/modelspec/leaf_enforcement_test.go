// leaf_enforcement_test.go enforces the Modelspec Leaf Invariant: production code in
// internal/modelspec imports ONLY the standard library, internal/configengine, and gopkg.in/yaml.v3
// — never configreg, envsource, yamlengine, lyxcwd, or any feature package.
// This check is an ALLOWLIST: any import outside the allowed set fails the test, so a future stray
// dependency is caught with no list maintenance required.

package modelspec

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// allowedImports are the only non-stdlib import paths production code in this package may use.
var allowedImports = []string{
	"github.com/Knatte18/loomyard/internal/configengine",
	"gopkg.in/yaml.v3",
}

func TestLeafInvariant_AllowlistOnly(t *testing.T) {
	scankit.AssertImportAllowlist(t, "internal/modelspec", allowedImports...)
}
