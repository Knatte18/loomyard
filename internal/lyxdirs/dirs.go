// dirs.go declares the lyx directory-name tokens: the two worktree-level directories, the beam directory under a worktree's .lyx and the pause directory under the board-level .lyx.
// internal/lyxdirs is the sole declarer of either worktree-level literal, enforced by
// TestEnforcement_GeometryLiterals in internal/lyxcwd/enforcement_test.go.

package lyxdirs

// LyxDirName is the durable, fabric-synced, git-tracked lyx directory within a worktree.
// internal/lyxdirs is the sole declarer of this literal, enforced by
// TestEnforcement_GeometryLiterals in internal/lyxcwd/enforcement_test.go.
const LyxDirName = "_lyx"

// DotLyxDirName is the ephemeral, machine-bound, never-git-tracked sibling of LyxDirName.
// Every never-tracked file lives under it at the mirrored subpath of the _lyx content it relates
// to.
// internal/lyxdirs is the sole declarer of this literal, enforced by
// TestEnforcement_GeometryLiterals in internal/lyxcwd/enforcement_test.go.
const DotLyxDirName = ".lyx"

// PauseDirName is the directory under the hub's board-level DotLyxDirName that holds the pause record and its lock.
const PauseDirName = "pause"

// HibernateDirName is the directory under the hub's board-level DotLyxDirName that holds the hibernate request, its lock and the daemon's log.
const HibernateDirName = "hibernate"

// CreelDirName is the directory under the hub's board-level DotLyxDirName that holds creel's journal and the daemon lock.
const CreelDirName = "creel"

// FailedPushDirName is the directory under the hub's board-level DotLyxDirName that holds one failed-push record per repository side and branch.
const FailedPushDirName = "failed-push"

// RunProcDirName is the directory under the hub's board-level DotLyxDirName that holds the owner record of the processes a run starts.
const RunProcDirName = "runproc"

// AdoptDirName is the directory under the hub's board-level DotLyxDirName that holds the log of the test suite an adopt starts.
const AdoptDirName = "adopt"

// BeamDirName is the directory under a worktree's DotLyxDirName that holds dobby's lock, state record and markers.
const BeamDirName = "beam"
