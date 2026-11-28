// originrecord.go holds the rules of a pair's origin record, of its beam mark and of the read of its remote task branch.

package fabriccore

// MarkBeamVerdict decides what marking a pair as a beam does with its origin record.
// mark asks for the mark to be written.
// missingWorktree and missingRecord report a record that is absent, by whether the records worktree stands.
// All three are false for a record that already carries the mark, which is left as it is.
func MarkBeamVerdict(recordFound, alreadyMarked, recordsWorktreePresent bool) (mark, missingWorktree, missingRecord bool) {
	switch {
	case !recordFound && !recordsWorktreePresent:
		return false, true, false
	case !recordFound:
		return false, false, true
	case alreadyMarked:
		return false, false, false
	}
	return true, false, false
}

// ReadOriginForVerdict decides whether another pair's origin record is read.
// An absent record is neither read nor an error.
// A record whose lock directory is absent is not read, and missingLockDir reports it.
func ReadOriginForVerdict(recordPresent, lockDirPresent bool) (read, missingLockDir bool) {
	switch {
	case !recordPresent:
		return false, false
	case !lockDirPresent:
		return false, true
	}
	return true, false
}

// RemoteOnly decides how far a read of the commits only origin's branch holds goes.
// tips maps each branch on origin to its tip, and is nil while origin's branches are not read yet.
// read is false when the repository has no origin remote, or tips lack branch.
// Otherwise it is true, and tip is branch's tip in tips.
func RemoteOnly(hasOrigin bool, tips map[string]string, branch string) (tip string, read bool) {
	if !hasOrigin {
		return "", false
	}
	if tips == nil {
		return "", true
	}
	tip, read = tips[branch]
	return tip, read
}
