// leaf_enforcement_test.go enforces internal/lyxcwd's own import cap from PATTERN-cwd-resolution.
// Production code in internal/lyxcwd imports ONLY the standard library and internal/dotgit.
// That keeps fabricengine -> logger -> lyxcwd acyclic, and leaves lyxcwd unable to spawn git.

package lyxcwd

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

func TestLeafInvariant_AllowlistOnly(t *testing.T) {
	scankit.AssertImportAllowlist(t, "internal/lyxcwd",
		"github.com/Knatte18/loomyard/internal/dotgit",
	)
}
