// weftgit.go — the weft-git content-sync verbs on Fabric: StatusWeft,
// CommitWeft, PushWeft, PullWeft, plus the package-level PushWeftAt for the
// detached-push child. CommitWeft's commit carries a Warp-SHA trailer and
// records the correspondence immediately — except on an unborn warp HEAD
// (see warpHeadSHA), where both are skipped for that one commit.

package fabricengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
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

// crossModuleMachineLocalExcludes are gitignore-syntax patterns for every
// round-loop module's machine-local, never-committed artifacts under
// _lyx/<module> — not just the caller's own module. This is what actually
// stops them from being tracked: CommitWeft's callers (builder's/webster's
// own weftCommit, and fabric's own `lyx fabric sync`/`lyx config --set`)
// each build a pathspec, but a caller can only exclude what it knows about,
// and fabric's own sync pathspec (internal/fabriccli/weft_verbs.go) has no
// exclusions at all — so a plain config sync used to sweep every module's
// lock files and pause flags into weft history permanently (see
// CONSTRAINTS.md's Weft Git Invariant, "Cross-module exclusions"). Seeding
// the exclude file here, at the one choke point every weft-git verb passes
// through, makes every committer correct by construction without fabric
// needing to import any module's CLI/engine package.
//
// Each pattern is `**/` + hubgeometry.LyxDirName + "/*/" + <name>, matching
// at ANY depth (multiple hubs at different RelPath depths share one weft
// checkout) and at exactly one module-name segment. This is gitignore glob
// syntax, not git pathspec syntax: a bare `*` here does NOT cross `/` (unlike
// the leading-wildcard pathspec bug CONSTRAINTS.md's "Anchored exclusions"
// bullet documents), so no per-RelPath anchoring is needed — `**/` alone
// handles arbitrary depth.
//
// "pause" and "prompts" are not sourced from hubgeometry — hubgeometry owns
// directory geometry, not the filenames a module chooses to write inside its
// own directory. They mirror builderengine.PauseFlagName,
// websterengine.PauseFlagName, and treadleengine.PauseFlagName (all
// literally "pause" by convention) and hubgeometry.WebsterPromptsDir's
// "prompts" leaf. fabricengine cannot import those packages to reference the
// constants directly: websterengine and perchengine already import
// fabricengine, so an import back would cycle. Wildcarding the module
// segment (rather than naming "builder"/"webster" specifically) means a
// future module adopting either convention is covered with no fabricengine
// change.
var crossModuleMachineLocalExcludes = []string{
	"**/" + hubgeometry.LyxDirName + "/*/*.lock",
	"**/" + hubgeometry.LyxDirName + "/*/pause",
	"**/" + hubgeometry.LyxDirName + "/*/prompts/",
}

// seedWeftArtifactExcludes appends fabric's own operational artifacts (the
// .weft/ lock directory and gitrepo's push lock file) and every module's
// cross-module machine-local artifacts (crossModuleMachineLocalExcludes) to
// the weft repo's .git/info/exclude, line-exact idempotent (the same
// discipline as seedGitExclude). Without this, every weft worktree that has
// ever run a weft-git verb reports the artifacts as untracked dirt forever:
// Remove's no-force dirty gate then refuses with a "run lyx fabric sync"
// hint that a pathspec-scoped sync can never satisfy. The exclude file lives
// in the repo's common gitdir, so one seeding covers every linked weft
// worktree, and — because excludes are evaluated at status time — it also
// heals worktrees that already carry the artifacts as untracked (though not
// ones where a prior sync already committed them; see CommitWeft's doc
// comment for that limit).
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
	// exactly one owner. crossModuleMachineLocalExcludes appends every
	// module's own machine-local patterns after fabric's own two.
	entries := append([]string{weftLockDirName + "/", gitrepo.PushLockFileName}, crossModuleMachineLocalExcludes...)
	for _, entry := range entries {
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

// warpHeadSHA returns the warp repo's current HEAD SHA. On a host repo with
// zero commits (a fresh `git init` -> `lyx init` -> `lyx config` first-run
// path, before the operator's first host commit), it reports unborn=true
// (sha="", err=nil) instead of propagating gitrepo.ErrNoCommits as a hard
// failure: pre-cutover, weftengine.Commit never touched the host repo and
// succeeded on exactly this path, and CommitWeft must not regress it just
// because it now reads warp HEAD for the Warp-SHA trailer. Any other
// CurrentSHA failure still propagates as a genuine error.
func (f *Fabric) warpHeadSHA() (sha string, unborn bool, err error) {
	sha, err = f.Warp.CurrentSHA()
	if err == nil {
		return sha, false, nil
	}
	if errors.Is(err, gitrepo.ErrNoCommits) {
		return "", true, nil
	}
	return "", false, err
}

// weftPathspecFilter filters pathspec entries before staging, so a caller's
// stale positive entry (e.g. "_pattern" in a worktree where nothing has ever
// been written there) never reaches `git add`, which fails its ENTIRE
// invocation — including every other, genuinely-matching entry — the moment
// one entry matches nothing at all.
//
// An entry is kept if either:
//   - it begins with ":" — git pathspec magic (an ":(exclude)..." entry from
//     internal/buildercli/weft.go, internal/webstercli/weft.go, or
//     internal/perchcli/run.go's cross-module exclusions). Magic entries are
//     always passed through untouched and NEVER evaluated for a match: they
//     do not name a path to check, and treating one as a plain path would
//     both mis-evaluate it and defeat its own purpose.
//   - it is a plain path that matches at least one path in the weft
//     worktree OR the index (see entryMatchesWeft). Untracked-in-worktree
//     must count: a brand-new "_pattern/PATTERN.md" is untracked at the
//     moment of its first commit, so a tracked-only check would drop the
//     very first PATTERN commit. Index-only must count too:
//     internal/initengine/undo.go commits a "_lyx" path that os.RemoveAll
//     has just deleted from the worktree, surviving only in the index, so a
//     worktree-existence-only check would silently break `lyx init --undo`.
//
// Returns the filtered entries and whether at least one non-magic (plain)
// entry survived the filter. When positive is false, CommitWeft must not
// call StageAndCommit at all, even with a non-empty filtered slice: handing
// git a pathspec made up of only ":(exclude)" entries and no positive entry
// is read by git as "everything except those," staging the entire weft
// worktree — the opposite of the no-op CommitWeft already promises for
// "nothing of ours to stage."
func weftPathspecFilter(weftPath string, pathspec []string) (filtered []string, positive bool, err error) {
	for _, entry := range pathspec {
		if strings.HasPrefix(entry, ":") {
			filtered = append(filtered, entry)
			continue
		}
		matches, err := entryMatchesWeft(weftPath, entry)
		if err != nil {
			return nil, false, err
		}
		if matches {
			filtered = append(filtered, entry)
			positive = true
		}
	}
	return filtered, positive, nil
}

// entryMatchesWeft reports whether pathspec entry matches at least one path
// tracked in the weft repo's index or present untracked in its worktree,
// via `git ls-files --cached --others -- <entry>` run with cwd at weftPath
// — the same anchor StageAndCommit's own `git add` uses, so this check can
// never disagree with the command it is filtering for. --cached covers an
// index-only path (already deleted from the worktree); --others covers a
// brand-new untracked file. Either alone would miss one of the two real
// callers this filter exists for — see weftPathspecFilter's doc comment.
func entryMatchesWeft(weftPath, entry string) (bool, error) {
	stdout, stderr, code, err := gitexec.RunGit([]string{"ls-files", "--cached", "--others", "--", entry}, weftPath)
	if err != nil {
		return false, fmt.Errorf("fabricengine: git ls-files --cached --others -- %s: %w", entry, err)
	}
	if code != 0 {
		return false, fmt.Errorf("fabricengine: git ls-files --cached --others -- %s in %s: %s", entry, weftPath, stderr)
	}
	return strings.TrimSpace(stdout) != "", nil
}

// CommitWeft stages pathspec-scoped changes in the weft worktree and commits
// them, under the fabric-layer write lock. Staging always goes through
// f.Weft.StageAndCommit's explicit pathspec list — CommitWeft never calls
// StageAllAndCommit, per gitrepo's doc.go consumer rules. Immediately before
// staging, pathspec is run through weftPathspecFilter (still inside the
// write lock): non-magic entries that match nothing in the worktree or
// index are dropped, and if no positive entry survives at all, CommitWeft
// returns ("", false, nil) without calling StageAndCommit — see
// weftPathspecFilter's doc comment for why that early return is not
// optional. When the warp repo already has a HEAD, the commit carries a
// Warp-SHA trailer naming it, and RecordCorrespondence is called immediately
// with the (pre-push) new weft SHA: this is the detached CLI push path's
// pre-push record, which self-corrects at lookup time if a later
// rebase-recovered push rewrites the SHA out from under it. When the warp
// repo has no commits yet (see warpHeadSHA), the commit lands with no
// trailer and no correspondence record — there is no warp SHA yet to name —
// and normal trailer/record behavior resumes on the first CommitWeft call
// after warp's first commit. Returns ("", false, nil) when opts.SkipGit is
// true, nothing was staged, or pathspec has already been fully removed from
// both the working tree and the index by a prior commit — CommitWeft
// tolerates git's "did not match any files" pathspec failure, which the
// shared gitrepo.StageAndCommit primitive does not special-case on its own
// (retained as a defense-in-depth fallback; weftPathspecFilter's own
// pre-check is what keeps this path from being reached in practice).
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

	warpSHA, unborn, err := f.warpHeadSHA()
	if err != nil {
		return "", false, fmt.Errorf("fabricengine: warp CurrentSHA: %w", err)
	}

	commitMessage := message
	if !unborn {
		commitMessage = appendWarpSHATrailer(message, warpSHA)
	}

	filteredPathspec, positive, err := weftPathspecFilter(f.weftPath, pathspec)
	if err != nil {
		return "", false, err
	}
	if !positive {
		return "", false, nil
	}

	sha, committed, err = f.Weft.StageAndCommit(commitMessage, filteredPathspec)
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
	if unborn {
		return sha, true, nil
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
