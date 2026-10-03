// rawgitmutation_test.go closes the Fabric Git Invariant's tracked gap by machine-checking that
// internal/websterengine's production source never constructs a raw
// gitrepo handle or calls gitexec.Run directly, beyond the one grandfathered read-only exemption
// this file names.
// See CONSTRAINTS.md's Fabric Git Invariant (warp + weft) — the "Known gap, tracked" clause
// internal/websterengine's migration onto internal/fabricengine's warp-only methods closes.
//
// The guard bans the two CONSTRUCTION/CALL tokens (gitrepo.New(, gitexec.Run() — not per-verb
// method names: a verb-name ban would both flag the correctly-migrated consumer code (which
// legitimately calls .CheckoutDetached(/.ResetHard( on the new FabricBisector/FabricResetter
// interfaces) and miss the raw gitexec.Run( bypass that carries no method token at all.

package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// rawGitMutationScanPackages are the module-relative package subtrees this
// guard walks: exactly the one package the discussion's regression-guard
// decision names, internal/websterengine.
var rawGitMutationScanPackages = []string{
	"internal/websterengine",
}

// rawGitMutationBannedTokens are the raw substrings a non-test .go file in
// rawGitMutationScanPackages may not contain, unless the file is on
// rawGitMutationAllowlist.
var rawGitMutationBannedTokens = []string{
	"gitrepo.New(",
	"gitexec.Run(",
}

// rawGitMutationAllowlist is this guard's per-file allowlist (path module-
// relative, slash-separated → reason): the one grandfathered read-only
// exemption the Fabric Git Invariant's "Known gap, tracked" clause carved
// out when the mutating paths in this package migrated onto
// internal/fabricengine's warp-only methods.
var rawGitMutationAllowlist = []scankit.Entry{
	{Key: "internal/websterengine/gitwrap.go", Why: "grandfathered read-only exemptions — via gitrepo.New the read-only queries CurrentSHA, MergeHeadPresent, the reconcile walk's CommitParents, ResolveSHA, IsAncestor, MergeTree and CommitTree, SHAExists (shaExists) and IsAncestor (isAncestor), and via the checked gitexec.Run the read-only probes the Shared Decision git-verification-via-gitrepo's carved-out exception covers: `status --porcelain` (dirty), `status --porcelain --ignored` (ignoredPath), `worktree list --porcelain` (otherWorktrees), `diff --quiet` and `ls-files --others` (worktreePathDiffers), `hash-object` (worktreeBlob), `rev-parse --verify --quiet` (commitBlob) and `ls-tree -r` (treePathsWithBlob)"},
}

// rawGitMutationMinScannedFiles is the vacuous-scan floor for this guard's
// one-package walk: fewer than 4 production .go files found in the package
// means the walk is misconfigured rather than the package having genuinely
// shrunk.
const rawGitMutationMinScannedFiles = 4

// TestNoRawGitMutation_WebsterProductionSource walks internal/websterengine's non-test .go files
// and fails if any of them (other than a rawGitMutationAllowlist entry) contains the raw substring
// "gitrepo.New(" or "gitexec.Run(" — the two construction/call tokens a raw, fabric-bypassing
// git mutation would carry.
func TestNoRawGitMutation_WebsterProductionSource(t *testing.T) {
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
						"%s: contains banned raw-git-mutation token %q — mutating git in this package must dispatch through internal/fabricengine's warp-only methods (see CONSTRAINTS.md's Fabric Git Invariant), or add a rawGitMutationAllowlist entry in cmd/lyx/rawgitmutation_test.go with a reason if this is a new grandfathered read-only exemption",
						f.Rel, tok,
					))
				}
			}
		})
	}

	scankit.RequireFloor(t, scanned, rawGitMutationMinScannedFiles, "raw git mutation guard")
	allow.RequireNoStale(t)

	if len(failures) > 0 {
		t.Errorf("Fabric Git Invariant violated (see CONSTRAINTS.md):\n%s", strings.Join(failures, "\n"))
	}
}
