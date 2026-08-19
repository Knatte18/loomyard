//go:build integration

// mutationoracle.go is the truthfulness oracle: AssertRecordMatchesDiff cross-checks a call's
// fabricengine.Mutations record against the unfiltered manifest diff of the same call, in both
// directions -- omission (every diff change is accounted for by some record entry) and commission
// (every manifest-observable record entry is backed by a real diff change).
// It lives apart from manifest.go, matching the package's one-concern-per-file layout: manifest.go
// owns the filesystem capture and diff, this file owns judging a record against that diff.

package fabricengine_test

import (
	"path"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/weftname"
)

// manifestObservableKind classifies every fabricengine.Kind except KindFileWritten for the commission
// direction: true means every entry of that kind is manifest-observable and must be backed by a real
// diff change; false means the kind is a git-state kind, exempt from commission and asserted instead
// through the per-verb Expectation.Effect assertions, which read git directly rather than infer from a
// manifest CaptureManifest never fully carries (branch existence, for one, is deliberately not
// recorded at all).
// KindFileWritten is deliberately absent: whether one of its entries participates depends on its
// Target, not on its kind alone -- see participatesInCommission.
// Declaring every other kind here, rather than testing membership in one list, is what makes a newly
// added Kind fail loud (participatesInCommission's "known" return) instead of silently falling through
// unclassified.
var manifestObservableKind = map[fabricengine.Kind]bool{
	fabricengine.KindPathRemoved:     true,
	fabricengine.KindWorktreeRemoved: true,
	fabricengine.KindWorktreeCreated: true,
	fabricengine.KindDirCreated:      true,
	fabricengine.KindLinkRemoved:     true,
	fabricengine.KindLinkCreated:     true,

	fabricengine.KindBranchCreated:    false,
	fabricengine.KindBranchDeleted:    false,
	fabricengine.KindBranchPushed:     false,
	fabricengine.KindCommitCreated:    false,
	fabricengine.KindWorktreeReset:    false,
	fabricengine.KindWorktreeSwitched: false,
	fabricengine.KindPushSpawned:      false,
	fabricengine.KindRepoAdvanced:     false,
	fabricengine.KindMergeStaged:      false,
	fabricengine.KindMergeCommitted:   false,
}

// invertedBy maps a constructive kind to the kinds of a later entry, at the same Target, that undo it
// -- the pairs the commission direction folds away before asserting, since a mutation performed and
// then undone inside one call nets to zero in the before/after manifest even though the record
// correctly carries both entries.
// A worktree_created or dir_created is undone by either a worktree_removed or a path_removed at the
// same Target -- Add's mint-then-rollbackAdd tears a freshly minted worktree back down through
// removePath's os.RemoveAll fallback, not always through removeGitWorktree -- a link_created is undone
// only by a link_removed, matching reconcile's own dangling-link repoint, and a file_written is undone
// by a path_removed at the same Target: removeLaunchers' own script teardown (launchers.go) removes
// each generated script individually via removePath before removing the launcher directory itself, so
// Add's own rollback writes and then removes each launcher script at the identical target.
var invertedBy = map[fabricengine.Kind][]fabricengine.Kind{
	fabricengine.KindDirCreated:      {fabricengine.KindPathRemoved, fabricengine.KindWorktreeRemoved},
	fabricengine.KindWorktreeCreated: {fabricengine.KindPathRemoved, fabricengine.KindWorktreeRemoved},
	fabricengine.KindLinkCreated:     {fabricengine.KindLinkRemoved},
	fabricengine.KindFileWritten:     {fabricengine.KindPathRemoved},
}

// subtreeDestroyingKind reports whether e's primitive removed everything beneath its own Target, not
// merely the Target path itself: worktree_removed always does (`git worktree remove` deletes the whole
// checkout), and path_removed does only when its own Detail says "recursive" (removePath's
// os.RemoveAll branch) rather than "single" (os.Remove, a leaf file only).
func subtreeDestroyingKind(e fabricengine.Mutation) bool {
	switch e.Kind {
	case fabricengine.KindWorktreeRemoved:
		return true
	case fabricengine.KindPathRemoved:
		return e.Detail == "recursive"
	default:
		return false
	}
}

// constructiveKind reports whether kind is one invertedBy ever maps as a key -- the closed set of
// kinds netCommissionEffect's ancestor-sweep pass considers for exemption when a later entry destroys
// their whole containing subtree, rather than their own exact Target.
func constructiveKind(kind fabricengine.Kind) bool {
	_, ok := invertedBy[kind]
	return ok
}

// AssertRecordMatchesDiff cross-checks rec against unfiltered, the manifest diff computed with a nil
// permitted list, and fails tb naming the offending entry or path.
//
// The omission direction runs over unfiltered's raw entries: every diff change must be covered by some
// record entry's coverage set, matching segment-wise subtree containment (see pathAtOrBelowRoot in
// manifest.go, reused rather than reimplemented) plus the worktree-admin and directory-ancestor
// widenings this file's coverage rules add.
// The commission direction runs over rec's net effect: netCommissionEffect folds away a
// mint-then-undo pair first, then every surviving manifest-observable entry must be backed by at least
// one diff change at or beneath its Target.
func AssertRecordMatchesDiff(tb testing.TB, rec fabricengine.Mutations, unfiltered []Change) {
	tb.Helper()

	entries := rec.Entries()
	assertOmission(tb, entries, unfiltered)
	assertCommission(tb, entries, unfiltered)
}

// netCommissionEffect returns entries with every mint-then-undo pair removed, via two independent
// consumption passes over the original entries, in order.
//
// Pass one is invertedBy's exact-Target pairing: for each constructive entry, the earliest later,
// not-yet-consumed entry at the SAME Target whose Kind appears in invertedBy[entry.Kind] is paired with
// it, and both are dropped.
//
// Pass two widens this to ancestor sweep: a constructive entry with no exact-Target pairing is ALSO
// dropped when a later entry destroys its whole containing subtree -- worktree_removed, or a
// path_removed whose own Detail is "recursive" (removePath's os.RemoveAll branch, never its
// single-file os.Remove branch) -- at or above the constructive entry's own Target. This is what nets
// Add's own mint-then-rollbackAdd to zero for a warp junction wired inside the pair's own worktree: the
// junction's own link_removed can legitimately never fire (removeWarpJunction's best-effort call falls
// back to removing nothing when RepoWiredNames can't be read, per add.go's own doc comment), yet the
// junction is physically swept away regardless the moment rollback's `git worktree remove` deletes the
// whole worktree tree it lives inside. Only the constructive (earlier) entry is dropped by this pass --
// the destroying entry itself keeps its own, independent commission obligation (or is itself netted by
// pass one against its own same-Target mint, e.g. the worktree_created/worktree_removed pair).
//
// Either way, a mutation performed and then undone inside one call would otherwise report a false lie
// of commission, since the before/after manifest nets to zero for a path (or subtree) the record still
// names.
func netCommissionEffect(entries []fabricengine.Mutation) []fabricengine.Mutation {
	consumed := make([]bool, len(entries))

	for i, e := range entries {
		if consumed[i] {
			continue
		}
		inverts, ok := invertedBy[e.Kind]
		if !ok {
			continue
		}
		for j := i + 1; j < len(entries); j++ {
			if consumed[j] || entries[j].Target != e.Target || !containsKind(inverts, entries[j].Kind) {
				continue
			}
			consumed[i] = true
			consumed[j] = true
			break
		}
	}

	for i, e := range entries {
		if consumed[i] || !constructiveKind(e.Kind) {
			continue
		}
		for j := i + 1; j < len(entries); j++ {
			if !subtreeDestroyingKind(entries[j]) || !pathAtOrBelowRoot(e.Target, entries[j].Target) {
				continue
			}
			consumed[i] = true
			break
		}
	}

	var net []fabricengine.Mutation
	for i, e := range entries {
		if !consumed[i] {
			net = append(net, e)
		}
	}
	return net
}

// containsKind reports whether kinds contains want.
func containsKind(kinds []fabricengine.Kind, want fabricengine.Kind) bool {
	for _, k := range kinds {
		if k == want {
			return true
		}
	}
	return false
}

// assertCommission asserts the commission direction over entries' net effect (see
// netCommissionEffect): every surviving manifest-observable entry must be backed by at least one
// change in unfiltered at or beneath its Target.
//
// An entry recorded after a whole-hub teardown (a path_removed at ".", resetHub's own
// removePath(hubPath) call) is exempt too, for the same mechanical reason the "." target itself is:
// once nothing survives for a finer entry to name, a reconstruction that happens to land back on
// byte-identical content -- CloneHub{Reset: true} against an unchanged upstream, the steady-state case
// cloneHubResetRealHubCase exists to prove is not trivially refused -- is indistinguishable from a
// no-op to a content-hash diff, even though every recorded primitive genuinely ran. Nothing before a
// whole-hub teardown loses this exemption; only entries recorded at or after it do.
func assertCommission(tb testing.TB, entries []fabricengine.Mutation, unfiltered []Change) {
	tb.Helper()

	wholeHubTornDown := false
	for _, e := range netCommissionEffect(entries) {
		if e.Target == "." {
			// A "." target is always exempt from commission, unconditionally and mechanically:
			// CaptureManifest never emits a "." key (it returns early at path == hubRoot), so no
			// "."-targeted entry could ever find a matching change regardless of what it claims.
			if e.Kind == fabricengine.KindPathRemoved {
				wholeHubTornDown = true
			}
			continue
		}
		if wholeHubTornDown {
			continue
		}

		participates, known := participatesInCommission(e)
		if !known {
			tb.Fatalf("AssertRecordMatchesDiff: record entry kind %q at %q is not classified in manifestObservableKind -- add it to that table before this oracle can judge it", e.Kind, e.Target)
			continue
		}
		if !participates {
			continue
		}

		if anyChangeAtOrBelow(unfiltered, e.Target) {
			continue
		}
		// A worktree-rooted kind's own change can land entirely on its derived admin entry rather than
		// on its own Target -- a stale registration a physically-already-absent worktree directory
		// leaves behind, which Prune's own removeStalePair clears via `git worktree prune` -- so the
		// same widening entryCovers applies for omission also applies here.
		if worktreeRootedKind[e.Kind] && anyChangeAtOrBelow(unfiltered, worktreeAdminTarget(e.Target)) {
			continue
		}
		tb.Errorf("AssertRecordMatchesDiff: record claims %s at %q, but the manifest diff shows no change at or beneath it (lie of commission)", e.Kind, e.Target)
	}
}

// participatesInCommission reports whether e's kind is manifest-observable, and therefore subject to
// the commission direction, and whether its kind is classified at all.
// KindFileWritten is decided by its Target rather than by manifestObservableKind alone: a file_written
// under the ".git" metadata directory records git-state bookkeeping CaptureManifest excludes wholesale
// (see manifest.go's git allowlist), so only a file_written outside ".git" participates.
func participatesInCommission(e fabricengine.Mutation) (participates, known bool) {
	if e.Kind == fabricengine.KindFileWritten {
		return !underGitMetadataDir(e.Target), true
	}
	participates, known = manifestObservableKind[e.Kind]
	return participates, known
}

// underGitMetadataDir reports whether target names a path at or below a ".git" path segment -- the
// boundary participatesInCommission uses to split KindFileWritten, and manifest.go's own CaptureManifest
// uses to decide what it excludes wholesale beneath ".git".
func underGitMetadataDir(target string) bool {
	for _, seg := range strings.Split(target, "/") {
		if seg == gitDirName {
			return true
		}
	}
	return false
}

// anyChangeAtOrBelow reports whether unfiltered carries at least one change at or beneath root.
func anyChangeAtOrBelow(unfiltered []Change, root string) bool {
	for _, c := range unfiltered {
		if pathAtOrBelowRoot(c.Path, root) {
			return true
		}
	}
	return false
}

// assertOmission asserts the omission direction over entries' raw, unfolded form: every change in
// unfiltered must be covered by at least one entry's coverage set (entryCovers).
// Unlike assertCommission, this direction is not folded to net effect -- a path the diff shows as
// changed must be accounted for by something in the record, whether or not a later entry later undid
// it.
func assertOmission(tb testing.TB, entries []fabricengine.Mutation, unfiltered []Change) {
	tb.Helper()

	for _, c := range unfiltered {
		covered := false
		for _, e := range entries {
			if entryCovers(e, c) {
				covered = true
				break
			}
		}
		if !covered {
			tb.Errorf("AssertRecordMatchesDiff: manifest diff shows %s, but no record entry covers it (lie of omission)", c.describe())
		}
	}
}

// worktreeRootedKind is the closed set of kinds whose Target names a worktree root, per the coverage
// rule: worktree_created, worktree_removed, worktree_reset, worktree_switched, repo_advanced,
// merge_staged, merge_committed.
var worktreeRootedKind = map[fabricengine.Kind]bool{
	fabricengine.KindWorktreeCreated:  true,
	fabricengine.KindWorktreeRemoved:  true,
	fabricengine.KindWorktreeReset:    true,
	fabricengine.KindWorktreeSwitched: true,
	fabricengine.KindRepoAdvanced:     true,
	fabricengine.KindMergeStaged:      true,
	fabricengine.KindMergeCommitted:   true,
}

// entryCovers reports whether e's coverage set includes diff change c: segment-wise subtree
// containment on e.Target, widened for a worktree-rooted kind to also cover the derived
// <prime>/.git/worktrees/<name> admin entry, and widened further to cover every KindDir ancestor
// directory of e.Target -- the implicit directory chains fslink.CreateDirLink and writeLaunchers
// MkdirAll around the roots fabric records, and pruneEmptyAncestors reaps on the way out.
// A "." Target is exempt except for path_removed, per the asymmetry AssertRecordMatchesDiff's own doc
// comment states: resetHub's whole-hub teardown is the one "."-targeted entry that genuinely covers
// everything, since nothing survives it for a finer entry to name.
func entryCovers(e fabricengine.Mutation, c Change) bool {
	if e.Target == "." {
		return e.Kind == fabricengine.KindPathRemoved
	}

	if pathAtOrBelowRoot(c.Path, e.Target) {
		return true
	}

	if worktreeRootedKind[e.Kind] {
		if admin := worktreeAdminTarget(e.Target); pathAtOrBelowRoot(c.Path, admin) {
			return true
		}
	}

	if changeEntryIsDir(c) && isAncestorDirOf(c.Path, e.Target) {
		return true
	}

	return false
}

// worktreeAdminTarget derives the git-admin bookkeeping entry a worktree-rooted record entry's own
// creation or destruction additionally covers: the prime warp worktree's or prime weft sibling's own
// ".git/worktrees/<name>" directory.
//
// primeWorktreeAdminPermittedRoot and primeWeftAdminPermittedRoot (verbs.go) both take a WARP slug --
// the weft one applies weftname.SiblingPath to it internally -- so feeding either one a weft-side
// target that already carries the "-weft" suffix would double it (e.g.
// "warp-bare-weft/.git/worktrees/<slug>-weft-weft", a key that never exists). This function tests
// target's basename for weftname.Suffix first: a weft-side target strips the suffix and goes to
// primeWeftAdminPermittedRoot; a warp-side one goes to primeWorktreeAdminPermittedRoot unchanged.
func worktreeAdminTarget(target string) string {
	base := path.Base(target)
	if strings.HasSuffix(base, weftname.Suffix) {
		return primeWeftAdminPermittedRoot(strings.TrimSuffix(base, weftname.Suffix))
	}
	return primeWorktreeAdminPermittedRoot(base)
}

// changeEntryIsDir reports whether c's own recorded entry -- After if present, else Before -- carries
// Kind KindDir, the test the upward ancestor-coverage widening requires: a *file* or *link* appearing
// beside a recorded target is still an uncovered change, and only directory levels are admitted.
func changeEntryIsDir(c Change) bool {
	if c.After != nil {
		return c.After.Kind == KindDir
	}
	if c.Before != nil {
		return c.Before.Kind == KindDir
	}
	return false
}

// isAncestorDirOf reports whether candidate is a strict ancestor directory of target: target resolves
// at or below candidate, and candidate is not target itself.
func isAncestorDirOf(candidate, target string) bool {
	if candidate == target {
		return false
	}
	return pathAtOrBelowRoot(target, candidate)
}
