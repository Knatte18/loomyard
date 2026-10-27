//go:build tier0

// leaf_enforcement_test.go enforces the lymtemp Leaf Invariant.
// Production code in internal/lymtemp imports only the standard library and internal/lock.

package lymtemp

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

func TestLeafInvariant_AllowlistOnly(t *testing.T) {
	scankit.AssertImportAllowlist(t, "internal/lymtemp", "github.com/Knatte18/loomyard/internal/lock")
}
