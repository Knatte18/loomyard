// noforceadd_test.go machine-checks CONSTRAINTS.md's Never Force-Add Invariant: internal/gitrepo's
// own non-test source may never reintroduce a `git add -f` branch or the deleted hasPathspecMagic
// helper that used to decide when to take it.
// It is deliberately untagged (no //go:build constraint) and does a pure substring scan — no git
// spawn — so it stays a Tier 1 test, modeled on cmd/lyx/rawgitmutation_test.go's token-ban
// structure.

package gitrepo

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// noForceAddBannedTokens are the raw substrings gitrepo's own non-test .go
// source may not contain: the `-f` argv entry a `git add` force-add branch
// would carry, and any reference to the deleted hasPathspecMagic helper that
// used to gate it.
var noForceAddBannedTokens = []string{
	`"-f"`,
	"hasPathspecMagic",
}

// noForceAddMinScannedFiles is the vacuous-scan floor for this guard's
// single-directory walk of internal/gitrepo: fewer than 5 non-test .go files
// found means the directory resolution is misconfigured rather than the
// package having genuinely shrunk (it has 8 today: ancestry.go, doc.go,
// gitrepo.go, gogit.go, pull.go, push.go, reset.go, worktree.go).
const noForceAddMinScannedFiles = 5

// TestNoForceAdd_GitrepoSourceHasNoForceAddBranch walks internal/gitrepo's own non-test .go files
// and fails if any of them contains a banned token from noForceAddBannedTokens — guarding against
// `git add -f` (or the hasPathspecMagic helper that used to gate it) ever reappearing.
// See CONSTRAINTS.md's Never Force-Add Invariant.
func TestNoForceAdd_GitrepoSourceHasNoForceAddBranch(t *testing.T) {
	var failures []string

	scanned := scankit.Walk(t, scankit.Options{Roots: []string{"internal/gitrepo"}, Shallow: true}, func(f *scankit.File) {
		content := string(f.Data)
		for _, tok := range noForceAddBannedTokens {
			if strings.Contains(content, tok) {
				failures = append(failures, f.Rel+": contains banned force-add token "+tok)
			}
		}
	})

	// Vacuous-scan protection: a mis-resolved directory that still finds a
	// handful of files must not pass.
	scankit.RequireFloor(t, scanned, noForceAddMinScannedFiles, "no-force-add guard")

	if len(failures) > 0 {
		t.Errorf("Never Force-Add Invariant violated (see CONSTRAINTS.md):\n%s", strings.Join(failures, "\n"))
	}
}
