// snapshot.go implements the read side of snapshot tags:
// snapshotWarpSHA returns the warp SHA recorded under a tag.
// It keeps no index or cache of its own and scans the Snapshot trailers of the weft history on every call.

package fabricengine

// snapshotWarpSHA returns the warp SHA recorded under tag by the newest weft commit on the current branch.
// Such a commit carries a "Snapshot: <tag>" trailer.
// A tag never recorded returns "" and a nil error, since an absent tag is normal on a first run.
// Tag matching is byte-exact.
// A dangling Warp-SHA is returned as recorded with a nil error, and the caller validates it at use.
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
