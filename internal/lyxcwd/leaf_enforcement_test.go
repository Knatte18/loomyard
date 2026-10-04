// leaf_enforcement_test.go enforces internal/lyxcwd's own import cap from PATTERN-cwd-resolution:
// production code in internal/lyxcwd imports ONLY the standard library and
// internal/gitexec — this is what keeps fabricengine -> logger -> lyxcwd acyclic.

package lyxcwd

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

func TestLeafInvariant_AllowlistOnly(t *testing.T) {
	scankit.AssertImportAllowlist(t, "internal/lyxcwd",
		"github.com/Knatte18/loomyard/internal/gitexec",
	)
}
