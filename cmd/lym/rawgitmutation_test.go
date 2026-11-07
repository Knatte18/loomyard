// rawgitmutation_test.go machine-checks the Fabric Git Invariant of `PATTERN-fabric-boundary`.
// It covers internal/websterengine.
// Its production source never constructs a raw gitrepo handle or calls gitexec.Run directly.
// The one grandfathered read-only exemption is on this file's allowlist.
//
// The guard bans the two tokens `gitrepo.New(` and `gitexec.Run(`, not per-verb method names.
// A verb-name ban would flag legitimate calls to mutating verbs, such as `.ResetHard(` on an engine-declared interface.
// It would also miss a raw `gitexec.Run(` bypass, which carries no method token.

package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// rawGitMutationScanPackages lists the module-relative package subtrees this guard walks.
var rawGitMutationScanPackages = []string{
	"internal/websterengine",
}

// rawGitMutationBannedTokens lists the raw substrings that a non-test .go file in the scanned packages must not contain.
// A file is exempt only when it is on the allowlist.
var rawGitMutationBannedTokens = []string{
	"gitrepo.New(",
	"gitexec.Run(",
}

// rawGitMutationAllowlist maps the module-relative, slash-separated path of each exempt file to its reason.
// Its one entry is the read-only exemption to the Fabric Git Invariant in internal/websterengine.
var rawGitMutationAllowlist = []scankit.Entry{
	{Key: "internal/websterengine/gitwrap.go", Why: "grandfathered read-only exemptions — via gitrepo.New the read-only queries CurrentSHA, MergeHeadPresent, CommitParents (the reconcile walk and the fix-commit walk, `fixCommitRejection`), ResolveSHA, IsAncestor, MergeTree and CommitTree, SHAExists (shaExists) and IsAncestor (isAncestor), and via the checked gitexec.Run the read-only probes the Shared Decision git-verification-via-gitrepo's carved-out exception covers: `status --porcelain` (dirty), `status --porcelain --ignored` (ignoredPath), `worktree list --porcelain` (otherWorktrees), `diff --quiet` and `ls-files --others` (worktreePathDiffers), `hash-object` (worktreeBlob), `rev-parse --verify --quiet` (commitBlob) and `ls-tree -r` (treePathsWithBlob)"},
}

// rawGitMutationMinScannedFiles is the floor below which a scan counts as vacuous.
// A count under it means the walk is misconfigured, not that the package shrank.
const rawGitMutationMinScannedFiles = 4

// TestNoRawGitMutation_WebsterProductionSource walks the non-test .go files of internal/websterengine.
// It fails if one contains the raw substring "gitrepo.New(" or "gitexec.Run(" and is not allowlisted.
// Those two tokens are the construction and call forms a raw, fabric-bypassing git mutation carries.
//
//lym:guard
func TestNoRawGitMutation_WebsterProductionSource(t *testing.T) {
	tiergate.Require(t, 0)
	allow := scankit.NewAllowlist(rawGitMutationAllowlist)

	var scanned int
	var failures []string

	for _, pkgRel := range rawGitMutationScanPackages {
		scanned += scankit.Walk(t, scankit.Options{Roots: []string{pkgRel}}, func(f *scankit.File) {
			if allow.Allowed(f.Rel) {
				return
			}
			content := string(f.Data)
			for _, tok := range rawGitMutationBannedTokens {
				if strings.Contains(content, tok) {
					failures = append(failures, fmt.Sprintf(
						"%s: contains banned raw-git-mutation token %q — mutating git in this package must dispatch through internal/fabricengine's code-side methods (see `PATTERN-fabric-boundary`), or add a rawGitMutationAllowlist entry in cmd/lym/rawgitmutation_test.go with a reason if this is a new grandfathered read-only exemption",
						f.Rel, tok,
					))
				}
			}
		})
	}

	scankit.RequireFloor(t, scanned, rawGitMutationMinScannedFiles, "raw git mutation guard")
	allow.RequireNoStale(t)

	if len(failures) > 0 {
		t.Errorf("`PATTERN-fabric-boundary` violated:\n%s", strings.Join(failures, "\n"))
	}
}
