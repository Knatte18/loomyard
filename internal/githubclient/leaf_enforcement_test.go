// leaf_enforcement_test.go enforces the GitHub Auth Invariant's leaf half.
// Production code in internal/githubclient imports only the standard library and these dependencies:
// go-github, golang.org/x/sys, internal/proc, and internal/pauserecord.
// It never imports internal/output, cobra, internal/gitexec, internal/gitrepo, or golang.org/x/oauth2.
// The check is an allowlist: any import outside the allowed set fails the test,
// so a future stray dependency is caught with no list maintenance required.

package githubclient

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// allowedImports are the only non-stdlib import paths production code in this package may use.
var allowedImports = []string{
	"github.com/google/go-github/v75/github",
	"golang.org/x/sys/windows",
	"github.com/Knatte18/loomyard/internal/proc",
	"github.com/Knatte18/loomyard/internal/pauserecord",
}

func TestLeafInvariant_AllowlistOnly(t *testing.T) {
	tiergate.Require(t, 0)
	scankit.AssertImportAllowlist(t, "internal/githubclient", allowedImports...)
}
