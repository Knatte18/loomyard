// snapshot.go — the snapshot-tracking read path: Fabric.SnapshotWarpSHA, the
// sole exported entry point onto the Snapshot-trailer history the write path
// (trailer.go's appendSnapshotTrailers, threaded through commitWeftLocked)
// already records. Per the trailer-is-truth-no-new-cache Shared Decision,
// this file adds no index and no cache of its own — it scans the same
// generalized trailer history index.go's scanWarpSHATrailers already builds
// for RebuildIndex, on demand, every call.

package fabricengine

// SnapshotWarpSHA returns the warp SHA recorded under tag by the newest weft
// commit (in scanWarpSHATrailers's topological order — see its doc comment)
// carrying a "Snapshot: <tag>" trailer on the current weft branch. Because no
// commit is ever listed before one of its own descendants in that order,
// "first record whose snapshotTags contains tag" is the same as "newest
// recorded baseline for tag".
//
// Four contract points are deliberate decisions, not implementation details,
// and are spelled out here rather than left for a caller to infer.
//
//  1. A tag never recorded in the current branch's history returns ("", nil),
//     not an error: absent is a normal state (matching the retired
//     gitrepo.SnapshotSHA contract exactly), and a first-ever consumer run
//     must be able to read "no baseline, generate everything" with no
//     special-casing. This is deliberately unlike index.go's
//     ErrNoCorrespondence, where a miss signals broken bookkeeping rather
//     than a legitimate first-run state.
//  2. tag is matched byte-exactly and is NOT validated against
//     snapshotTagPattern: validation belongs on the write path and already
//     lives there (see validateSnapshotTag); a tag that could not have been
//     written can never match anything, and fuzzy matching would let a
//     lookup for "Raddle" silently resolve a baseline recorded as "raddle",
//     hiding a caller bug behind a false-positive convenience.
//  3. A dangling Warp-SHA — the newest commit carrying tag names a warp SHA
//     that no longer exists because warp history was rewritten — is returned
//     RAW, with a nil error: SnapshotWarpSHA does not validate the SHA, does
//     not skip back to an older tagged commit, and does not collapse the
//     answer to ("", nil). This is the same validate-at-use posture
//     RebuildIndex already takes for its own trailer values. Skipping to the
//     next-newest tagged commit would silently answer with an OLDER baseline
//     and under-report staleness — the one failure direction that loses
//     data; collapsing to absent would conflate "never recorded" with
//     "recorded, then rewritten" for no benefit, since both drive the same
//     consumer action. The intended three-step consumer idiom is: read the
//     SHA via SnapshotWarpSHA, check f.Warp.SHAExists(sha), then call
//     f.Warp.ChangedFilesSince(sha) only if it exists — treating a missing
//     SHA as total staleness. This is not a burden invented here:
//     ChangedFilesSince's own doc comment already asks every caller to check
//     SHAExists first; the naive two-step composition would hard-error on
//     exactly the rebase case this mechanism exists to survive.
//  4. The reader is PER-BRANCH, because it scans the current weft branch's
//     history only. A snapshot recorded on another branch reads as absent
//     after a coordinated Checkout switches this pair onto that branch —
//     the intended behaviour and the safe failure direction, since the
//     consumer then regenerates from scratch rather than trusting a
//     cross-branch baseline. Contrast this with refreshCorrIndexAfterSwitch:
//     the correspondence index is a per-worktree FILE that survives a branch
//     switch and can therefore go on answering cross-branch, which is
//     exactly why Checkout must discard and rebuild it. SnapshotWarpSHA
//     holds no state of its own to discard — it simply stops seeing the
//     other branch's commits the moment the weft worktree switches.
func (f *Fabric) SnapshotWarpSHA(tag string) (string, error) {
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
