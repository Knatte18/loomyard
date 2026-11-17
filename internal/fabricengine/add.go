// add.go implements Add, which creates a paired warp and weft worktree and wires its junctions.
// A failure after the first worktree is made rolls the whole pair back, best-effort, and never touches origin.
// The pre-flight decides, before the first mutation, whether the pair is live and what origin lends it.

package fabricengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// addBeforeWeftReplaceHook, when non-nil, runs just before an archived leftover weft branch is replaced.
// It is nil in production; only a non-parallel test sets it, through `SetAddBeforeWeftReplaceHookForTest`.
var addBeforeWeftReplaceHook func()

// addFailAfterWarpHook, when non-nil, runs right after the warp worktree exists.
// A non-nil error from it fails the add there, so a test reaches rollback of both a created and an adopted warp branch.
// It is nil in production; only a non-parallel test sets it, through `SetAddFailAfterWarpHookForTest`.
var addFailAfterWarpHook func() error

// AddOptions holds the optional settings that change how a worktree pair is created.
// Setting SkipGit or SkipPush skips the origin probes and the archived-leftover replacement.
// A pair is then live only by a local weft branch.
// Tests set these fields directly, not through environment variables, which keeps them safe under t.Parallel().
type AddOptions struct {
	// SkipGit skips all git operations of the records helpers if true.
	SkipGit bool
	// SkipPush skips push operations if true.
	SkipPush bool
	// BackoffSleep sleeps for one delay of the backoff that a push waits out.
	// A push waits it out before it records a branch gone from origin or an unreachable origin.
	// Nil sleeps for the delay.
	BackoffSleep func(time.Duration)
	// Parent is the slug of the hub pair the new pair forks from, its last commit and its records branch.
	// Empty forks from the acting worktree's HEAD and refuses its tracked dirt.
	Parent string
	// Beam writes the beam mark into the new pair's origin record.
	Beam bool
	// AdoptLocal makes the pair over a local warp branch that already exists, instead of refusing with ErrBranchExists.
	// The adopted branch is checked out into the new worktree and is never deleted by a rollback.
	// The `--adopt-local` flag sets it, and so does a resume of a pair, together with AdoptOnly, to re-make a live pair.
	// An unattended run never adopts a stale branch left under a reused slug.
	AdoptLocal bool
	// AdoptOnly makes Add re-make a live pair and never fork one.
	// It is set only together with AdoptLocal.
	AdoptOnly bool
	// Anchor is the directory of the warp checkout the new pair takes as its anchor, relative to the warp root.
	// Empty, or equal to the fork point's anchor, keeps the fork point's own.
	// It is set only by a command naming the flag, and a pair over an existing weft branch refuses it.
	Anchor string
	// Hold, when set, takes a lock the caller serializes the hub's worktree writes under and returns its release.
	// Add takes it once its origin probes have started and before it reads the fork point,
	// and it releases it when Add returns.
	// The hold therefore covers only the probe time left when Add waits for the probes under it.
	// An error from Hold is returned as is, before any mutation.
	// Add calls Hold while it holds the pair's teardown lock.
	// If Hold waits on a lock whose holder waits for this pair's teardown lock, both stall until the wait runs out.
	// Calling Hold under that lock is safe only because a pair is never created and removed at once.
	Hold func() (release func(), err error)
}

// probesOrigins reports whether an add under these options consults origin: neither SkipGit nor SkipPush is set.
func (o AddOptions) probesOrigins() bool {
	return !o.SkipGit && !o.SkipPush
}

// syncOptions returns the options the records helpers take.
func (o AddOptions) syncOptions() SyncOptions {
	return SyncOptions{SkipGit: o.SkipGit, SkipPush: o.SkipPush, BackoffSleep: o.BackoffSleep}
}

// AddResult describes a newly created worktree pair by its slug, branch and path.
// It embeds MutationRecord, which records the mutations accumulated over the call.
type AddResult struct {
	MutationRecord
	Slug   string `json:"slug"`
	Branch string `json:"branch"`
	Path   string `json:"path"`
}

// ErrBranchExists reports that the warp branch a slug would create already exists.
// It is a typed error so a caller can recognise the refusal and word its own remedy.
// The message's remedy is wrong for a caller on the hub's prime, where `lym fabric checkout` switches prime itself.
// Removing a pair leaves its warp branch behind, since the branch may carry unmerged work.
// So re-adding a removed slug reaches this refusal.
// The message names the way forward rather than a bare "already exists".
type ErrBranchExists struct {
	// Branch is the existing warp branch, prefix included.
	Branch string
}

// Error implements the error interface, naming the ways out for a caller inside a pair.
func (e *ErrBranchExists) Error() string {
	return fmt.Sprintf(
		"branch %q already exists; switch a pair onto it with \"lym fabric checkout %s\", or delete it first with \"git branch -D %s\" if it is a leftover from a removed pair; or make a pair over it with \"lym fabric add <slug> --adopt-local\"",
		e.Branch, e.Branch, e.Branch,
	)
}

// ErrSourceDirty reports that the source worktree has tracked uncommitted changes.
// It is a typed error so a caller that checks the same condition before an add can read the paths it would refuse on.
type ErrSourceDirty struct {
	// Paths are the tracked uncommitted paths of the source worktree, relative to it.
	Paths []string
}

// Error implements the error interface, listing the dirty paths.
func (e *ErrSourceDirty) Error() string {
	return "source worktree has uncommitted changes: " + strings.Join(e.Paths, ", ")
}

// SourceDirtyPaths returns the tracked uncommitted paths of the source worktree l names, relative to it.
// Untracked files never count, so an empty result means the source has no tracked changes.
func SourceDirtyPaths(l *lymcwd.Location) ([]string, error) {
	absolute, err := trackedDirtyPaths(l.WorktreePath())
	if err != nil {
		return nil, err
	}
	relative := make([]string, 0, len(absolute))
	for _, path := range absolute {
		rel, err := filepath.Rel(l.WorktreePath(), path)
		if err != nil {
			return nil, fmt.Errorf("make %s relative to %s: %w", path, l.WorktreePath(), err)
		}
		relative = append(relative, rel)
	}
	return relative, nil
}

// Add creates a paired warp and weft git worktree for the given slug.
// It validates the slug, creates both worktrees and wires junctions.
// It records and commits the pair's parent-branch and parent-worktree provenance (the acting worktree's name).
// It rolls back all changes on any failure.
// It pushes neither branch; both stay committed locally for the caller to push.
// Once the slug is validated, Add takes the slug's teardown lock exclusively, before any origin probe or mutation.
// It holds the lock until it returns, rollback included, and it waits at most the topology's teardown wait.
// When the wait runs out, it returns an error wrapping ErrPairPushRunning, before any mutation.
// That error names re-running the command that made the pair.
// The caller's push of the new pair runs after Add has released the lock, and it takes the lock shared.
// The origin probes start in the background, and every return waits for them, a refusal before them included.
// A newly forked weft branch does not inherit the parent's shed run records.
// The fork is no-checkout, so the run-records root never reaches the new worktree's disk,
// and the pair's first weft commit, the origin record's, also records the root's deletion.
// An adopted, already-existing weft branch keeps its own run records.
// A pair is live when its weft branch exists locally, or on origin with no `archive/<slug>/*` tag covering its tip.
// Tags are consulted only when there is no local branch.
// A live pair is adopted rather than forked.
// The weft branch comes from the local copy, which is fast-forwarded to origin's tip when it is behind.
// Without a local copy, it comes from origin as a local tracking branch.
// The warp branch comes from origin when it is there, whatever its relation to HEAD.
// A weft worktree that already carries the origin record keeps it,
// so a task moved between machines keeps its recorded parent.
// Under SkipGit or SkipPush no origin is consulted:
// the pair is live only by a local weft branch, and the warp branch forks from HEAD.
// Add sets gc.auto=0 and maintenance.auto=false in both hub stores.
// It does so once its pre-flight refusals have passed and before its first mutation.
// A failure there is logged and never fails the Add.
// Add then drops the remote-tracking ref and the upstream setting of each branch the probes read as absent on origin.
// Such a branch is a removed pair's weft or warp branch deleted there.
// The drop keeps a reconcile before the new pair's first push from reading the branch as gone.
// A drop that fails fails the Add before any worktree is made, and the error names the branch.
// The dropped ref names a branch origin does not hold, so no origin head is lost.
// A rollback restores neither the ref nor the setting.
// An existing local warp branch is refused with ErrBranchExists, unless opts.AdoptLocal adopts it.
// With opts.AdoptLocal, Add refuses before any mutation a warp branch checked out in another worktree.
// It also refuses a records branch checked out in another records worktree.
// Both refusals are *ErrAdoptLocalCheckedOut, which names the worktree and the way forward.
// A records worktree directory that is not a registered worktree is refused with *ErrRecordsWorktreeNotPair.
// A task worktree path that already exists is refused with *ErrTaskPathTaken.
// One exception: a directory that no worktree registers, holds no `.git` entry and holds only files git ignores.
// Add deletes such a directory, the output a build left at a removed pair's path, and goes on.
// It deletes it only once its pre-flight refusals have passed, so a refused Add leaves the directory.
// A delete that fails returns an error wrapping ErrStrayPathKept and leaves the path on disk.
// With opts.AdoptOnly, Add re-makes a live pair and forks nothing.
// A weft branch that is not live is refused with *ErrNotLive (Records true).
// A task branch that is neither local nor on origin is refused with *ErrNotLive too.
// Both match ErrPairNotLive, and both are refused before any mutation.
// It checks no source dirt, reads no HEAD, resolves no parent and checks no fork point.
// An adopted weft with no origin record is written none.
// The adopted local task branch is never fetched or replaced from origin.
// A records branch checked out in another records worktree is refused even when the task branch comes from origin.
// A records worktree that is registered, not prunable, and on the weft branch is reused.
// Add then makes only the task worktree and wires the pair to it.
// For a behind weft branch, Add fast-forwards the branch inside the reused worktree.
// A rollback leaves the reused worktree and its branch in place.
// When the adopted branch is the warp's default branch, the pair is a root pair.
// Its origin record names neither a parent branch nor a parent worktree.
// With opts.Parent naming a pair of the hub, the warp branch forks from that pair's last commit.
// The weft branch forks from that pair's records branch, and the origin record names the parent's branch and slug.
// Add does not check the tracked dirt of either worktree,
// and a parent that cannot be forked from is refused before any mutation.
// A forked pair takes the anchor of its fork point through the marker its weft branch inherits.
// The fork point is the named parent, or else the acting worktree.
// The pair's junctions, scratch link and origin record sit at that anchor.
// A fork point whose own marker is present but empty is refused before any mutation.
// With opts.Anchor naming another directory, the pair takes that anchor instead.
// Add validates the anchor against the fork point's warp checkout before any mutation.
// It then moves the forked weft's content and marker there in one commit under a move record,
// and it removes that record once the pair is wired at the anchor.
// A failure anywhere rolls the whole pair back, the record with it.
// An adopted weft branch that already has an origin record keeps its parent fields.
// With opts.Beam, the record gains the beam mark in an origin commit.
func (t *Topology) Add(l *lymcwd.Location, slug string, opts AddOptions) (res AddResult, err error) {
	rec := NewMutations(l.HubPath)
	defer func() { res.Mutations = rec.Snapshot() }()

	// Validate the slug before any mutation.
	if err := validateWorktreeSlug(slug, t.cfg.Dirs()); err != nil {
		return AddResult{}, err
	}

	releaseTeardown, lockErr := t.lockPairTeardown(rec, l, slug, false, "re-run the command that made the pair")
	if lockErr != nil {
		return AddResult{}, lockErr
	}
	defer releaseTeardown()

	warpBranch := t.cfg.BranchPrefix + slug
	weftBranch := RecordsBranchName(warpBranch)

	// A missing branch is not an error; a failed read is.
	warpBranchExists, existsErr := gitrepo.New(l.WorktreePath()).BranchExists(warpBranch)
	if existsErr != nil {
		return AddResult{}, fmt.Errorf("check whether warp branch %q exists: %w", warpBranch, existsErr)
	}
	warpAdopted := warpBranchExists
	weftBranchAlreadyExists := weftBranchExists(l, weftBranch)

	// Probe both origins before the first mutation to decide whether the pair is live.
	// A leftover that cannot be replaced is refused here, rather than at the caller's later push.
	// Both probes run beside each other and beside the hold's wait and the pre-flight reads below,
	// so the add waits on one round of network, and the hold covers only what is left of it when the probes are waited for.
	// The answers are read only once the pre-flight refusals have passed,
	// so a refusal keeps its precedence over a probe's error.
	probeOrigins := opts.probesOrigins()
	// A local warp branch being adopted is the pair's own, so origin's copy of it is not consulted.
	// AdoptOnly is the exception: its probe only learns whether origin holds the branch,
	// and the adopted branch is never fetched or replaced from origin.
	probeWarp := probeOrigins && (!warpAdopted || opts.AdoptOnly)
	var (
		probes       sync.WaitGroup
		weftProbe    weftLeftover
		weftProbeErr error
		warpTip      string
		warpTipErr   error
	)
	if probeWarp {
		probes.Add(1)
		go func() {
			defer probes.Done()
			warpTip, warpTipErr = remoteHeadTip(l.WorktreePath(), warpBranch)
		}()
	}
	probes.Add(1)
	go func() {
		defer probes.Done()
		weftProbe, weftProbeErr = weftLiveness(l, slug, weftBranch, weftBranchAlreadyExists, opts)
	}()
	// No git call a probe started outlives Add.
	defer probes.Wait()

	if opts.Hold != nil {
		releaseHold, err := opts.Hold()
		if err != nil {
			return AddResult{}, err
		}
		defer releaseHold()
	}

	// A named parent replaces the acting worktree as the fork point, and the fork takes its last commit only.
	// The acting worktree's tracked dirt is then not checked, and a dirty parent is logged, not refused.
	// An adopt-only add forks nothing, so it checks no source dirt, resolves no parent and checks no fork point.
	var parent addParent
	if !opts.AdoptOnly && opts.Parent != "" {
		parent, err = resolveAddParent(l, t.cfg, opts.Parent)
		if err != nil {
			return AddResult{}, err
		}
		parentDirty, err := SourceDirtyPaths(parent.location)
		if err != nil {
			return AddResult{}, fmt.Errorf("read the status of parent %q: %w", opts.Parent, err)
		}
		if len(parentDirty) > 0 {
			logger.Info("fabricengine: forking from a parent with uncommitted changes, which the new pair does not carry", "verb", "add", "slug", slug, "parent", opts.Parent, "dirty", parentDirty)
		}
	} else if !opts.AdoptOnly {
		dirtyPaths, err := SourceDirtyPaths(l)
		if err != nil {
			return AddResult{}, fmt.Errorf("read warp worktree status: %w", err)
		}
		if len(dirtyPaths) > 0 {
			return AddResult{}, &ErrSourceDirty{Paths: dirtyPaths}
		}
	}

	// The fork point supplies the anchor a forked pair takes, through the marker its weft branch inherits.
	// An empty marker there would leave the pair with no readable anchor, so it is refused before anything is created.
	fork := l
	if !opts.AdoptOnly {
		if opts.Parent != "" {
			fork = parent.location
		}
		forkWeft := RecordsWorktreePath(fork, fork.WorktreeName)
		if err := refuseEmptyOwnMarker(forkWeft); err != nil {
			return AddResult{}, err
		}
		// A weft whose anchor is mid-change has no settled content for the new pair to fork from.
		moveStop, err := moveRecordStop(forkWeft)
		if err != nil {
			return AddResult{}, err
		}
		if moveStop != nil {
			return AddResult{}, fmt.Errorf("%w: %s", moveStop, moveStop.WayForward)
		}
	}
	// A requested anchor other than the fork point's is judged against the fork point's warp checkout,
	// so a refusal leaves nothing created.
	anchorTo, err := newPairAnchor(fork, opts.Anchor)
	if err != nil {
		return AddResult{}, err
	}
	// pairAt is the Location the new pair's junctions, scratch link and origin record are built at.
	// An adopted weft branch carries its own anchor,
	// and the adopt step below replaces pairAt with the adopted pair's Location.
	// A pair moved to a requested anchor takes it once the move has committed.
	pairAt := fork

	if warpBranchExists && !opts.AdoptLocal {
		return AddResult{}, &ErrBranchExists{Branch: warpBranch}
	}

	target := WorktreePath(l, slug)
	// A directory of only git-ignored output is judged here and deleted once the pre-flight refusals have passed.
	deleteIgnoredLeftover := false
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		if !ignoredTaskPathDeletable(l, target) {
			return AddResult{}, &ErrTaskPathTaken{Path: target}
		}
		deleteIgnoredLeftover = true
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
		return AddResult{}, fmt.Errorf("no weft repo at %s; create the hub with \"lym fabric clone\" first", weftRepoRoot)
	}

	weftTarget := RecordsWorktreePath(l, slug)
	// weftReused is whether an adopt-only add reuses a records worktree that already stands registered on the weft branch.
	weftReused := false
	if _, statErr := os.Stat(weftTarget); !os.IsNotExist(statErr) {
		if opts.AdoptOnly {
			recordsRoot, rootErr := RecordsRepoRoot(l)
			if rootErr != nil {
				return AddResult{}, fmt.Errorf("resolve weft repo root: %w", rootErr)
			}
			var reuseErr error
			weftReused, reuseErr = reusableRecordsWorktree(recordsRoot, weftTarget, weftBranch)
			if reuseErr != nil {
				return AddResult{}, reuseErr
			}
		}
		if !weftReused {
			return AddResult{}, &ErrRecordsWorktreeNotPair{Path: weftTarget}
		}
	}

	// An adopt-only add also guards a records branch another records worktree holds when the task branch comes from origin.
	// Otherwise the add would fail later with a plain worktree-add error.
	if opts.AdoptLocal && (warpAdopted || (opts.AdoptOnly && weftBranchAlreadyExists)) {
		reusedWeft := ""
		if weftReused {
			reusedWeft = weftTarget
		}
		if err := refuseAdoptLocalCheckedOut(l, slug, warpBranch, weftBranch, weftBranchAlreadyExists, reusedWeft); err != nil {
			return AddResult{}, err
		}
	}

	// Resolve parent warp branch before worktree creation to avoid partial state on failure.
	// A detached HEAD and an unborn branch are both refusals, and neither is a read error.
	// A named parent was resolved above; its branch, records branch and worktree name stand in for the acting worktree's.
	parentBranch, parentWeftBranch, parentWorktree := parent.branch, parent.recordsBranch, opts.Parent
	if opts.Parent == "" && !opts.AdoptOnly {
		warpRepo := gitrepo.New(l.WorktreePath())
		var detached bool
		var headErr error
		parentBranch, detached, headErr = warpRepo.HeadRef()
		if headErr != nil {
			return AddResult{}, fmt.Errorf("read warp HEAD: %w", headErr)
		}
		if _, shaErr := warpRepo.CurrentSHA(); detached || errors.Is(shaErr, gitrepo.ErrNoCommits) {
			return AddResult{}, fmt.Errorf("cannot spawn weft branch: warp worktree is on a detached HEAD or unborn branch")
		}
		parentWeftBranch, parentWorktree = RecordsBranchName(parentBranch), l.WorktreeName
	}

	// A pair over the warp's default branch is a root pair: its record names no parent, and landing a root pair is refused.
	recordedParentBranch, recordedParentWorktree := parentBranch, parentWorktree
	if opts.AdoptLocal && warpAdopted && !opts.AdoptOnly {
		defaultBranch, err := warpDefaultBranch(l, opts)
		if err != nil {
			return AddResult{}, err
		}
		if defaultBranch == warpBranch {
			recordedParentBranch, recordedParentWorktree = "", ""
		}
	}

	// The weft answer is carried on: its fastForwardTo advances a behind local weft branch,
	// and an archived leftover is replaced at step 11, so the caller's push of the weft branch is not rejected.
	// The warp branch's origin tip is judged once the weft answer says whether the pair is live.
	probes.Wait()
	if weftProbeErr != nil {
		return AddResult{}, weftProbeErr
	}
	weftOld := weftProbe
	var warpAdoptTip string
	var warpPush addWarpBranch
	// warpAbsentOnOrigin is whether the warp probe ran and read the branch absent on origin.
	warpAbsentOnOrigin := false
	if opts.AdoptOnly && !weftOld.live {
		return AddResult{}, &ErrNotLive{Branch: weftBranch, Records: true}
	}
	if probeWarp {
		if warpTipErr != nil {
			return AddResult{}, warpTipErr
		}
		var warpOnOrigin bool
		warpAdoptTip, warpOnOrigin, err = judgeWarpLeftover(l, slug, warpBranch, warpTip, weftOld.live)
		if err != nil {
			return AddResult{}, err
		}
		warpAbsentOnOrigin = !warpOnOrigin
		if warpAdopted {
			warpAdoptTip = ""
		}
	}
	if opts.AdoptOnly && !warpAdopted && warpAdoptTip == "" {
		return AddResult{}, &ErrNotLive{Branch: warpBranch}
	}

	// A live weft branch is adopted at the anchor it records, which no flag of this add moves.
	if opts.Anchor != "" && weftOld.live {
		return AddResult{}, fmt.Errorf("the weft branch %q already exists, so the pair is made over it at the anchor it records and cannot take the anchor %s; run the verb without the anchor, then `lym fabric anchor %s --set %s`",
			weftBranch, opts.Anchor, slug, opts.Anchor)
	}

	// The pre-flight refusals have passed: every exit from here, a rollback included, regenerates the hub workspace file.
	defer regenerateAfterVerb(rec, l, "add")

	// Switch both stores' auto gc off before the first mutation.
	if gcErr := disableStoreAutoGC(l); gcErr != nil {
		logger.Warn("fabricengine: disabling store auto gc failed (non-fatal)", "verb", "add", "slug", slug, "error", gcErr)
	}

	// A former pair's remote-tracking ref for a branch that origin does not hold is dropped here.
	// Left in place, it would make the next reconcile read the new pair's unpushed branch as gone.
	if err := dropStaleOriginTracking(l, weftBranch, weftOld.originAbsent, warpBranch, warpAbsentOnOrigin); err != nil {
		return AddResult{}, err
	}

	// An adopted warp branch is a local branch tracking origin's, created from the remote-tracking ref this fetch writes.
	warpStart, warpRemote := "", ""
	if warpAdoptTip != "" {
		warpStart, warpRemote = originRemoteName+"/"+warpBranch, originRemoteName
		if _, err := gitrepo.New(l.WorktreePath()).FetchWithArgs("--no-tags", originRemoteName, "refs/heads/"+warpBranch+":refs/remotes/"+warpStart); err != nil {
			return AddResult{}, fmt.Errorf("fetch warp branch %q from %q: %w", warpBranch, originRemoteName, err)
		}
	}
	if deleteIgnoredLeftover {
		// The pre-flight can take a network round, so the path is judged again just before the delete:
		// a file saved there since the first judgement must keep it.
		if !ignoredTaskPathDeletable(l, target) {
			return AddResult{}, &ErrTaskPathTaken{Path: target}
		}
		if err := t.removeIgnoredTaskPath(rec, l.HubPath, target); err != nil {
			return AddResult{}, err
		}
		logger.Info("fabricengine: deleted a directory of git-ignored output at the task worktree path", "verb", "add", "slug", slug, "path", target)
	}

	warpTok, branchTok, err := createGitWorktree(rec, l.WorktreePath(), l.HubPath, target, warpBranch, func(worktreePath string) []string {
		if warpAdopted {
			return []string{"worktree", "add", worktreePath, warpBranch}
		}
		if warpStart != "" {
			return []string{"worktree", "add", "--track", "-b", warpBranch, worktreePath, warpStart}
		}
		if parent.headSHA != "" {
			return []string{"worktree", "add", "-b", warpBranch, worktreePath, parent.headSHA}
		}
		return []string{"worktree", "add", "-b", warpBranch, worktreePath}
	})
	if err != nil {
		return AddResult{}, fmt.Errorf("create worktree %q for branch %q failed: %w", target, warpBranch, err)
	}
	if warpAdopted {
		// The branch predates this call: rollbackAdd must leave it as it found it.
		warpPush.adopted = true
	} else {
		// The `-b warpBranch` argument to the worktree add above means this same call created a branch, not merely a worktree.
		// A branch is a ref, so it records via AppendRef rather than Append.
		rec.AppendRef(KindBranchCreated, warpBranch, refDetail("code", l.WorktreePath(), warpRemote))
		warpPush.tok = branchTok
	}

	if addFailAfterWarpHook != nil {
		if hookErr := addFailAfterWarpHook(); hookErr != nil {
			_ = t.rollbackAdd(rec, pairAt, slug, warpBranch, weftBranch, target, weftBranchAlreadyExists, weftReused, warpTok, &warpPush)
			return AddResult{}, hookErr
		}
	}

	// Install the post-checkout hook now that the warp worktree exists.
	// Hook installation is non-fatal: a failure is logged,
	// and it does not abort Add or trigger the all-or-nothing rollback (the hook is belt-and-suspenders).
	if hookErr := InstallPostCheckoutHook(l); hookErr != nil {
		logger.Warn("fabricengine: post-checkout hook install failed (non-fatal)", "verb", "add", "slug", slug, "error", hookErr)
	}

	weftPath := RecordsWorktreePath(l, slug)
	weftRepoRoot, weftRepoRootErr := RecordsRepoRoot(l)
	if weftRepoRootErr != nil {
		_ = t.rollbackAdd(rec, pairAt, slug, warpBranch, weftBranch, target, weftBranchAlreadyExists, weftReused, warpTok, &warpPush)
		return AddResult{}, fmt.Errorf("resolve weft repo root: %w", weftRepoRootErr)
	}

	// A local weft branch behind origin advances to origin's tip.
	// git refuses a fetch into a branch that is not a fast-forward,
	// so a branch that moved since the pre-flight is never rewound or overwritten.
	// A branch a reused records worktree has checked out cannot be fetched into,
	// so its tip is fetched by SHA and merged inside that worktree.
	if weftOld.fastForwardTo != "" {
		var advanceErr error
		if weftReused {
			if _, advanceErr = gitrepo.New(weftRepoRoot).FetchWithArgs("--no-tags", originRemoteName, weftOld.fastForwardTo); advanceErr == nil {
				_, advanceErr = gitexec.Run([]string{"merge", "--ff-only", weftOld.fastForwardTo}, weftPath)
			}
		} else {
			_, advanceErr = gitrepo.New(weftRepoRoot).FetchWithArgs("--no-tags", originRemoteName, "refs/heads/"+weftBranch+":refs/heads/"+weftBranch)
		}
		if advanceErr != nil {
			_ = t.rollbackAdd(rec, pairAt, slug, warpBranch, weftBranch, target, weftBranchAlreadyExists, weftReused, warpTok, &warpPush)
			return AddResult{}, fmt.Errorf("fast-forward weft branch %q to its tip on %q failed: %w", weftBranch, originRemoteName, advanceErr)
		}
		rec.Append(KindRepoAdvanced, weftRepoRoot, weftBranch+" "+weftOld.fastForwardTo)
	}

	weftAdopted := false
	if weftOld.live {
		weftAdopted, _, err = resolveWeftBranch(rec, l, weftBranch, !opts.SkipGit && !opts.SkipPush)
		if err != nil {
			_ = t.rollbackAdd(rec, pairAt, slug, warpBranch, weftBranch, target, weftBranchAlreadyExists, weftReused, warpTok, &warpPush)
			return AddResult{}, err
		}
	}
	if weftAdopted {
		pairAt = l
	}

	// runRecordsTracked is whether the fork point tracked run records that step 8 dropped from the new worktree.
	var runRecordsTracked bool
	if weftAdopted {
		// Adopt: git worktree add <path> <branch> (no -b, branch exists), through containedWorktreeAdd,
		// so a symlink toggled at weftPath cannot carry the worktree outside the hub.
		// A reused records worktree already stands on the branch, so only the task worktree is made.
		if !weftReused {
			err := containedWorktreeAdd(weftRepoRoot, l.HubPath, weftPath, func(worktreePath string) []string {
				return []string{"worktree", "add", worktreePath, weftBranch}
			})
			if err != nil {
				_ = t.rollbackAdd(rec, pairAt, slug, warpBranch, weftBranch, target, weftBranchAlreadyExists, weftReused, warpTok, &warpPush)
				return AddResult{}, fmt.Errorf("adopt weft worktree for branch %q failed: %w", weftBranch, err)
			}
		}
		if _, err := ensureWeftLockDirAt(weftPath); err != nil {
			_ = t.rollbackAdd(rec, pairAt, slug, warpBranch, weftBranch, target, weftBranchAlreadyExists, weftReused, warpTok, &warpPush)
			return AddResult{}, fmt.Errorf("adopt weft worktree for branch %q failed: create weft lock dir in %q: %w", weftBranch, weftPath, err)
		}
		// An adopted branch keeps its own marker; one with none is given the anchor its tree implies,
		// and every path built below uses the adopted pair's own anchor.
		adoptNames, err := RepoWiredNames(l)
		if err != nil {
			_ = t.rollbackAdd(rec, pairAt, slug, warpBranch, weftBranch, target, weftBranchAlreadyExists, weftReused, warpTok, &warpPush)
			return AddResult{}, fmt.Errorf("adopt weft worktree for branch %q failed: load fabric config: %w", weftBranch, err)
		}
		adoptedOwn, err := recordAdoptedWeftAnchor(rec, l.HubPath, slug, weftPath, adoptNames)
		if err != nil {
			_ = t.rollbackAdd(rec, pairAt, slug, warpBranch, weftBranch, target, weftBranchAlreadyExists, weftReused, warpTok, &warpPush)
			return AddResult{}, fmt.Errorf("adopt weft worktree for branch %q failed: record its anchor: %w", weftBranch, err)
		}
		// pairAt stays on the acting worktree, which the rollback runs git in, and takes the adopted pair's own anchor.
		adoptedAt := *l
		adoptedAt.AnchorRel = adoptedOwn.AnchorRel
		pairAt = &adoptedAt
	} else {
		// Create: fork from the parent's weft branch without checking the run-records root out,
		// so the pair never inherits another run's seed or status.
		// Step 10c commits the root's deletion.
		// The adopt path above drops nothing: an existing branch's own records are its own.
		var err error
		runRecordsTracked, err = createWeftWorktreeDroppingRuns(rec, pairAt, slug, weftBranch, parentWeftBranch)
		if err != nil {
			_ = t.rollbackAdd(rec, pairAt, slug, warpBranch, weftBranch, target, weftBranchAlreadyExists, weftReused, warpTok, &warpPush)
			return AddResult{}, err
		}
		// A requested anchor moves the forked content before anything is wired or written at the new anchor.
		// The move commit carries the deletion of the run records the fork left off the disk, so step 10c has none to stage.
		if anchorTo != "" {
			if err := moveForkedWeftToAnchor(rec, fork, slug, anchorTo, runRecordsTracked); err != nil {
				_ = t.rollbackAdd(rec, pairAt, slug, warpBranch, weftBranch, target, weftBranchAlreadyExists, weftReused, warpTok, &warpPush)
				return AddResult{}, fmt.Errorf("move the weft of %q to the anchor %s: %w", slug, anchorTo, err)
			}
			runRecordsTracked = false
			movedAt := *fork
			movedAt.AnchorRel = anchorTo
			pairAt = &movedAt
		}
	}

	if t.beforeWire != nil {
		t.beforeWire()
	}

	// (10b) Wire the new worktree's warp junctions as soon as the weft worktree exists,
	// so a hub reader polling the half-built pair meets a warp worktree without its junction for as little time as possible.
	// The wired name-set comes from the repo-wide BoardDir base (not any per-pair weft base or acting-worktree config):
	// every worktree must converge to the same repo-wide pathspec, as a later checkout's re-point does.
	// The weft worktree already exists from step 8, so junction targets resolve.
	// On failure, roll back the whole pair via the post-step-7 path.
	names, err := RepoWiredNames(l)
	if err != nil {
		_ = t.rollbackAdd(rec, pairAt, slug, warpBranch, weftBranch, target, weftBranchAlreadyExists, weftReused, warpTok, &warpPush)
		return AddResult{}, fmt.Errorf("wire junctions: load fabric config: %w", err)
	}
	if err := WireJunctionsWith(rec, pairAt, slug, names); err != nil {
		_ = t.rollbackAdd(rec, pairAt, slug, warpBranch, weftBranch, target, weftBranchAlreadyExists, weftReused, warpTok, &warpPush)
		return AddResult{}, fmt.Errorf("wire junctions: %w", err)
	}

	// (10c) Record and commit the pair's provenance now that the pair is fully wired;
	// the caller's push of the weft branch carries this commit to the remote.
	// An adopted weft worktree that already carries the record keeps its parent fields,
	// so a task moved between machines keeps its recorded parent.
	// Only a beam request not yet marked on it adds the mark, in the same commit.
	// A forked weft always gets its own record, though it may carry the parent's.
	origin := Origin{ParentBranch: recordedParentBranch, ParentWorktree: recordedParentWorktree, Beam: opts.Beam}
	writeOrigin := true
	if weftAdopted {
		if existing, found, readErr := ReadOriginFor(l, slug); readErr != nil {
			_ = t.rollbackAdd(rec, pairAt, slug, warpBranch, weftBranch, target, weftBranchAlreadyExists, weftReused, warpTok, &warpPush)
			return AddResult{}, fmt.Errorf("read adopted origin record: %w", readErr)
		} else if found {
			origin = existing
			origin.Beam = existing.Beam || opts.Beam
			writeOrigin = origin.Beam != existing.Beam
		} else if opts.AdoptOnly {
			// The record is never invented for a pair that is re-made: it has no fork point to name a parent.
			writeOrigin = false
		}
	}
	if writeOrigin {
		if err := WriteOrigin(rec, pairAt, slug, origin); err != nil {
			_ = t.rollbackAdd(rec, pairAt, slug, warpBranch, weftBranch, target, weftBranchAlreadyExists, weftReused, warpTok, &warpPush)
			return AddResult{}, fmt.Errorf("record parent branch: %w", err)
		}
		// The commit's sha and committed returns are not read here:
		// CommitRecordsPaths records the KindCommitCreated entry itself, at its own success site.
		// The run-records root joins the commit's paths only when the fork tracked it:
		// git add on an untracked, absent root is a hard pathspec error, which a hub with no run records must not hit.
		commitPaths := []string{OriginRecordRel()}
		if runRecordsTracked {
			commitPaths = append(commitPaths, shedrun.RunsRootRel())
		}
		if _, _, err := CommitRecordsPaths(rec, weftPath, pairAt.AnchorRel, commitPaths, OriginRecordSubject(slug), opts.syncOptions()); err != nil {
			_ = t.rollbackAdd(rec, pairAt, slug, warpBranch, weftBranch, target, weftBranchAlreadyExists, weftReused, warpTok, &warpPush)
			return AddResult{}, fmt.Errorf("commit parent branch record: %w", err)
		}
	}

	// The pair is wired and recorded at its requested anchor: the move is finished.
	if anchorTo != "" {
		if err := RemoveMoveRecord(weftPath); err != nil {
			_ = t.rollbackAdd(rec, pairAt, slug, warpBranch, weftBranch, target, weftBranchAlreadyExists, weftReused, warpTok, &warpPush)
			return AddResult{}, err
		}
	}

	// (11) Replace an archived leftover of the weft branch on origin,
	// so the caller's push of the weft branch is not rejected.
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
			_ = t.rollbackAdd(rec, pairAt, slug, warpBranch, weftBranch, target, weftBranchAlreadyExists, weftReused, warpTok, &warpPush)
			return AddResult{}, fmt.Errorf("replace leftover weft branch %q on origin: %w", weftBranch, delErr)
		}
	}

	// The pair is complete, so the worktree seam hears of it last;
	// a seam failure is logged by the call and never fails the add.
	if pair, locateErr := lymcwd.ResolveWorktree(target); locateErr != nil {
		logger.Warn("fabricengine: locating the new worktree for the worktree seam failed (non-fatal)", "verb", "add", "slug", slug, "error", locateErr)
	} else {
		NotifyWorktreeMade(pair)
	}

	return AddResult{
		Slug:   slug,
		Branch: warpBranch,
		Path:   target,
	}, nil
}

// addWarpBranch records whether an add created or adopted its warp branch.
// A rollback deletes that branch only when the add created it.
type addWarpBranch struct {
	// tok proves the add created the branch; the zero value proves nothing.
	tok createdBranchToken
	// adopted is whether the branch existed before the add, in which case a rollback leaves it as it found it.
	adopted bool
}

// rollbackAdd performs best-effort paired cleanup when an add fails.
// It unwires junctions, removes worktrees and branches, and preserves pre-existing adopted weft branches.
// weftBranchAdopted is whether the local weft branch existed before the add.
// A branch the add created, forked or took from origin as a local tracking branch, is deleted.
// A pre-existing one survives, a fast-forward the add made to it included; it is never rewound.
// It never touches an origin branch: the add pushes nothing.
// weftReused is whether the add reused a records worktree that stood before it:
// that worktree and its branch are left in place, as an adopted branch is.
// warpTok is the token minted when the add created the warp worktree at target;
// it is the ownership proof the gate's warp-side removal requires.
// warp carries the proof that the add created the warp branch;
// that proof lets the gate delete the local branch, whether it was forked or adopted from origin.
// A warp branch that predates the add (warp.adopted) is never deleted.
// A refused or failed branch deletion is logged, not swallowed.
// Every caller discards this function's return, so the log is what makes a branch left behind visible in the trace.
// Rollback never restores a remote weft branch that the add's step 11 replaced:
// its content stays reachable from the archive tag, and recreating it would re-block the next retry.
// rec is the add's own recorder, passed to every gate-bound removal this function makes,
// so a rollback's destructions land in the same record as the add's creations, in execution order.
// The origin record needs no removal step of its own.
// On the created-branch path, the weft-worktree and weft-branch removal in step 1 takes the record with it.
// On the adopted-weft-branch path, the record's commit is deliberately left in place:
// reverting or resetting an adopted branch to undo one commit would destroy pre-existing history.
// Step 1 prevents that by deleting a weft branch only when the add created it.
func (t *Topology) rollbackAdd(rec *Mutations, l *lymcwd.Location, slug, warpBranch, weftBranch, target string, weftBranchAdopted, weftReused bool, warpTok createdToken, warp *addWarpBranch) error {
	var firstErr error

	// (1) Remove the weft worktree; delete the weft branch only when the add created it,
	// so a rollback never destroys pre-existing weft history.
	// A reused records worktree predates the add and stays.
	// remote is hardcoded false here:
	// the add pushes nothing, and an adopted branch's copy on origin is not the add's to delete.
	if !weftReused {
		if _, err := removeWeftWorktree(rec, l, slug, weftBranch, true, !weftBranchAdopted, false, "", t.leftover); err != nil {
			if firstErr == nil {
				firstErr = err
			}
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

	// The warp branch is deleted locally only when this Add created it: an adopted branch predates this Add and stays.
	if !warp.adopted {
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
			// The callers discard this function's return, so the refusal is logged here.
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
	}

	if _, err := gitexec.Run([]string{"worktree", "prune"}, l.WorktreePath()); err != nil {
		if firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}
