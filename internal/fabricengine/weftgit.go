// weftgit.go — the weft-git content-sync verbs on Fabric: StatusWeft,
// CommitWeft, PushWeft, PullWeft, plus the package-level PushWeftAt for the
// detached-push child. CommitWeft's commit carries a Warp-SHA trailer and
// records the correspondence immediately.

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
	// weftLockDirName and weftWriteLockFile name fabric's own write-lock
	// location, so every concurrent CommitWeft caller contends on the
	// identical file rather than racing past each other.
	weftLockDirName   = ".weft"
	weftWriteLockFile = "weft.write.lock"
)

// ensureWeftLockDir creates (idempotently) the .weft lock directory inside
// the weft worktree and returns its path. It also seeds the weft repo's
// git-exclude entries for fabric's own lock artifacts (see
// seedWeftArtifactExcludes): this is the choke point every lock-creating weft
// verb passes through before any lock file exists, so excluding here
// guarantees the artifacts never surface as untracked dirt.
func (f *Fabric) ensureWeftLockDir() (string, error) {
	dir := filepath.Join(f.weftPath, weftLockDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("fabricengine: mkdir weft lock dir: %w", err)
	}
	if err := seedWeftArtifactExcludes(f.weftPath); err != nil {
		return "", err
	}
	return dir, nil
}

// seedWeftArtifactExcludes appends fabric's own operational artifacts — the
// .weft/ lock directory and gitrepo's push lock file — to the weft repo's
// .git/info/exclude, line-exact idempotent (the same discipline as
// seedGitExclude). Without this, every weft worktree that has ever run a
// weft-git verb reports the artifacts as untracked dirt forever: Remove's
// no-force dirty gate then refuses with a "run lyx fabric sync" hint that a
// pathspec-scoped sync can never satisfy. The exclude file lives in the
// repo's common gitdir, so one seeding covers every linked weft worktree,
// and — because excludes are evaluated at status time — it also heals
// worktrees already carrying the lock files.
func seedWeftArtifactExcludes(weftPath string) error {
	stdout, stderr, exitCode, err := gitexec.RunGit(
		[]string{"rev-parse", "--git-path", "info/exclude"},
		weftPath,
	)
	if err != nil {
		return fmt.Errorf("fabricengine: resolve weft git exclude path: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("fabricengine: git rev-parse --git-path info/exclude in %s: %s", weftPath, stderr)
	}

	excludePath := strings.TrimSpace(stdout)
	if !filepath.IsAbs(excludePath) {
		excludePath = filepath.Join(weftPath, excludePath)
	}
	if err := os.MkdirAll(filepath.Dir(excludePath), 0o755); err != nil {
		return fmt.Errorf("fabricengine: mkdir weft exclude dir: %w", err)
	}

	content, err := os.ReadFile(excludePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("fabricengine: read weft exclude file: %w", err)
	}
	contentStr := string(content)

	// The trailing slash on the lock-dir entry scopes it to the directory,
	// matching gitignore semantics; the push lock is a single file at the
	// worktree root, named via gitrepo's exported constant so the literal has
	// exactly one owner.
	for _, entry := range []string{weftLockDirName + "/", gitrepo.PushLockFileName} {
		present := false
		for _, line := range strings.Split(contentStr, "\n") {
			if strings.TrimSpace(line) == entry {
				present = true
				break
			}
		}
		if present {
			continue
		}
		if contentStr != "" && !strings.HasSuffix(contentStr, "\n") {
			contentStr += "\n"
		}
		contentStr += entry + "\n"
	}

	if string(content) == contentStr {
		return nil
	}
	if err := os.WriteFile(excludePath, []byte(contentStr), 0o644); err != nil {
		return fmt.Errorf("fabricengine: write weft exclude file: %w", err)
	}
	return nil
}

// StatusWeft returns a content-sync status report for the weft worktree:
// weft_worktree, branch, dirty, ahead, behind — ahead/behind are nil rather
// than a zero count when no upstream is configured, so a caller can
// distinguish "not tracked" from "fully in sync".
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
	_, _ = fmt.Sscanf(strings.TrimSpace(aheadOut), "%d", &ahead)
	result["ahead"] = ahead

	behindOut, stderr, code, err := gitexec.RunGit([]string{"rev-list", "--count", "HEAD..@{u}"}, f.weftPath)
	if err != nil {
		return nil, fmt.Errorf("fabricengine: rev-list behind: %w", err)
	}
	if code != 0 {
		return nil, fmt.Errorf("fabricengine: rev-list behind in %s: %s", f.weftPath, stderr)
	}
	var behind int
	_, _ = fmt.Sscanf(strings.TrimSpace(behindOut), "%d", &behind)
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
// the working tree and the index by a prior commit — CommitWeft tolerates
// git's "did not match any files" pathspec failure, which the shared
// gitrepo.StageAndCommit primitive does not special-case on its own.
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
	defer func() { _ = l.Release() }()

	warpSHA, err := f.Warp.CurrentSHA()
	if err != nil {
		return "", false, fmt.Errorf("fabricengine: warp CurrentSHA: %w", err)
	}

	sha, committed, err = f.Weft.StageAndCommit(appendWarpSHATrailer(message, warpSHA), pathspec)
	if err != nil {
		// gitrepo.StageAndCommit's `git add --` does not tolerate a pathspec
		// that no longer matches anything at all, on disk or in the index.
		// Tolerate that case explicitly here: nothing of ours to stage, not
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

// PushWeft pushes unpushed weft commits, honoring SkipGit/SkipPush gating.
// Serialization reuses gitrepo's PushCoalesced — its .gitrepo-push.lock is
// the push serialization; fabric ports no separate weft push lock.
func (f *Fabric) PushWeft(opts SyncOptions) error {
	if opts.SkipGit || opts.SkipPush {
		return nil
	}
	return f.Weft.PushCoalesced()
}

// PullWeft fast-forwards the weft worktree from its upstream, honoring
// SkipGit gating.
func (f *Fabric) PullWeft(opts SyncOptions) error {
	if opts.SkipGit {
		return nil
	}
	return f.Weft.Pull()
}

// PushWeftAt pushes unpushed commits at weftPath directly, with no Fabric
// instance and no warp path involved — the detached-push child's bypass-push
// entry point (spawnPush). Gating matches PushWeft exactly.
func PushWeftAt(weftPath string, opts SyncOptions) error {
	if opts.SkipGit || opts.SkipPush {
		return nil
	}
	return gitrepo.New(weftPath).PushCoalesced()
}
