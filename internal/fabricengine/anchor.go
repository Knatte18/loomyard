// anchor.go holds the walk behind Fabric.Pull's warp-rebase reconcile path:
// it finds the newest correspondence entry whose warp SHA still survives on the possibly rewritten upstream tip.
// The walk takes its reachability check as a parameter, so it stays free of git calls and can be unit-tested.

package fabricengine

// reachableAnchor walks entries from newest to oldest and returns the first one whose warp SHA reachable reports true.
// reachable must answer ancestry, not object existence.
// The walk stops at the first error from reachable and returns it unchanged.
// When no entry is reachable, it returns a zero entry, false, and nil.
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
