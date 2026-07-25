// weftgit.go — the weft-git parity verbs on Fabric: StatusWeft, CommitWeft,
// PushWeft, PullWeft, plus the package-level PushWeftAt for the detached-push
// child. These reproduce weftengine's observable behavior with one
// deliberate delta: CommitWeft's commit carries a Warp-SHA trailer and
// records the correspondence immediately, per the weft-git parity decision
// (fabric reuses weftengine's exact operational constants — env gates,
// default commit message, and write-lock path — so both modules serialize
// against each other during the parallel-build period).

package fabricengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lock"
)

const (
	// weftLockDirName and weftWriteLockFile name the same lock location
	// weftengine uses (its lockDirName/writeLockFile), so CommitWeft's write
	// lock and weftengine's write lock contend on the identical file during
	// the parallel-build period rather than silently racing past each other.
	weftLockDirName   = ".weft"
	weftWriteLockFile = "weft.write.lock"
)

// ensureWeftLockDir creates (idempotently) the .weft lock directory inside
// the weft worktree and returns its path.
func (f *Fabric) ensureWeftLockDir() (string, error) {
	dir := filepath.Join(f.weftPath, weftLockDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("fabricengine: mkdir weft lock dir: %w", err)
	}
	return dir, nil
}

// StatusWeft returns a content-sync status report for the weft worktree,
// matching weftengine.Status's keys exactly: weft_worktree, branch, dirty,
// ahead, behind — ahead/behind are nil rather than a zero count when no
// upstream is configured, so a caller can distinguish "not tracked" from
// "fully in sync".
func (f *Fabric) StatusWeft(pathspec []string) (map[string]any, error) {
	result := make(map[string]any)
	result["weft_worktree"] = f.weftPath

	branchOut, stderr, code, err := gitexec.RunGit([]string{"rev-parse", "--abbrev-ref", "HEAD"}, f.weftPath)
	if err != nil {
		return nil, fmt.Errorf("fabricengine: rev-parse --abbrev-ref HEAD: %w", err)
	}
	if code != 0 {
		return nil, fmt.Errorf("fabricengine: rev-parse --abbrev-ref HEAD in %s: %s", f.weftPath, stderr)
	}
	result["branch"] = strings.TrimSpace(branchOut)

	statusArgs := append([]string{"status", "--porcelain", "--"}, pathspec...)
	dirtyOut, stderr, code, err := gitexec.RunGit(statusArgs, f.weftPath)
	if err != nil {
		return nil, fmt.Errorf("fabricengine: git status: %w", err)
	}
	if code != 0 {
		return nil, fmt.Errorf("fabricengine: git status in %s: %s", f.weftPath, stderr)
	}
	result["dirty"] = strings.TrimSpace(dirtyOut) != ""

	// A non-zero exit from rev-list here means no upstream is configured —
	// a valid state, not a failure — so ahead/behind report nil rather than
	// propagating an error.
	aheadOut, _, code, err := gitexec.RunGit([]string{"rev-list", "--count", "@{u}..HEAD"}, f.weftPath)
	if err != nil {
		return nil, fmt.Errorf("fabricengine: rev-list ahead: %w", err)
	}
	if code != 0 {
		result["ahead"] = nil
		result["behind"] = nil
		return result, nil
	}

	var ahead int
	fmt.Sscanf(strings.TrimSpace(aheadOut), "%d", &ahead)
	result["ahead"] = ahead

	behindOut, stderr, code, err := gitexec.RunGit([]string{"rev-list", "--count", "HEAD..@{u}"}, f.weftPath)
	if err != nil {
		return nil, fmt.Errorf("fabricengine: rev-list behind: %w", err)
	}
	if code != 0 {
		return nil, fmt.Errorf("fabricengine: rev-list behind in %s: %s", f.weftPath, stderr)
	}
	var behind int
	fmt.Sscanf(strings.TrimSpace(behindOut), "%d", &behind)
	result["behind"] = behind

	return result, nil
}

// CommitWeft stages pathspec-scoped changes in the weft worktree and commits
// them with a Warp-SHA trailer naming the warp repo's current HEAD, under
// the fabric-layer write lock. Staging always goes through
// f.Weft.StageAndCommit's explicit pathspec list — CommitWeft never calls
// StageAllAndCommit, per gitrepo's doc.go consumer rules. On a real commit,
// RecordCorrespondence is called immediately with the (pre-push) new weft
// SHA: this is the detached CLI push path's pre-push record, which
// self-corrects at lookup time if a later rebase-recovered push rewrites the
// SHA out from under it. Returns ("", false, nil) when opts.SkipGit is true,
// nothing was staged, or pathspec has already been fully removed from both
// the working tree and the index by a prior commit — matching
// weftengine.Commit's identical tolerance of git's "did not match any
// files" pathspec failure, which the shared gitrepo.StageAndCommit
// primitive does not special-case on its own.
func (f *Fabric) CommitWeft(pathspec []string, message string, opts SyncOptions) (sha string, committed bool, err error) {
	if opts.SkipGit {
		return "", false, nil
	}

	lockDir, err := f.ensureWeftLockDir()
	if err != nil {
		return "", false, err
	}
	l, err := lock.AcquireWriteLock(filepath.Join(lockDir, weftWriteLockFile))
	if err != nil {
		return "", false, fmt.Errorf("fabricengine: acquire weft write lock: %w", err)
	}
	defer l.Release()

	warpSHA, err := f.Warp.CurrentSHA()
	if err != nil {
		return "", false, fmt.Errorf("fabricengine: warp CurrentSHA: %w", err)
	}

	sha, committed, err = f.Weft.StageAndCommit(appendWarpSHATrailer(message, warpSHA), pathspec)
	if err != nil {
		// gitrepo.StageAndCommit's `git add --` does not tolerate a pathspec
		// that no longer matches anything at all, on disk or in the index —
		// unlike weftengine.Commit's explicit tolerance of this exact
		// message. Treat it the same way here: nothing of ours to stage, not
		// a hard failure. Any other add/commit failure still propagates.
		if strings.Contains(err.Error(), "did not match any files") {
			return "", false, nil
		}
		return "", false, err
	}
	if !committed {
		return "", false, nil
	}

	// The commit already exists on disk at this point; a RecordCorrespondence
	// failure does not undo it, so the commit SHA and committed=true are
	// still reported alongside the error rather than swallowed — the caller
	// can decide whether to retry recording (or rely on RebuildIndex healing
	// it later) without losing track of the commit that did land.
	if err := f.RecordCorrespondence(warpSHA, sha); err != nil {
		return sha, true, err
	}

	return sha, true, nil
}

// PushWeft pushes unpushed weft commits, matching weftengine.Push's
// SkipGit/SkipPush gating. Serialization reuses gitrepo's PushCoalesced —
// its .gitrepo-push.lock is the push serialization; fabric ports no separate
// weft push lock, per the weft-git parity decision.
func (f *Fabric) PushWeft(opts SyncOptions) error {
	if opts.SkipGit || opts.SkipPush {
		return nil
	}
	return f.Weft.PushCoalesced()
}

// PullWeft fast-forwards the weft worktree from its upstream, matching
// weftengine.Pull's SkipGit gating.
func (f *Fabric) PullWeft(opts SyncOptions) error {
	if opts.SkipGit {
		return nil
	}
	return f.Weft.Pull()
}

// PushWeftAt pushes unpushed commits at weftPath directly, with no Fabric
// instance and no warp path involved — the detached-push child's entry
// point, mirroring weftcli's bypass push (spawnPush). Gating matches
// PushWeft exactly.
func PushWeftAt(weftPath string, opts SyncOptions) error {
	if opts.SkipGit || opts.SkipPush {
		return nil
	}
	return gitrepo.New(weftPath).PushCoalesced()
}
