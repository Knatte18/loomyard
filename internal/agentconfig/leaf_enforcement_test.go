// leaf_enforcement_test.go holds the package to its leaf import set: the standard library, configengine and yaml.v3.

package agentconfig

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestLeafInvariant_AllowlistOnly(t *testing.T) {
	tiergate.Require(t, 0)
	scankit.AssertImportAllowlist(t, "internal/agentconfig", "github.com/Knatte18/loomyard/internal/configengine", "gopkg.in/yaml.v3")
}
