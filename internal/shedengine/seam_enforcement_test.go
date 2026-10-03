// seam_enforcement_test.go enforces the Shed Producer-Seam Invariant: production code in
// internal/shedengine imports ONLY the standard library, internal/state, and internal/lock — never
// internal/loomengine, never any adapter package, never internal/lyxcwd, and never internal/logger.
// Like internal/treadleengine's seam_enforcement_test.go, this check is an ALLOWLIST: any import
// outside the allowed set fails the test, so a future stray dependency is caught with no list
// maintenance required.
// One observation worth recording about today's allowlist, though this test checks direct imports
// only and enforces nothing beyond that: the exclusion of internal/lyxcwd happens to hold
// transitively too, because internal/lock imports no internal package at all and internal/state
// imports only internal/fsx and internal/lock.

package shedengine

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// shedengineAllowedImports are the only non-stdlib import paths production code in this package
// may use.
var shedengineAllowedImports = []string{
	"github.com/Knatte18/loomyard/internal/state",
	"github.com/Knatte18/loomyard/internal/lock",
}

// TestProducerSeamInvariant_AllowlistOnly verifies that every non-test .go file in this package
// imports only stdlib or an entry in shedengineAllowedImports.
func TestProducerSeamInvariant_AllowlistOnly(t *testing.T) {
	scankit.AssertImportAllowlist(t, "internal/shedengine", shedengineAllowedImports...)
}
