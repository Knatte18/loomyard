// noforceadd_test.go machine-checks PATTERN-gitrepo-client-boundary for internal/gitrepo's own non-test source:
// it must never reintroduce a `git add -f` branch or the retired `hasPathspecMagic` helper.
// The test has no build tag and reads source text without spawning git, so it runs in tier0, the offline tier.

package gitrepo

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// noForceAddBannedTokens are the raw substrings that gitrepo's own non-test .go source may not contain:
// the `-f` argv entry of a `git add` force-add branch, and any reference to the retired hasPathspecMagic helper.
var noForceAddBannedTokens = []string{
	`"-f"`,
	"hasPathspecMagic",
}

// noForceAddMinScannedFiles is the floor on the non-test .go files the walk of internal/gitrepo must find.
// Finding fewer means the directory resolution is misconfigured, not that the package has shrunk.
const noForceAddMinScannedFiles = 5

// TestNoForceAdd_GitrepoSourceHasNoForceAddBranch scans the non-test .go files directly in internal/gitrepo.
// It fails if any file contains a banned token, which keeps a `git add -f` branch from reappearing.
// The retired `hasPathspecMagic` helper is banned the same way.
func TestNoForceAdd_GitrepoSourceHasNoForceAddBranch(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	var failures []string

	scanned := scankit.Walk(t, scankit.Options{Roots: []string{"internal/gitrepo"}, Shallow: true}, func(f *scankit.File) {
		content := string(f.Data)
		for _, tok := range noForceAddBannedTokens {
			if strings.Contains(content, tok) {
				failures = append(failures, f.Rel+": contains banned force-add token "+tok)
			}
		}
	})

	// A mis-resolved directory that still yields a handful of files must fail here, not pass vacuously.
	scankit.RequireFloor(t, scanned, noForceAddMinScannedFiles, "no-force-add guard")

	if len(failures) > 0 {
		t.Errorf("PATTERN-gitrepo-client-boundary violated:\n%s", strings.Join(failures, "\n"))
	}
}
