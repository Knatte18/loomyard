// livestate_mutationoracle_test.go judges a call's fabricengine.Mutations record against the unfiltered manifest diff.
// AssertRecordMatchesDiff checks the record in two directions.
// Omission: every diff change is accounted for by some record entry.
// Commission: every manifest-observable record entry is backed by a real diff change.
// The filesystem capture and diff live in livestate_manifest_test.go; this file judges a record against that diff.

package fabricengine_test

import (
	"path"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/weftname"
)

// manifestObservableKind classifies each fabricengine.Kind, except KindFileWritten, for the commission direction.
// A true entry means every entry of that kind is manifest-observable and must be backed by a real diff change.
// A false entry marks a git-state kind: it is exempt from commission.
// It is asserted instead through the per-verb Expectation.Effect assertions, which read git directly.
// The manifest never fully carries that state; branch existence, for one, is deliberately not recorded at all.
// KindFileWritten is absent on purpose:
// whether one of its entries participates depends on its Target, not on its kind alone.
// Declaring every other kind here, rather than testing membership in one list, makes a new Kind fail loud.
// Otherwise a newly added Kind would fall through unclassified without a failure.
var manifestObservableKind = map[fabricengine.Kind]bool{
	fabricengine.KindPathRemoved:     true,
	fabricengine.KindWorktreeRemoved: true,
	fabricengine.KindWorktreeCreated: true,
	fabricengine.KindDirCreated:      true,
	fabricengine.KindPathRenamed:     true,
	fabricengine.KindLinkRemoved:     true,
	fabricengine.KindLinkCreated:     true,

	fabricengine.KindBranchCreated:       false,
	fabricengine.KindBranchDeleted:       false,
	fabricengine.KindRemoteBranchDeleted: false,
	fabricengine.KindRemoteBranchUpdated: false,
	fabricengine.KindBranchPushed:        false,
	fabricengine.KindCommitCreated:       false,
	fabricengine.KindWorktreeReset:       false,
	fabricengine.KindWorktreeSwitched:    false,
	fabricengine.KindPushSpawned:         false,
	fabricengine.KindRepoAdvanced:        false,
	fabricengine.KindMergeStaged:         false,
	fabricengine.KindMergeCommitted:      false,
	fabricengine.KindTagPushed:           false,
	fabricengine.KindCommitsDropped:      false,
	fabricengine.KindTagCreated:          false,
}

// invertedBy maps a constructive kind to the kinds of a later entry at the same Target that undo it.
// The commission direction folds these pairs away before asserting.
// Within one call, such a pair nets to zero in the before/after manifest, though the record carries both entries.
// A worktree_created or dir_created is undone by a worktree_removed or a path_removed at the same Target.
// A rollback may tear a freshly minted worktree down through the os.RemoveAll fallback, not only through git.
// A link_created is undone only by a link_removed, including a dangling link repointed during reconcile.
// A file_written is undone by a path_removed at the same Target.
var invertedBy = map[fabricengine.Kind][]fabricengine.Kind{
	fabricengine.KindDirCreated:      {fabricengine.KindPathRemoved, fabricengine.KindWorktreeRemoved},
	fabricengine.KindWorktreeCreated: {fabricengine.KindPathRemoved, fabricengine.KindWorktreeRemoved},
	fabricengine.KindLinkCreated:     {fabricengine.KindLinkRemoved},
	fabricengine.KindFileWritten:     {fabricengine.KindPathRemoved},
}

// subtreeDestroyingKind reports whether e's primitive removed everything beneath its own Target.
// The Target path itself is not enough; the removal must reach its contents.
// A worktree_removed always does, since `git worktree remove` deletes the whole checkout.
// A path_removed does only when its Detail is "recursive" (os.RemoveAll), not "single" (os.Remove of a leaf file).
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

// constructiveKind reports whether kind is a key of invertedBy.
// A later entry that destroys the whole containing subtree of such a kind can exempt it without an exact-Target undo.
func constructiveKind(kind fabricengine.Kind) bool {
	_, ok := invertedBy[kind]
	return ok
}

// AssertRecordMatchesDiff checks rec against unfiltered, the manifest diff computed with a nil permitted list.
// It fails tb naming the offending entry or path.
//
// The omission direction checks unfiltered's raw entries.
// Every diff change must be covered by some record entry's coverage set.
// Coverage is segment-wise subtree containment, plus the worktree-admin and directory-ancestor widenings.
//
// The commission direction checks rec's net effect.
// Net effect drops every mint-then-undo pair first.
// Each surviving manifest-observable entry must then be backed by at least one diff change at or beneath its Target.
func AssertRecordMatchesDiff(tb testing.TB, rec fabricengine.Mutations, unfiltered []Change) {
	tb.Helper()

	entries := rec.Entries()
	assertOmission(tb, entries, unfiltered)
	assertCommission(tb, entries, unfiltered)
}

// netCommissionEffect returns entries with every mint-then-undo pair removed.
// It consumes the original entries in two independent passes, in order.
//
// The first pass pairs each constructive entry with the earliest later entry at the same Target:
// one that is not yet consumed and whose Kind is in invertedBy[entry.Kind]; both are dropped.
//
// The second pass is an ancestor sweep.
// A constructive entry with no exact-Target pair is also dropped.
// That happens when a later entry destroys its whole containing subtree at or above the entry's own Target.
// The destroying entry is a worktree_removed.
// A path_removed counts only when its Detail is "recursive" (os.RemoveAll, never the single-file os.Remove).
// This nets a mint-then-rollback to zero for a warp junction wired inside the worktree the pair creates.
// The junction's own link_removed may never fire;
// its best-effort removal does nothing when the wired names cannot be read.
// The junction is still swept away when the worktree is removed.
// Only the constructive (earlier) entry is dropped by this pass.
// The destroying entry keeps its own commission obligation, or the first pass nets it against its own same-Target mint.
//
// Without these passes, a mutation performed and undone inside one call would report a false commission.
// The before/after manifest nets to zero for a path or subtree the record still names.
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

func containsKind(kinds []fabricengine.Kind, want fabricengine.Kind) bool {
	for _, k := range kinds {
		if k == want {
			return true
		}
	}
	return false
}

// assertCommission asserts the commission direction over the net effect of entries.
// Every surviving manifest-observable entry must be backed by at least one change in unfiltered.
// That change must sit at or beneath the entry's Target.
//
// An entry recorded at or after a whole-hub teardown (a path_removed at ".") is exempt too.
// Once nothing survives for a finer entry to name, a reconstruction may restore byte-identical content.
// A content-hash diff then cannot tell it from a no-op, even though every recorded primitive ran.
// CloneHub with Reset against an unchanged upstream is one such reconstruction.
// Nothing recorded before a whole-hub teardown loses this exemption.
func assertCommission(tb testing.TB, entries []fabricengine.Mutation, unfiltered []Change) {
	tb.Helper()

	wholeHubTornDown := false
	for _, e := range netCommissionEffect(entries) {
		if e.Target == "." {
			// A "." target is always exempt from commission.
			// The manifest capture never emits a "." key (it returns early at the hub root),
			// so no "."-targeted entry can find a matching change, whatever it claims.
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
		// on its own Target.
		// That happens when a stale registration is left behind after the worktree directory is already gone:
		// `git worktree prune` clears it.
		// So the change is also searched for under the derived admin target.
		if worktreeRootedKind[e.Kind] && anyChangeAtOrBelow(unfiltered, worktreeAdminTarget(e.Target)) {
			continue
		}
		tb.Errorf("AssertRecordMatchesDiff: record claims %s at %q, but the manifest diff shows no change at or beneath it (lie of commission)", e.Kind, e.Target)
	}
}

// participatesInCommission reports whether e's kind is manifest-observable, and therefore subject to
// the commission direction, and whether its kind is classified at all.
// KindFileWritten is decided by its Target rather than by the kind alone.
// A file_written under the ".git" directory records git-state bookkeeping the manifest capture excludes wholesale.
// So only a file_written outside ".git" participates.
func participatesInCommission(e fabricengine.Mutation) (participates, known bool) {
	if e.Kind == fabricengine.KindFileWritten {
		return !underGitMetadataDir(e.Target), true
	}
	participates, known = manifestObservableKind[e.Kind]
	return participates, known
}

// underGitMetadataDir reports whether target names a path at or below a ".git" path segment.
// It is the boundary that splits file_written entries for the commission check,
// and the manifest capture excludes everything beneath a ".git" segment the same way.
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
// unfiltered must be covered by at least one entry's coverage set.
// This direction is not folded to net effect.
// A path the diff shows as changed must be accounted for by something in the record.
// That holds even when a later entry undid the change.
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

// worktreeRootedKind is the closed set of kinds whose Target names a worktree root:
// worktree_created, worktree_removed, worktree_reset, worktree_switched, repo_advanced,
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

// entryCovers reports whether e's coverage set includes diff change c.
// Coverage is segment-wise subtree containment on e.Target.
// A worktree-rooted kind also covers its derived <prime>/.git/worktrees/<name> admin entry.
// It also covers every KindDir ancestor directory of e.Target.
// These directories are created implicitly around the recorded roots by fslink.CreateDirLink.
// A "." Target is exempt except for path_removed.
// A whole-hub teardown is the one "."-targeted entry that covers everything.
// Nothing survives it for a finer entry to name.
func entryCovers(e fabricengine.Mutation, c Change) bool {
	if e.Target == "." {
		return e.Kind == fabricengine.KindPathRemoved
	}

	if pathAtOrBelowRoot(c.Path, e.Target) {
		return true
	}

	// A rename's source subtree is gone from its old path.
	// A change at or below that source, which e.Detail holds relative to the hub, is the rename's own doing.
	if e.Kind == fabricengine.KindPathRenamed && pathAtOrBelowRoot(c.Path, e.Detail) {
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

// worktreeAdminTarget returns the git-admin entry a worktree-rooted record entry covers when created or destroyed.
// That entry is the ".git/worktrees/<name>" directory of the prime warp worktree or prime weft sibling.
// A weft-side target has its "-weft" suffix stripped first, because the weft lookup adds that suffix itself.
// A kept suffix would be doubled and name a key that never exists.
// A warp-side target goes to the warp lookup unchanged.
func worktreeAdminTarget(target string) string {
	base := path.Base(target)
	if strings.HasSuffix(base, weftname.Suffix) {
		return primeWeftAdminPermittedRoot(strings.TrimSuffix(base, weftname.Suffix))
	}
	return primeWorktreeAdminPermittedRoot(base)
}

// changeEntryIsDir reports whether the recorded entry of c, After if present and else Before, is a directory.
// Only directory levels may cover an ancestor.
// A file or link beside a recorded target is still an uncovered change.
func changeEntryIsDir(c Change) bool {
	if c.After != nil {
		return c.After.Kind == KindDir
	}
	if c.Before != nil {
		return c.Before.Kind == KindDir
	}
	return false
}

// isAncestorDirOf reports whether candidate is a strict ancestor directory of target.
func isAncestorDirOf(candidate, target string) bool {
	if candidate == target {
		return false
	}
	return pathAtOrBelowRoot(target, candidate)
}
