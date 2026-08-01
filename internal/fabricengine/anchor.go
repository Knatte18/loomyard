// anchor.go — the pure nearest-older-reachable-anchor walk Fabric.Pull's
// warp-rebase reconcile path builds on: given the correspondence index's
// recorded entries and an injected reachability predicate, find the newest
// entry whose warp SHA still survives on the (possibly rewritten) upstream
// tip. This is the one genuinely unit-testable core of the reconcile flow —
// see the discussion's TDD-candidate decision — so it takes the predicate as
// a parameter rather than calling git itself, keeping it git-free like
// corrindex.go.

package fabricengine

// reachableAnchor walks entries — the correspondence index's WarpSeq-sorted
// slice, as corrIndex.entries() returns it — newest-to-oldest, looking for
// the first entry whose warp SHA reachable reports true. Because entries is
// sorted by WarpSeq ASCENDING, "newest" means the highest index, so the walk
// runs from len(entries)-1 down to 0, not a plain forward range.
//
// reachable must answer ancestry ("is this warp SHA still reachable from the
// current upstream tip"), never mere object-existence: a rebased-away commit
// survives fetch as a dangling object (git fetch never prunes), so an
// existence check would never detect the rewrite the reconcile flow exists
// to recover from — see the reachability-never-object-existence Shared
// Decision. The caller supplies this predicate (typically
// f.Warp.IsAncestor(warpSHA, upstreamSHA)) so this function itself never
// touches git.
//
// Returns the first reachable entry found and true. If reachable itself
// errors for any entry, the walk aborts immediately and the error is
// returned unchanged (never swallowed) alongside a zero corrEntry and false.
// If entries is empty or no entry is reachable, it returns a zero corrEntry,
// false, and a nil error — a legitimate "nothing survives" answer, not a
// failure.
func reachableAnchor(entries []corrEntry, reachable func(warpSHA string) (bool, error)) (corrEntry, bool, error) {
	for i := len(entries) - 1; i >= 0; i-- {
		ok, err := reachable(entries[i].WarpSHA)
		if err != nil {
			return corrEntry{}, false, err
		}
		if ok {
			return entries[i], true, nil
		}
	}
	return corrEntry{}, false, nil
}
