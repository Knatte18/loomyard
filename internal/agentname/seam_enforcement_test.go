// seam_enforcement_test.go enforces the Agent Name Invariant's leaf clause:
// production code in internal/agentname imports the standard library only.

package agentname

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

func TestAgentNameInvariant_StdlibOnly(t *testing.T) {
	scankit.AssertImportAllowlist(t, "internal/agentname")
}
