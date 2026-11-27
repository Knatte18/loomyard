// leaf_enforcement_test.go enforces that internal/fabriccore stays a leaf:
// a closed import set, and no standard-library import that reaches the disk, a process or the network.

package fabriccore

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

//lym:guard
//lym:watch internal/fabriccore/*.go internal/testkit/scankit/*.go
func TestLeafInvariant_AllowlistOnly(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	scankit.AssertImportAllowlistNoStale(t, "internal/fabriccore",
		"github.com/Knatte18/loomyard/internal/fabriclayout")
}

//lym:guard
//lym:watch internal/fabriccore/*.go internal/testkit/scankit/*.go
func TestLeafInvariant_NoIO(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	scankit.AssertNoImports(t, "internal/fabriccore", "os", "os/...", "io/ioutil", "io/fs", "syscall", "net", "net/...")
}
