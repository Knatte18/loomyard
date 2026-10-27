//go:build tier0

// leaf_enforcement_test.go enforces internal/lymcwd's own import cap from PATTERN-cwd-resolution.
// Production code in internal/lymcwd imports ONLY the standard library, internal/dotgit and internal/weftname.
// That keeps fabricengine -> logger -> lymcwd acyclic, and leaves lymcwd unable to spawn git.

package lymcwd

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

func TestLeafInvariant_AllowlistOnly(t *testing.T) {
	scankit.AssertImportAllowlist(t, "internal/lymcwd",
		"github.com/Knatte18/loomyard/internal/dotgit",
		"github.com/Knatte18/loomyard/internal/weftname",
	)
}
