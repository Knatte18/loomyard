//go:build tier0

// leaf_enforcement_test.go enforces that production code in internal/lensreport imports the standard library and nothing else.

package lensreport_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

func TestLeafInvariant_AllowlistOnly(t *testing.T) {
	scankit.AssertImportAllowlist(t, "internal/lensreport")
}
