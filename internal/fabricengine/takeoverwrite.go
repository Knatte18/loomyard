// takeoverwrite.go writes the takeover entry that moves a run to this machine, the writer beside takeover.go's reader.
// It fetches and archives the run's tips, moves its pair onto origin's, and pushes the entry on two records branches.

package fabricengine

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"time"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
)

// primePairSlug names the prime's pair in owner archive tags and in the teardown lock of its records branch.
const primePairSlug = "root"

// takeoverBeforePushHook is nil in production.
// When set, it runs before each push attempt of a takeover entry, with the attempt's number counted from one.
var takeoverBeforePushHook func(attempt int)

// TakeOverRequest names the run a takeover moves and how it waits.
type TakeOverRequest struct {
	// Slug is the run moved, and the slug of its pair.
	Slug string
	// Subject is the adopt subject written into the entry.
	Subject string
	// Machine is this machine's current name, the machine the entry names.
	Machine string
	// Pair is the run's pair, which is already present here.
	Pair *lymcwd.Location
	// Parent is the parent pair, nil when the parent is the prime.
	Parent *lymcwd.Location
	// At is the time the entry is recorded.
	At time.Time
	// Delays are the waits between a rejected push and its retry, and nil means gitrepo.PushBackoffDelays.
	Delays []time.Duration
	// LockWait is the wait for each teardown lock, and zero means StatusPushLockWait.
	LockWait time.Duration
}

// TakeOverResult reports what a takeover did.
type TakeOverResult struct {
	// Tags lists the owner archive tags it pushed.
	Tags []string
	// Retries counts the pushes it repeated after origin rejected one.
	Retries int
}

// takeoverStatusPatch declares the one key of a status file the takeover writes.
// A rewrite through internal/state keeps every other key of the file.
type takeoverStatusPatch struct {
	Takeover *shedengine.Takeover `json:"takeover"`
}

// takeoverBranch is a records branch that gets the takeover entry.
type takeoverBranch struct {
	// loc is the pair whose records branch this is.
	loc *lymcwd.Location
	// weft is the records worktree.
	weft string
	// runIDs lists the run directories the status file may sit in, the first that exists being the file.
	runIDs []string
	// base is the local tip before the entry commit.
	base string
}

// TakeOverRun moves the run req names to this machine and leaves the takeover entry on origin.
// It passes the process's pause gate before it takes the first teardown lock and stays admitted until it returns,
// so a pause never holds it while it holds a teardown lock or a push lock.
// It takes the moved slug's teardown lock and then the parent slug's, `root` for the prime, each exclusive.
// It holds both until it returns, so no push of either pair runs meanwhile.
// A lock still held when its wait runs out refuses with an error wrapping ErrPairPushRunning, before anything is fetched.
// It then fetches the code and records branches of the pair and of the parent.
// It archives the local tips origin lacks of both pairs under owner archive tags pushed to origin.
// It moves the pair's local branches onto origin's tips, and an anchor change there refuses unwrapped.
// It writes the entry into the run's status file on the pair's records branch and on the parent's.
// It commits each and pushes the pair's before the parent's, with no code side.
// A push origin rejects is retried after each of Delays:
// the entry commit is dropped, this machine's other commits origin lacks are replayed onto origin's new tip,
// and the entry is written again on top.
// A replay that conflicts is undone and returned naming the run,
// and an exhausted retry returns the last push's error;
// either way the branch ends at its local commits without the entry commit.
// It last moves the local branches to origin's tips and drops this machine's hand-over mark of the slug.
// A fetch or fast-forward error is returned as it is.
// The entry stays once pushed, and a re-run finds the run's record on origin naming this machine.
func TakeOverRun(prime *lymcwd.Location, req TakeOverRequest) (TakeOverResult, error) {
	hubPath := prime.HubPath
	parent, parentSlug := req.Parent, primePairSlug
	if parent == nil {
		parent = prime
	} else {
		parentSlug = parent.WorktreeName
	}
	lockWait := req.LockWait
	if lockWait == 0 {
		lockWait = StatusPushLockWait
	}

	leaveGate := pauserecord.Admit(pushLockGateOp)
	defer leaveGate()
	for _, slug := range []string{req.Slug, parentSlug} {
		if err := ensureTeardownLockDir(hubPath, slug); err != nil {
			return TakeOverResult{}, err
		}
		held, acquired, err := lock.AcquireWriteLockWithin(TeardownLockPath(hubPath, slug), lockWait)
		if err != nil {
			return TakeOverResult{}, fmt.Errorf("take the teardown lock of the pair %q for the takeover of run %q: %w", slug, req.Slug, err)
		}
		if !acquired {
			return TakeOverResult{}, fmt.Errorf("a push or removal of the pair %q still holds its teardown lock, so the takeover of run %q is not started: %w", slug, req.Slug, ErrPairPushRunning)
		}
		defer func() { _ = held.Release() }()
	}

	rec := NewMutations(hubPath)
	var result TakeOverResult

	for _, dir := range []string{req.Pair.WorktreePath(), RecordsWorktree(req.Pair), parent.WorktreePath(), RecordsWorktree(parent)} {
		if err := gitrepo.New(dir).Fetch(); err != nil {
			return result, fmt.Errorf("fetch %s for the takeover of run %q: %w", dir, req.Slug, err)
		}
	}

	for _, pair := range []OwnerArchivePair{{Slug: req.Slug, Pair: req.Pair}, {Slug: parentSlug, Pair: parent}} {
		tags, err := archiveOwnerSides(rec, pair)
		result.Tags = append(result.Tags, tags...)
		if err != nil {
			return result, fmt.Errorf("%w: pair %s: %w", ErrArchiveFailed, pair.Slug, err)
		}
	}

	if _, err := DropLocal(rec, prime, req.Slug); err != nil {
		return result, err
	}

	entry := shedengine.Takeover{Machine: req.Machine, Subject: req.Subject, At: req.At.UTC().Format(time.RFC3339)}
	branches := []*takeoverBranch{
		{loc: req.Pair, weft: RecordsWorktree(req.Pair), runIDs: []string{req.Slug, shedrun.SelfRunID}},
		{loc: parent, weft: RecordsWorktree(parent), runIDs: []string{req.Slug}},
	}
	for _, branch := range branches {
		if err := branch.land(rec, req, entry, &result.Retries); err != nil {
			return result, err
		}
	}

	for _, dir := range []string{req.Pair.WorktreePath(), RecordsWorktree(req.Pair), RecordsWorktree(parent)} {
		if err := fastForwardToUpstream(dir); err != nil {
			return result, err
		}
	}
	return result, DropHandOverMark(hubPath, req.Slug)
}

// land writes the takeover entry on the branch, commits it and pushes it under the records push lock.
// The caller has passed the process's pause gate, so a pause never holds it while it holds the lock.
// retries counts each push it repeated.
func (b *takeoverBranch) land(rec *Mutations, req TakeOverRequest, entry shedengine.Takeover, retries *int) error {
	delays := req.Delays
	if delays == nil {
		delays = gitrepo.PushBackoffDelays()
	}
	lockDir, err := ensureWeftLockDirAt(b.weft)
	if err != nil {
		return err
	}
	held, acquired, err := lock.AcquireWriteLockWithin(filepath.Join(lockDir, weftPushLockFile), StatusPushLockWait)
	if err != nil {
		return fmt.Errorf("take the push lock of %s for the takeover of run %q: %w", b.weft, req.Slug, err)
	}
	if !acquired {
		return fmt.Errorf("the push lock of %s stayed held, so the takeover entry of run %q is not written: %w", b.weft, req.Slug, ErrPushLockBusy)
	}
	defer func() { _ = held.Release() }()

	repo := gitrepo.New(b.weft)
	if err := b.commitEntry(rec, req.Slug, entry); err != nil {
		return err
	}
	for attempt := 1; ; attempt++ {
		if takeoverBeforePushHook != nil {
			takeoverBeforePushHook(attempt)
		}
		pushErr := repo.PushRebaseFree()
		if pushErr == nil {
			return nil
		}
		if !errors.Is(pushErr, gitrepo.ErrPushRejected) {
			return errors.Join(fmt.Errorf("push the takeover entry of run %q from %s: %w", req.Slug, b.weft, pushErr), b.dropEntry(rec))
		}
		if attempt > len(delays) {
			return errors.Join(fmt.Errorf("push the takeover entry of run %q from %s: origin kept moving through %d attempts: %w", req.Slug, b.weft, attempt, pushErr), b.dropEntry(rec))
		}
		time.Sleep(delays[attempt-1])
		*retries++
		if err := b.rewriteOverOrigin(rec, req.Slug, entry); err != nil {
			return err
		}
	}
}

// commitEntry writes the entry into the run's status file and commits it, remembering the tip it was committed onto.
func (b *takeoverBranch) commitEntry(rec *Mutations, slug string, entry shedengine.Takeover) error {
	repo := gitrepo.New(b.weft)
	base, err := repo.CurrentSHA()
	if err != nil {
		return err
	}
	b.base = base
	statusPath, rel, lockPath, err := b.statusFile(slug)
	if err != nil {
		return err
	}
	if err := state.WriteJSON(statusPath, lockPath, takeoverStatusPatch{Takeover: &entry}); err != nil {
		return fmt.Errorf("write the takeover entry of run %q into %s: %w", slug, statusPath, err)
	}
	sha, committed, err := repo.StageAndCommit(fmt.Sprintf("takeover of %s by %s", slug, entry.Machine), []string{rel})
	if err != nil {
		return err
	}
	if committed {
		rec.Append(KindCommitCreated, b.weft, sha)
	}
	return nil
}

// statusFile returns the run's status file in the records worktree, its path relative to the worktree and its lock.
// The file sits under the anchor the branch records.
func (b *takeoverBranch) statusFile(slug string) (statusPath, rel, lockPath string, err error) {
	names, err := RepoWiredNames(b.loc)
	if err != nil {
		return "", "", "", fmt.Errorf("load the wired names to find the status file of run %q: %w", slug, err)
	}
	anchor, err := treeAnchor(b.weft, "HEAD", names, b.loc.AnchorRel)
	if err != nil {
		return "", "", "", err
	}
	for _, runID := range b.runIDs {
		_, statusRel, err := shedrun.TreeRunFileRels(runID)
		if err != nil {
			return "", "", "", err
		}
		rel := path.Join(filepath.ToSlash(anchor), statusRel)
		statusPath := filepath.Join(b.weft, filepath.FromSlash(rel))
		if pathPresent(statusPath) {
			return statusPath, rel, shedrun.StatusLock(b.loc, runID), nil
		}
	}
	return "", "", "", fmt.Errorf("the records branch at %s holds no status record of run %q, so no takeover entry is written there", b.weft, slug)
}

// rewriteOverOrigin fetches the branch and drops the entry commit.
// It replays this machine's other commits origin lacks onto origin's new tip and commits the entry again on top.
// A replay that conflicts is undone, so the branch ends at the commits it held before the entry.
func (b *takeoverBranch) rewriteOverOrigin(rec *Mutations, slug string, entry shedengine.Takeover) error {
	repo := gitrepo.New(b.weft)
	if err := repo.Fetch(); err != nil {
		return errors.Join(fmt.Errorf("fetch %s to retry the takeover entry of run %q: %w", b.weft, slug, err), b.dropEntry(rec))
	}
	upstream, hasUpstream, err := upstreamSHAAt(b.weft)
	if err != nil {
		return errors.Join(fmt.Errorf("read the upstream of %s to retry the takeover entry of run %q: %w", b.weft, slug, err), b.dropEntry(rec))
	}
	if !hasUpstream {
		return errors.Join(fmt.Errorf("%s has no upstream to retry the takeover entry of run %q over", b.weft, slug), b.dropEntry(rec))
	}
	ahead, err := repo.CommitsNotIn(b.base, upstream)
	if err != nil {
		return errors.Join(err, b.dropEntry(rec))
	}
	slices.Reverse(ahead)
	touched, err := diffNames(b.weft, "HEAD", upstream, nil)
	if err != nil {
		return errors.Join(err, b.dropEntry(rec))
	}
	req := replayRequest(b.weft, ownedWarpCheckout(b.weft), touched)
	if err := dropTakeoverEntryAndReplayOnto(rec, req, repo, b.base, upstream, ahead); err != nil {
		return fmt.Errorf("replay this machine's other commits of %s onto origin's tip for the takeover of run %q: %w", b.weft, slug, err)
	}
	return b.commitEntry(rec, slug, entry)
}

// dropEntry moves the branch back to the tip it held before the entry commit.
func (b *takeoverBranch) dropEntry(rec *Mutations) error {
	req := replayRequest(b.weft, ownedWarpCheckout(b.weft), nil)
	return dropTakeoverEntryAndReplayOnto(rec, req, gitrepo.New(b.weft), b.base, b.base, nil)
}

// fastForwardToUpstream moves the branch checked out at dir to its upstream tip, when it has an upstream.
func fastForwardToUpstream(dir string) error {
	upstream, hasUpstream, err := upstreamSHAAt(dir)
	if err != nil || !hasUpstream {
		return err
	}
	return gitrepo.New(dir).MergeFFOnly(upstream)
}
