// anchor.go — the pure nearest-older-reachable-anchor walk Fabric.Pull's
// warp-rebase reconcile path builds on: given the correspondence index's
// recorded entries and an injected reachability predicate, find the newest
// entry whose warp SHA still survives on the (possibly rewritten) upstream
// tip. This is the one genuinely unit-testable core of the reconcile flow —
// see the discussion's TDD-candidate decision — so it takes the predicate as
// a parameter rather than calling git itself, keeping it git-free like
// corrindex.go.

package fabricengine

// reachableAnchor walks entries newest-to-oldest, looking for the first entry
// whose warp SHA reachable reports true. reachable must answer ancestry, not
// object-existence. Returns the first reachable entry and true; on reachable error,
// returns that error unchanged; if no entry is reachable, returns a zero entry,
// false, and nil error.
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
