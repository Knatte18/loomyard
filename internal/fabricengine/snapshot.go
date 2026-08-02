// snapshot.go — the snapshot-tracking read path: Fabric.snapshotWarpSHA, the
// entry point onto the Snapshot-trailer history the write path (trailer.go's
// appendSnapshotTrailers, threaded through commitWeftLocked) already records.
// Per the trailer-is-truth-no-new-cache Shared Decision, this file adds no
// index and no cache of its own — it scans the same generalized trailer
// history index.go's scanWarpSHATrailers already builds for RebuildIndex, on
// demand, every call.

package fabricengine

// snapshotWarpSHA returns the warp SHA recorded under tag by the newest weft commit
// carrying a "Snapshot: <tag>" trailer on the current weft branch. A tag never
// recorded returns ("", nil), not an error (absent is a normal state for first-ever
// consumer runs). Tag matching is byte-exact. A dangling Warp-SHA is returned raw
// with nil error (validate-at-use pattern). The reader is per-branch only.
func (f *Fabric) snapshotWarpSHA(tag string) (string, error) {
	commits, err := f.scanWarpSHATrailers()
	if err != nil {
		return "", err
	}

	for _, c := range commits {
		for _, t := range c.snapshotTags {
			if t == tag {
				return c.warpSHA, nil
			}
		}
	}
	return "", nil
}
