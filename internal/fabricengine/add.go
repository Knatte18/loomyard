// add.go implements the transactional Add: it creates the warp worktree, portal, and launchers,
// wires junctions, records and commits the pair's parent-branch and parent-worktree provenance, then pushes last,
// performing a best-effort full rollback on any post-creation failure so a partial worktree PAIR
// is never left behind.
// The rollback also deletes the warp branch this Add created, under any branch_prefix, and on origin only the one ref this Add pushed — see rollbackAdd.
// Whether the pair is live, which branches origin lends it and whether a leftover remote branch from a removed pair is replaceable are all decided at pre-flight, before the first mutation (see remoteleftover.go).
// The weft side always uses the suffixed branch produced by RecordsBranchName.

package fabricengine

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// addBeforeWeftReplaceHook, when non-nil, runs at step 12 just before an archived leftover weft branch is replaced.
// It is nil in production; only a non-parallel test sets it, through SetAddBeforeWeftReplaceHookForTest.
var addBeforeWeftReplaceHook func()

// AddOptions controls optional behaviour for Add.
// It is an alias of SyncOptions (same SkipGit/SkipPush field shape as warp's own AddOptions) rather
// than a distinct type, so Add can pass opts straight through to pushWeftBranch, which already
// takes SyncOptions.
// Tests pass these directly instead of relying on environment variables, which makes t.Parallel()
// safe.
type AddOptions = SyncOptions

// AddResult contains the result of successfully adding a new worktree pair.
// It embeds MutationRecord, which carries the mutation record accumulated over the call.
type AddResult struct {
	MutationRecord
	Slug   string `json:"slug"`
	Branch string `json:"branch"`
	Path   string `json:"path"`
	Pushed bool   `json:"pushed"`
}

// ErrBranchExists is Add's refusal when the warp branch a slug would create already exists.
// It is a typed error so a caller standing somewhere the remedy below is wrong for -- the hub's
// prime, where "lyx fabric checkout" switches prime itself -- can recognise the refusal and word
// its own.
// Remove deliberately leaves the warp branch behind (it may carry unmerged work), so
// remove-then-re-add of the same slug lands here, and the message names the way forward rather
// than a bare "already exists".
type ErrBranchExists struct {
	// Branch is the existing warp branch, prefix included.
	Branch string
}

// Error implements the error interface, naming the two ways out for a caller inside a pair.
func (e *ErrBranchExists) Error() string {
	return fmt.Sprintf(
		"branch %q already exists; switch a pair onto it with \"lyx fabric checkout %s\", or delete it first with \"git branch -D %s\" if it is a leftover from a removed pair",
		e.Branch, e.Branch, e.Branch,
	)
}

// Add creates a new paired warp and weft git worktree with the given slug.
// It validates the slug, creates both worktrees, wires junctions, records and commits the pair's
// parent-branch and parent-worktree provenance (the acting worktree's name), and pushes branches, rolling back all changes on any failure.
// A newly forked weft branch does not inherit the parent's shed run records: the fork is no-checkout, so the run-records root never reaches the new worktree's disk,
// and the pair's first weft commit (the origin record's) also records the root's deletion.
// An adopted, already-existing weft branch keeps its own run records.
// A pair is live when its weft branch exists locally, or on origin with no archive/<slug>/* tag covering its tip (tags are consulted only when there is no local branch).
// A live pair is adopted rather than forked.
// The weft branch comes from the local copy, with a behind local copy fast-forwarded to origin's tip, or else from origin as a local tracking branch;
// the warp branch comes from origin when it is there, whatever its relation to HEAD.
// A weft worktree that already carries the origin record keeps it,
// so a task moved between machines keeps its recorded parent.
// Under SkipGit or SkipPush no origin is consulted: the pair is live only by a local weft branch,
// and the warp branch forks from HEAD.
func (t *Topology) Add(l *lyxcwd.Location, slug string, opts AddOptions) (res AddResult, err error) {
	rec := NewMutations(l.HubPath)
	defer func() { res.Mutations = rec.Snapshot() }()

	// (0) Slug validation, shared with Remove via slug.go's single validator.
	if err := validateWorktreeSlug(slug, t.cfg.Dirs()); err != nil {
		return AddResult{}, err
	}

	dirty, _, err := worktreeDirty(scopeTracked, l.WorktreePath())
	if err != nil {
		return AddResult{}, fmt.Errorf("read warp worktree status: %w", err)
	}
	if dirty {
		return AddResult{}, fmt.Errorf("source worktree has uncommitted changes")
	}

	warpBranch := t.cfg.BranchPrefix + slug
	weftBranch := RecordsBranchName(warpBranch)

	// rev-parse --verify is a mixed probe: its exit path is an answer ("the branch does not
	// exist"), recovered via errors.As, while its exec path returns a real error.
	_, verifyErr := gitexec.Run([]string{"rev-parse", "--verify", "refs/heads/" + warpBranch}, l.WorktreePath())
	if verifyErr == nil {
		return AddResult{}, &ErrBranchExists{Branch: warpBranch}
	}
	var verifyGitErr *gitexec.GitError
	if !errors.As(verifyErr, &verifyGitErr) {
		return AddResult{}, fmt.Errorf("check whether warp branch %q exists: %w", warpBranch, verifyErr)
	}

	target := WorktreePath(l, slug)
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		// Name the recovery path, since a directory here is not always a live pair: an empty leftover
		// stranded by an interrupted remove/reconcile is invisible to `lyx fabric list`/`prune`
		// (they enumerate only git-registered worktrees), so an operator hitting one otherwise has no
		// clue why the slug is blocked. If it IS a live pair, a different slug is the answer.
		return AddResult{}, fmt.Errorf(
			"worktree directory %q already exists; if it is a live pair, use a different slug; if it is a leftover directory from an interrupted remove/reconcile (which `lyx fabric list` and `prune` cannot see, as they list only git-registered worktrees), remove the directory and retry",
			target)
	}

	stdout, err := gitexec.Run([]string{"remote"}, l.WorktreePath())
	if err != nil {
		return AddResult{}, fmt.Errorf("list warp remotes: %w", err)
	}
	if strings.TrimSpace(stdout) == "" {
		return AddResult{}, fmt.Errorf("no remote configured")
	}

	if !weftRepoExists(l) {
		weftRepoRoot, weftRepoRootErr := RecordsRepoRoot(l)
		if weftRepoRootErr != nil {
			return AddResult{}, fmt.Errorf("resolve weft repo root: %w", weftRepoRootErr)
		}
		return AddResult{}, fmt.Errorf("no weft repo at %s; create the hub with \"lyx fabric clone\" first", weftRepoRoot)
	}

	weftTarget := RecordsWorktreePath(l, slug)
	if _, err := os.Stat(weftTarget); !os.IsNotExist(err) {
		return AddResult{}, fmt.Errorf("weft worktree directory already exists: %s", weftTarget)
	}

	weftBranchAlreadyExists := weftBranchExists(l, weftBranch)

	// Resolve parent warp branch before worktree creation to avoid partial state on failure.
	// This is a compound guard, not a two-message merge: the second disjunct of the old
	// exitCode != 0 || TrimSpace(stdout) == "HEAD" condition fires on a *successful* git call
	// (a detached HEAD), so there is no error to wrap on that arm — the two conditions stay
	// apart rather than collapsing into one errors.As recovery.
	headStdout, headErr := gitexec.Run([]string{"rev-parse", "--abbrev-ref", "HEAD"}, l.WorktreePath())
	if headErr != nil {
		var headGitErr *gitexec.GitError
		if !errors.As(headErr, &headGitErr) {
			return AddResult{}, fmt.Errorf("rev-parse abbrev-ref HEAD: %w", headErr)
		}
		return AddResult{}, fmt.Errorf("cannot spawn weft branch: warp worktree is on a detached HEAD or unborn branch")
	}
	if strings.TrimSpace(headStdout) == "HEAD" {
		return AddResult{}, fmt.Errorf("cannot spawn weft branch: warp worktree is on a detached HEAD or unborn branch")
	}
	parentBranch := strings.TrimSpace(headStdout)
	parentWeftBranch := RecordsBranchName(parentBranch)

	// Probe both origins before the first mutation: decide whether the pair is live, and refuse an unreplaceable leftover here rather than at step 11 or 12's push.
	// The weft answer is carried on: its fastForwardTo advances a behind local weft branch,
	// and an archived leftover is replaced at step 12 just before the push.
	weftOld := weftLeftover{live: weftBranchAlreadyExists}
	var warpAdoptTip string
	var warpPush addWarpBranch
	if !opts.SkipPush && !opts.SkipGit {
		weftOld, err = probeWeftLeftover(l, slug, weftBranch, weftBranchAlreadyExists)
		if err != nil {
			return AddResult{}, err
		}
		warpAdoptTip, warpPush.origin, err = probeWarpLeftover(l, slug, warpBranch, weftOld.live)
		if err != nil {
			return AddResult{}, err
		}
	}

	// An adopted warp branch is a local branch tracking origin's, created from the remote-tracking ref this fetch writes.
	warpStart, warpRemote := "", ""
	if warpAdoptTip != "" {
		warpStart, warpRemote = originRemoteName+"/"+warpBranch, originRemoteName
		if _, err := gitexec.Run([]string{"fetch", "--no-tags", originRemoteName, "refs/heads/" + warpBranch + ":refs/remotes/" + warpStart}, l.WorktreePath()); err != nil {
			return AddResult{}, fmt.Errorf("fetch warp branch %q from %q: %w", warpBranch, originRemoteName, err)
		}
	}
	warpTok, branchTok, err := createGitWorktree(rec, l.WorktreePath(), l.HubPath, target, warpBranch, func(worktreePath string) []string {
		if warpStart != "" {
			return []string{"worktree", "add", "--track", "-b", warpBranch, worktreePath, warpStart}
		}
		return []string{"worktree", "add", "-b", warpBranch, worktreePath}
	})
	if err != nil {
		return AddResult{}, fmt.Errorf("create worktree %q for branch %q failed: %w", target, warpBranch, err)
	}
	// The `-b warpBranch` argument to the worktree add above means this same call created a branch,
	// not merely a worktree; a branch is a ref, so it records via AppendRef rather than Append.
	rec.AppendRef(KindBranchCreated, warpBranch, refDetail("code", l.WorktreePath(), warpRemote))
	warpPush.tok = branchTok

	// Install the post-checkout hook now that the warp worktree exists.
	// Hook installation is non-fatal: a failure is logged but does not abort
	// Add or trigger the all-or-nothing rollback (the hook is belt-and-suspenders).
	if hookErr := InstallPostCheckoutHook(l); hookErr != nil {
		logger.Warn("fabricengine: post-checkout hook install failed (non-fatal)", "verb", "add", "slug", slug, "error", hookErr)
	}

	weftPath := RecordsWorktreePath(l, slug)
	weftRepoRoot, weftRepoRootErr := RecordsRepoRoot(l)
	if weftRepoRootErr != nil {
		_ = t.rollbackAdd(rec, l, slug, warpBranch, weftBranch, target, weftBranchAlreadyExists, warpTok, &warpPush)
		return AddResult{}, fmt.Errorf("resolve weft repo root: %w", weftRepoRootErr)
	}

	// A local weft branch behind origin advances to origin's tip.
	// git refuses a fetch into a branch that is not a fast-forward,
	// so a branch that moved since the pre-flight is never rewound or overwritten.
	if weftOld.fastForwardTo != "" {
		if _, err := gitexec.Run([]string{"fetch", "--no-tags", originRemoteName, "refs/heads/" + weftBranch + ":refs/heads/" + weftBranch}, weftRepoRoot); err != nil {
			_ = t.rollbackAdd(rec, l, slug, warpBranch, weftBranch, target, weftBranchAlreadyExists, warpTok, &warpPush)
			return AddResult{}, fmt.Errorf("fast-forward weft branch %q to its tip on %q failed: %w", weftBranch, originRemoteName, err)
		}
		rec.Append(KindRepoAdvanced, weftRepoRoot, weftBranch+" "+weftOld.fastForwardTo)
	}

	weftAdopted := false
	if weftOld.live {
		weftAdopted, _, err = resolveWeftBranch(rec, l, weftBranch, !opts.SkipGit && !opts.SkipPush)
		if err != nil {
			_ = t.rollbackAdd(rec, l, slug, warpBranch, weftBranch, target, weftBranchAlreadyExists, warpTok, &warpPush)
			return AddResult{}, err
		}
	}

	// runRecordsTracked is whether the fork point tracked run records that step 8 dropped from the new worktree.
	var runRecordsTracked bool
	if weftAdopted {
		// Adopt: git worktree add <path> <branch> (no -b, branch exists), through
		// containedWorktreeAdd so a symlink toggled at weftPath cannot carry the worktree outside the hub.
		err := containedWorktreeAdd(weftRepoRoot, l.HubPath, weftPath, func(worktreePath string) []string {
			return []string{"worktree", "add", worktreePath, weftBranch}
		})
		if err != nil {
			_ = t.rollbackAdd(rec, l, slug, warpBranch, weftBranch, target, weftBranchAlreadyExists, warpTok, &warpPush)
			return AddResult{}, fmt.Errorf("adopt weft worktree for branch %q failed: %w", weftBranch, err)
		}
		if _, err := ensureWeftLockDirAt(weftPath); err != nil {
			_ = t.rollbackAdd(rec, l, slug, warpBranch, weftBranch, target, weftBranchAlreadyExists, warpTok, &warpPush)
			return AddResult{}, fmt.Errorf("adopt weft worktree for branch %q failed: create weft lock dir in %q: %w", weftBranch, weftPath, err)
		}
	} else {
		// Create: fork from the parent's weft branch without checking the run-records root out, so the pair never inherits another run's seed or status;
		// step 10c commits the root's deletion.
		// The adopt path above drops nothing: an existing branch's own records are its own.
		var err error
		runRecordsTracked, err = createWeftWorktreeDroppingRuns(rec, l, slug, weftBranch, parentWeftBranch)
		if err != nil {
			_ = t.rollbackAdd(rec, l, slug, warpBranch, weftBranch, target, weftBranchAlreadyExists, warpTok, &warpPush)
			return AddResult{}, err
		}
	}

	if err := createPortal(rec, l, slug); err != nil {
		_ = t.rollbackAdd(rec, l, slug, warpBranch, weftBranch, target, weftBranchAlreadyExists, warpTok, &warpPush)
		return AddResult{}, err
	}

	if err := writeLaunchers(rec, l, slug); err != nil {
		_ = t.rollbackAdd(rec, l, slug, warpBranch, weftBranch, target, weftBranchAlreadyExists, warpTok, &warpPush)
		return AddResult{}, err
	}

	// (10b) Wire the new worktree's warp junctions eagerly, sourcing the wired
	// name-set from the repo-wide BoardDir base (not any per-pair weft base or
	// acting-worktree config) — every worktree must converge to the same
	// repo-wide pathspec, matching Checkout's re-point call. The weft worktree
	// already exists from step 8, so junction targets resolve. On failure, roll
	// back the whole pair via the existing post-step-7 path.
	names, err := RepoWiredNames(l)
	if err != nil {
		_ = t.rollbackAdd(rec, l, slug, warpBranch, weftBranch, target, weftBranchAlreadyExists, warpTok, &warpPush)
		return AddResult{}, fmt.Errorf("wire junctions: load fabric config: %w", err)
	}
	if err := WireJunctionsWith(rec, l, slug, names); err != nil {
		_ = t.rollbackAdd(rec, l, slug, warpBranch, weftBranch, target, weftBranchAlreadyExists, warpTok, &warpPush)
		return AddResult{}, fmt.Errorf("wire junctions: %w", err)
	}

	// (10c) Record and commit the pair's provenance now that the pair is fully wired, and
	// before step 11's warp push and step 12's weft push — the weft push that already runs at
	// step 12 carries this commit to the remote, so no new push call is added here.
	// An adopted weft worktree that already carries the record keeps it: neither rewritten nor committed,
	// so a task moved between machines keeps its recorded parent.
	// A forked weft always gets its own record, though it may carry the parent's.
	if _, statErr := os.Stat(OriginRecordPathFor(l, slug)); !weftAdopted || statErr != nil {
		if err := WriteOrigin(rec, l, slug, Origin{ParentBranch: parentBranch, ParentWorktree: l.WorktreeName}); err != nil {
			_ = t.rollbackAdd(rec, l, slug, warpBranch, weftBranch, target, weftBranchAlreadyExists, warpTok, &warpPush)
			return AddResult{}, fmt.Errorf("record parent branch: %w", err)
		}
		// The commit's sha and committed returns are not read here: CommitRecordsPaths records the
		// KindCommitCreated entry itself, at its own success site, per the
		// origin-record-records-both-its-write-and-its-commit decision.
		// The run-records root joins the commit's paths only when the fork tracked it:
		// git add on an untracked, absent root is a hard pathspec error, which a hub with no run records must not hit.
		commitPaths := []string{OriginRecordRel()}
		if runRecordsTracked {
			commitPaths = append(commitPaths, shedrun.RunsRootRel())
		}
		if _, _, err := CommitRecordsPaths(rec, weftPath, l.AnchorRel, commitPaths, "fabric: record parent branch for "+slug, opts); err != nil {
			_ = t.rollbackAdd(rec, l, slug, warpBranch, weftBranch, target, weftBranchAlreadyExists, warpTok, &warpPush)
			return AddResult{}, fmt.Errorf("commit parent branch record: %w", err)
		}
	}

	// (11) Push warp branch (LAST step for warp)
	pushedTip, err := gitexec.Run([]string{"rev-parse", "refs/heads/" + warpBranch}, l.WorktreePath())
	if err != nil {
		_ = t.rollbackAdd(rec, l, slug, warpBranch, weftBranch, target, weftBranchAlreadyExists, warpTok, &warpPush)
		return AddResult{}, fmt.Errorf("read warp branch %q tip: %w", warpBranch, err)
	}
	warpPush.pushAttempted = true
	warpPush.pushedSHA = strings.TrimSpace(pushedTip)
	if err := t.push.pushBranchWithRetry(l.WorktreePath(), warpBranch); err != nil {
		_ = t.rollbackAdd(rec, l, slug, warpBranch, weftBranch, target, weftBranchAlreadyExists, warpTok, &warpPush)
		return AddResult{}, fmt.Errorf("push branch %q failed: %w", warpBranch, err)
	}
	rec.AppendRef(KindBranchPushed, warpBranch, refDetail("code", l.WorktreePath(), "origin"))

	// (12) Replace an archived leftover of the weft branch on origin, then push the weft branch.
	// The lease pins the deletion to the tip the pre-flight proved archived, so a branch that moved since is refused.
	if weftOld.tip != "" {
		if addBeforeWeftReplaceHook != nil {
			addBeforeWeftReplaceHook()
		}
		_, delErr := deleteRemoteBranch(rec, remoteBranchRequest{
			what:      "replace archived leftover weft branch on origin",
			repoDir:   weftRepoRoot,
			remote:    originRemoteName,
			branch:    weftBranch,
			ownership: ownedPairWeftBranch(l, warpBranch),
			dirtiness: dirtyArchivedOnRemote(weftOld.tag),
			leaseSHA:  weftOld.tip,
			force:     false,
		})
		if delErr != nil {
			_ = t.rollbackAdd(rec, l, slug, warpBranch, weftBranch, target, weftBranchAlreadyExists, warpTok, &warpPush)
			return AddResult{}, fmt.Errorf("replace leftover weft branch %q on origin: %w", weftBranch, delErr)
		}
	}
	if err := pushWeftBranch(rec, l, slug, weftBranch, opts, t.push); err != nil {
		_ = t.rollbackAdd(rec, l, slug, warpBranch, weftBranch, target, weftBranchAlreadyExists, warpTok, &warpPush)
		return AddResult{}, err
	}

	return AddResult{
		Slug:   slug,
		Branch: warpBranch,
		Path:   target,
		// Pushed reflects whether the weft branch was actually pushed to the remote.
		// It is false when either SkipPush or SkipGit suppresses the push.
		Pushed: !opts.SkipPush && !opts.SkipGit,
	}, nil
}

// addWarpBranch is what Add knows about the warp branch it created, handed to rollbackAdd.
type addWarpBranch struct {
	// tok proves this Add created the branch; the zero value proves nothing.
	tok createdBranchToken
	// origin is what the pre-flight probe found on origin.
	origin warpOriginState
	// pushAttempted is whether step 11 started its push.
	pushAttempted bool
	// pushedSHA is the warp branch's tip read just before step 11's push.
	pushedSHA string
}

// rollbackAdd performs best-effort paired cleanup on Add failure, unwiring junctions,
// removing worktrees and branches, preserving pre-existing adopted weft branches.
// weftBranchAdopted is whether the local weft branch existed before this Add:
// a branch Add created, forked or taken from origin as a local tracking branch, is deleted, while a pre-existing one survives, a fast-forward Add made to it included (it is never rewound).
// The one origin branch it may delete is the warp branch this Add pushed at step 11.
// It does so only when the pre-flight probe found the branch absent from origin, step 11's push was attempted, and origin still holds it at the commit that push carried (a lease), so a branch that moved since is refused.
// No other origin branch is ever touched.
// warpTok is the token createGitWorktree minted when this Add call created the warp worktree at
// target; it is the ownership proof the gate's warp-side removal requires.
// warp carries the proof that this Add created the warp branch, which deletes the local branch whether it was forked or adopted from origin,
// and what step 11 did about origin.
// A refused or failed branch deletion is logged, not swallowed (this function's return is discarded by every caller), so a branch left behind is visible in the trace.
// Rollback never restores a remote weft branch Add's step 12 replaced:
// its content stays reachable from the archive tag, and recreating it would re-block the next retry.
// rec is Add's own recorder, threaded through to all six gate-bound calls this function reaches
// (its own removeGitWorktree and deleteBranch, plus removeWeftWorktree, removeWarpJunction,
// removePortal and removeLaunchers), so a rollback's own destructions land in the same record as
// Add's creations, in execution order.
// The origin record needs no removal step of its own: on the created-branch path the existing
// weft-worktree and weft-branch removal (step 1) takes the record with it, and on the
// adopted-weft-branch path the record's commit is deliberately left in place — reverting or
// resetting an adopted branch to undo one commit is exactly the pre-existing-history destruction
// the !weftBranchAdopted guard above exists to prevent.
func (t *Topology) rollbackAdd(rec *Mutations, l *lyxcwd.Location, slug, warpBranch, weftBranch, target string, weftBranchAdopted bool, warpTok createdToken, warp *addWarpBranch) error {
	var firstErr error

	// (1) Remove the weft worktree; delete the weft branch only when this Add
	// created it, so a rollback never destroys pre-existing weft history.
	// remote is hardcoded false here — not because the branch was never pushed (Add pushes the warp
	// branch at step (11) and the weft branch at step (12), so a rollback can genuinely face an
	// already-pushed branch on either side), but because an unattended best-effort rollback on an
	// error path the operator did not choose must not make a network-visible destructive change:
	// --remote is opt-in precisely because deleting a shared ref needs an explicit operator decision.
	if _, err := removeWeftWorktree(rec, l, slug, weftBranch, true, !weftBranchAdopted, false, t.cfg.BranchPrefix); err != nil {
		if firstErr == nil {
			firstErr = err
		}
	}

	// (1b) Remove warp junctions wired by step 10b, best-effort. Source names
	// from the repo-wide BoardDir base, mirroring Remove's step 5: a rollback
	// must not hard-fail when the repo-wide config is unreadable (Add is
	// failing already), so a load error falls back to names == nil, which
	// removes nothing rather than guessing a wiring set.
	names, namesErr := RepoWiredNames(l)
	if namesErr != nil {
		names = nil
	}
	if err := removeWarpJunction(rec, l, slug, names); err != nil {
		if firstErr == nil {
			firstErr = err
		}
	}

	// (2) Remove warp portal
	if err := removePortal(rec, l, slug); err != nil {
		if firstErr == nil {
			firstErr = err
		}
	}

	// (3) Remove warp launchers
	if err := removeLaunchers(rec, l, slug); err != nil {
		if firstErr == nil {
			firstErr = err
		}
	}

	// (4) Remove warp worktree
	removeReq := pathRequest{
		what:      "remove warp worktree",
		container: l.HubPath,
		target:    target,
		ownership: ownedFreshlyCreatedWorktree(warpTok),
		dirtiness: dirtinessNA("rollback of the worktree this Add created"),
		force:     true,
	}
	err := removeGitWorktree(rec, removeReq, l.WorktreePath())
	if refusalErr := surfaceRefusal(err); refusalErr != nil {
		if firstErr == nil {
			firstErr = refusalErr
		}
	} else if err != nil && firstErr == nil {
		firstErr = err
	}

	// (5) Delete the warp branch this Add created, locally, then on origin when this Add alone put it there.
	branchReq := branchRequest{
		what:      "delete warp branch",
		repoDir:   l.WorktreePath(),
		branch:    warpBranch,
		ownership: ownedCreatedBranch(l, warp.tok),
		dirtiness: dirtyCheckedOutBranch(),
		force:     false,
	}
	err = deleteBranch(rec, branchReq)
	if refusalErr := surfaceRefusal(err); refusalErr != nil {
		// Log the swallowed refusal: this function's return is discarded by every caller.
		var refusal *destructiveRefusal
		if errors.As(refusalErr, &refusal) {
			logger.Warn("fabricengine: rollbackAdd's warp-branch deletion was refused by the destructive gate; delete the branch with `git branch -D` before retrying", "branch", warpBranch, "check", string(refusal.Check))
		}
		if firstErr == nil {
			firstErr = refusalErr
		}
	} else if err != nil {
		logger.Warn("fabricengine: rollbackAdd's warp-branch deletion failed", "branch", warpBranch, "error", err)
		if firstErr == nil {
			firstErr = err
		}
	}
	if warp.origin == warpOriginAbsent && warp.pushAttempted {
		_, err = deleteRemoteBranch(rec, remoteBranchRequest{
			what:      "delete the warp branch this Add pushed",
			repoDir:   l.WorktreePath(),
			remote:    originRemoteName,
			branch:    warpBranch,
			ownership: ownedCreatedBranch(l, warp.tok),
			dirtiness: dirtyPushedByThisCall(),
			leaseSHA:  warp.pushedSHA,
			force:     false,
		})
		if err != nil {
			logger.Warn("fabricengine: rollbackAdd left the warp branch on origin: its deletion was refused or failed", "branch", warpBranch, "lease", warp.pushedSHA, "error", err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	// (6) Prune warp worktrees
	if _, err := gitexec.Run([]string{"worktree", "prune"}, l.WorktreePath()); err != nil {
		if firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}
