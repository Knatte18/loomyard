// leaf_enforcement_test.go enforces the GitHub Auth Invariant's leaf half: production code in
// internal/githubclient imports ONLY the standard library, go-github, golang.org/x/sys, and
// internal/proc -- never internal/output, cobra, internal/gitexec, internal/gitrepo, or
// golang.org/x/oauth2.
// Like modelspec's and tokenvocab's leaf_enforcement_test.go, this check is an ALLOWLIST: any
// import outside the allowed set fails the test, so a future stray dependency is caught with no
// list maintenance required.

package githubclient

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// allowedImports are the only non-stdlib import paths production code in this package may use.
var allowedImports = []string{
	"github.com/google/go-github/v75/github",
	"golang.org/x/sys/windows",
	"github.com/Knatte18/loomyard/internal/proc",
}

func TestLeafInvariant_AllowlistOnly(t *testing.T) {
	scankit.AssertImportAllowlist(t, "internal/githubclient", allowedImports...)
}
