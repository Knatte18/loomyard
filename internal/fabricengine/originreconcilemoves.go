// originreconcilemoves.go holds the origin reconcile's moving half.
// It fast-forwards or replays the local branches a judging pass found behind origin.
// The order and each branch's verdict are fabric's core rules; the locks, the probes and the git are here.
// The prime lock and a records branch's write lock are try-acquired, and a held one reads busy at once.
// A records branch's push lock is try-acquired too, and retried for the bound the reconcile's options set.
// The live-pair seam is asked once per pair.
// It also reads the ended set, the runs origin has ended, that every replay is told.

package fabricengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/fabriccore"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// The locks a busy branch names in fabriclayout.BranchOutcome.BusyLock.
const (
	busyPrimeLock       = "prime"
	busyPrimeParentLock = "prime parent"
	busyWeftWriteLock   = "weft write"
	busyWeftPushLock    = "weft push"
	busyBoardWriteLock  = "board write"
	busyBoardPushLock   = "board push"
)

// pushLockPoll is the delay between two tries of a records branch's locks while only its push lock is held.
const pushLockPoll = time.Second

// tryPrimeLock takes the hub's prime lock by one try-acquire, never waiting.
// held reports whether this call holds it; a lock another holder has returns held false with no error.
// The caller calls release once the prime's branches are moved.
func tryPrimeLock(l *lymcwd.Location) (release func(), held bool, err error) {
	path := installedRunLocks.Load().PrimeRunLock(l)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, false, fmt.Errorf("fabricengine: create the prime lock directory: %w", err)
	}
	prime, acquired, err := lock.TryAcquireWriteLock(path)
	if err != nil {
		return nil, false, fmt.Errorf("fabricengine: acquire the prime lock: %w", err)
	}
	if !acquired {
		return nil, false, nil
	}
	return func() { _ = prime.Release() }, true, nil
}

// tryRecordsLocks takes a records branch's write lock and then its push lock, each by one try-acquire.
// On Bolt they are the board write lock and the board checkout's push lock.
// On a weft they are the weft's write and push locks.
// A held lock returns its name in busy, with pushHeld set when it was the push lock, and nothing held.
func tryRecordsLocks(worktree string, bolt bool) (release func(), busy string, pushHeld bool, err error) {
	writePath := ""
	pushPath := ""
	writeName := busyWeftWriteLock
	pushName := busyWeftPushLock
	// weftLockDir is the weft's lock directory, whose push lock gets a holder record; it stays empty on Bolt.
	weftLockDir := ""
	if bolt {
		writePath, err = BoltLockPath(worktree, fabriclayout.BoardWriteLockFile)
		if err != nil {
			return nil, "", false, err
		}
		pushPath, err = gitrepo.New(worktree).PushLockPath()
		if err != nil {
			return nil, "", false, err
		}
		writeName = busyBoardWriteLock
		pushName = busyBoardPushLock
	} else {
		lockDir, err := ensureWeftLockDirAt(worktree)
		if err != nil {
			return nil, "", false, err
		}
		weftLockDir = lockDir
		writePath = filepath.Join(lockDir, weftWriteLockFile)
		pushPath = filepath.Join(lockDir, weftPushLockFile)
	}

	writeLock, acquired, err := lock.TryAcquireWriteLock(writePath)
	if err != nil {
		return nil, "", false, fmt.Errorf("fabricengine: acquire the %s lock of %s: %w", writeName, worktree, err)
	}
	if !acquired {
		return nil, writeName, false, nil
	}
	pushLock, acquired, err := lock.TryAcquireWriteLock(pushPath)
	if err != nil {
		_ = writeLock.Release()
		return nil, "", false, fmt.Errorf("fabricengine: acquire the %s lock of %s: %w", pushName, worktree, err)
	}
	if !acquired {
		_ = writeLock.Release()
		return nil, pushName, true, nil
	}
	releasePush := pushLock.Release
	if weftLockDir != "" {
		releasePush = holdPushLock(pushLock, weftLockDir, time.Now())
	}
	return func() {
		_ = releasePush()
		_ = writeLock.Release()
	}, "", false, nil
}

// waitRecordsLocks takes a records branch's locks as tryRecordsLocks does.
// While the write lock is free and the push lock is held, it tries both again every pushLockPoll.
// It stops once opts' PushLockWait has passed in sleeps.
// So a push that ends within that bound leaves the branch movable.
// It holds no lock between two tries, so the push holding the lock can take the write lock meanwhile.
func waitRecordsLocks(worktree string, bolt bool, opts ReconcileOptions) (release func(), busy string, pushHeld bool, err error) {
	var waited time.Duration
	for {
		release, busy, pushHeld, err = tryRecordsLocks(worktree, bolt)
		if err != nil || !pushHeld || waited >= opts.PushLockWait {
			return release, busy, pushHeld, err
		}
		opts.sleepFor(pushLockPoll)
		waited += pushLockPoll
	}
}

// relativeSlashPaths returns paths, absolute under dir, as slash paths relative to dir, sorted.
func relativeSlashPaths(dir string, paths []string) ([]string, error) {
	relative := make([]string, 0, len(paths))
	for _, path := range paths {
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return nil, err
		}
		relative = append(relative, filepath.ToSlash(rel))
	}
	sort.Strings(relative)
	return relative, nil
}

// dirtyTrackedPaths returns the tracked uncommitted paths of the checkout at dir, relative to it with slash separators.
func dirtyTrackedPaths(dir string) ([]string, error) {
	absolute, err := trackedDirtyPaths(dir)
	if err != nil {
		return nil, err
	}
	return relativeSlashPaths(dir, absolute)
}

// pathsChangedBetween returns the paths of the repository at dir whose content differs between two commits.
// A rename counts as a delete and an add.
func pathsChangedBetween(dir, from, to string) ([]string, error) {
	out, err := gitexec.Run([]string{"diff", "--name-only", "--no-renames", "-z", from, to}, dir)
	if err != nil {
		return nil, fmt.Errorf("list the paths that differ between %s and %s in %s: %w", from, to, dir, err)
	}
	var paths []string
	for _, path := range strings.Split(out, "\x00") {
		if path != "" {
			paths = append(paths, path)
		}
	}
	return paths, nil
}

// ffWorktree fast-forwards the checkout at worktree to originHead, a descendant of its tip.
// A tracked change on a path the move touches returns BranchDirty naming those paths, with nothing moved.
// A moved checkout returns BranchLevel and records one repo_advanced entry.
func ffWorktree(rec *Mutations, worktree, originHead string) (state fabriclayout.BranchState, dirty []string, err error) {
	touched, err := pathsChangedBetween(worktree, "HEAD", originHead)
	if err != nil {
		return "", nil, err
	}
	changed, err := dirtyTrackedPaths(worktree)
	if err != nil {
		return "", nil, err
	}
	for _, path := range changed {
		if slices.Contains(touched, path) {
			dirty = append(dirty, path)
		}
	}
	if len(dirty) > 0 {
		return fabriclayout.BranchDirty, dirty, nil
	}
	if err := gitrepo.New(worktree).MergeFFOnly(originHead); err != nil {
		return "", nil, err
	}
	rec.Append(fabriclayout.KindRepoAdvanced, worktree, originHead)
	return fabriclayout.BranchLevel, nil, nil
}

// reconcileRecordsBranch brings the records branch checked out at worktree up to originHead.
// It takes its write lock and then its push lock through waitRecordsLocks, which waits on the push lock for opts' bound.
// bolt names the board checkout, replayed by entry; any other records branch is a weft, replayed by commit.
// A lock still held returns BranchBusy naming it, and nothing moves.
// A branch behind origin is fast-forwarded.
// A branch both sides moved has its local commits replayed onto origin's, with ended told to the replay.
// A replay that stops on a conflict returns BranchDiverged naming the conflicting paths and the local commits.
// A replay that an uncommitted change blocks returns BranchDirty.
// A branch the locks found already level or ahead is returned as that, unmoved.
// A moved branch returns BranchLevel, or BranchAhead when replayed commits sit on origin's head, with Moved set.
func reconcileRecordsBranch(rec *Mutations, l *lymcwd.Location, worktree, originHead string, ended EndedSet, bolt bool, opts ReconcileOptions) (fabriclayout.BranchOutcome, error) {
	var outcome fabriclayout.BranchOutcome
	var facts fabriccore.RecordsMoveFacts
	release, busy, pushHeld, err := waitRecordsLocks(worktree, bolt, opts)
	if err != nil {
		return outcome, err
	}
	if busy != "" {
		facts.BusyLock, facts.PushLockHeld = busy, pushHeld
		return fabriccore.RecordsMoveState(facts), nil
	}
	defer release()

	repo := gitrepo.New(worktree)
	head, err := repo.CurrentSHA()
	if err != nil {
		return outcome, err
	}
	facts.Level = head == originHead
	if !facts.Level {
		if facts.Behind, err = repo.IsAncestor(head, originHead); err != nil {
			return outcome, err
		}
	}
	if !facts.Level && !facts.Behind {
		if facts.Ahead, err = repo.IsAncestor(originHead, head); err != nil {
			return outcome, err
		}
	}
	switch verdict := fabriccore.RecordsMoveState(facts); verdict.State {
	case fabriclayout.BranchFastForward:
		state, dirty, err := ffWorktree(rec, worktree, originHead)
		if err != nil {
			return outcome, err
		}
		outcome.State = state
		outcome.Paths = dirty
		outcome.Moved = state == fabriclayout.BranchLevel
		return outcome, nil
	case fabriclayout.BranchNeedsReplay:
	default:
		return verdict, nil
	}

	var replay ReplayOutcome
	if bolt {
		replay, err = replayBolt(rec, l, worktree, originHead, ended)
	} else {
		replay, err = replayWeft(rec, l, worktree, originHead, ended)
	}
	var refusal *destructiveRefusal
	switch {
	case err == nil:
		moved, err := repo.CurrentSHA()
		if err != nil {
			return outcome, err
		}
		facts.Replayed, facts.Level = true, moved == originHead
	case errors.Is(err, ErrReplayDirty):
		facts.ReplayDirty = true
		if facts.Paths, err = dirtyTrackedPaths(worktree); err != nil {
			return outcome, err
		}
	case errors.Is(err, ErrReplayConflict), errors.As(err, &refusal):
		facts.ReplayConflict, facts.Paths = true, replay.Conflicts
		if facts.Commits, err = repo.CommitsNotIn(head, originHead); err != nil {
			return outcome, err
		}
	default:
		return outcome, err
	}
	return fabriccore.RecordsMoveState(facts), nil
}

// originMover moves the branches one reconcile judged, in the order its work is done.
// It reports each move on the outcome the branch was judged as.
type originMover struct {
	rec      *Mutations
	l        *lymcwd.Location
	scope    fabriclayout.ReconcileScope
	seams    OriginSeams
	opts     ReconcileOptions
	ended    EndedSet
	targets  []originTarget
	outcomes []fabriclayout.BranchOutcome
	live     map[string]bool
	// ownedByEnded holds the slugs of pairs the ended-entries step already judged, whose branches no move touches.
	ownedByEnded map[string]bool
	// owners maps the slug of each entry another machine owns to that owner; such an entry's records branch is never replayed.
	owners map[string]string
}

// moveAll moves every judged branch that is behind origin.
// It moves Bolt first, then the prime's branches, its records before its code, then each pair's.
// The prime's branches move only under the prime lock and a free parent lock.
// A run scope moves Bolt and its own pair, and judges the parent branch only.
func (m *originMover) moveAll(pairs []string) error {
	targets := make([]fabriccore.MoveTarget, len(m.targets))
	for i, target := range m.targets {
		targets[i] = fabriccore.MoveTarget{Repo: target.side, Slug: target.slug, Prime: target.prime}
	}
	bolt, prime, pair := fabriccore.MoveOrder(targets, pairs)

	for _, i := range bolt {
		if err := m.move(i, ""); err != nil {
			return err
		}
	}
	if err := m.movePrime(prime); err != nil {
		return err
	}
	for _, i := range pair {
		if err := m.move(i, ""); err != nil {
			return err
		}
	}
	return nil
}

// moveFacts returns what is known of the judged branch at index i before any probe, with busy the held lock's name.
func (m *originMover) moveFacts(i int, busy string) fabriccore.MoveFacts {
	target := m.targets[i]
	_, elsewhere := m.owners[target.slug]
	return fabriccore.MoveFacts{
		State:          m.outcomes[i].State,
		Run:            m.scope.Run,
		Repo:           target.side,
		Slug:           target.slug,
		OwnedByEnded:   m.ownedByEnded[target.slug],
		OwnedElsewhere: elsewhere,
		BusyLock:       busy,
	}
}

// movePrime moves the prime's branches under the prime lock, taking it only when one of them needs a move.
// A held prime lock, or a prime branch's parent lock a live owner holds, leaves each branch needing a move busy.
func (m *originMover) movePrime(indexes []int) error {
	pending := slices.ContainsFunc(indexes, func(i int) bool {
		return fabriccore.MoveVerdict(m.moveFacts(i, "")) != fabriccore.MoveNone
	})
	if !pending {
		return nil
	}

	busy := ""
	free := true
	if m.seams.PrimeParentLockFree != nil {
		var err error
		if free, err = m.seams.PrimeParentLockFree(); err != nil {
			return fmt.Errorf("fabricengine: read the prime branch's parent lock: %w", err)
		}
	}
	if !free {
		busy = busyPrimeParentLock
	} else {
		release, held, err := tryPrimeLock(m.l)
		if err != nil {
			return err
		}
		if held {
			defer release()
		} else {
			busy = busyPrimeLock
		}
	}
	for _, i := range indexes {
		if err := m.move(i, busy); err != nil {
			return err
		}
	}
	return nil
}

// move does with the judged branch at index i what the move verdict says, and records the result on the outcome.
// A busy lock name is the held lock that keeps the branch from moving.
// The live-pair seam, the move record and the anchor marker are read just before the move they guard.
func (m *originMover) move(i int, busy string) error {
	outcome := &m.outcomes[i]
	target := m.targets[i]
	facts := m.moveFacts(i, busy)
	step := fabriccore.MoveVerdict(facts)
	if step == fabriccore.MoveProbeLive {
		live, err := m.pairLive(target.slug)
		if err != nil {
			return err
		}
		facts.LiveProbed, facts.Live = true, live
		step = fabriccore.MoveVerdict(facts)
	}
	var moveWayForward, anchor string
	if step == fabriccore.MoveRecords && target.side == fabriclayout.RepoRecords {
		var err error
		moveWayForward, facts.MoveStanding, err = MoveInProgress(filepath.Base(target.codeWorktree), target.worktree)
		if err != nil {
			return err
		}
		if !facts.MoveStanding {
			if facts.AnchorStopped, anchor, err = m.weftAnchorStopped(target, outcome.OriginHead); err != nil {
				return err
			}
		}
		step = fabriccore.MoveVerdict(facts)
	}

	switch step {
	case fabriccore.MoveNone:
		return nil
	case fabriccore.MoveBusy:
		outcome.State = fabriclayout.BranchBusy
		outcome.BusyLock = busy
	case fabriccore.MoveLive:
		outcome.State = fabriclayout.BranchLive
	case fabriccore.MoveAnchorStopped:
		outcome.State = fabriclayout.BranchAnchorStopped
		if facts.MoveStanding {
			outcome.MoveWayForward = moveWayForward
		} else {
			outcome.Anchor = anchor
		}
	case fabriccore.MoveFastForward:
		headBefore, err := gitrepo.New(target.worktree).CurrentSHA()
		if err != nil {
			return err
		}
		state, dirty, err := ffWorktree(m.rec, target.worktree, outcome.OriginHead)
		if err != nil {
			return err
		}
		outcome.State = state
		outcome.Paths = dirty
		outcome.Moved = state == fabriclayout.BranchLevel
		outcome.WayForward = branchStopWayForward(*outcome)
		if outcome.Moved {
			outcome.Warning = fireTookInAt(target.worktree, headBefore, outcome.OriginHead)
		}
		return nil
	case fabriccore.MoveRecords:
		location := m.l
		if target.slug != "" && target.slug != m.scope.Run {
			var err error
			if location, err = lymcwd.ResolveWorktree(target.codeWorktree); err != nil {
				return err
			}
		}
		moved, err := reconcileRecordsBranch(m.rec, location, target.worktree, outcome.OriginHead, m.ended, target.side == fabriclayout.RepoBolt, m.opts)
		if err != nil {
			return err
		}
		outcome.State = moved.State
		outcome.Paths = moved.Paths
		outcome.Commits = moved.Commits
		outcome.BusyLock = moved.BusyLock
		outcome.PushLockHeld = moved.PushLockHeld
		outcome.Moved = moved.Moved
	}
	outcome.WayForward = branchStopWayForward(*outcome)
	return nil
}

// weftAnchorStopped reports whether moving the weft target to originHead is held back.
// An empty own marker stops it with no anchor.
// Otherwise it stops when taking originHead in would cross a change of the anchor marker.
// Anchor is the one the way forward sets.
// The anchor of a markerless tip falls back to that of the pair's, or the prime's, own worktree.
func (m *originMover) weftAnchorStopped(target originTarget, originHead string) (stopped bool, anchor string, err error) {
	if emptyMarkerStop(target.worktree) != nil {
		return true, "", nil
	}
	own, err := lymcwd.NamedWorktree(m.l.HubPath, filepath.Base(target.codeWorktree))
	if err != nil {
		return false, "", err
	}
	names, err := RepoWiredNames(own)
	if err != nil {
		return false, "", fmt.Errorf("fabricengine: load the wired names to compare anchor markers: %w", err)
	}
	anchor, stopped, err = weftAnchorStop(target.worktree, "HEAD", originHead, names, own.AnchorRel, false)
	return stopped, anchor, err
}

// pairLive reports whether the pair slug is live, asking the seam once per pair.
// A reconcile with no live-pair seam reads every pair as not live.
func (m *originMover) pairLive(slug string) (bool, error) {
	if live, asked := m.live[slug]; asked {
		return live, nil
	}
	live := false
	if m.seams.LivePair != nil {
		var err error
		if live, err = m.seams.LivePair(slug); err != nil {
			return false, fmt.Errorf("fabricengine: read whether pair %s is live: %w", slug, err)
		}
	}
	m.live[slug] = live
	return live, nil
}

// recordedPairs returns the slugs of pairs that have a batten run record at the prime.
func recordedPairs(l *lymcwd.Location, pairs []string) ([]string, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	prime, err := resolvePrime(l)
	if err != nil {
		return nil, err
	}
	var recorded []string
	for _, slug := range pairs {
		found, err := runRecorded(prime, slug)
		if err != nil {
			return nil, err
		}
		if found {
			recorded = append(recorded, slug)
		}
	}
	return recorded, nil
}

// readOriginEnded reads which runs origin has ended, from origin's Bolt tip and its history, before any replay.
// The candidates are runs, plus the runs that the local commits origin lacks touch.
// On Bolt those are read from the local run ledger; on a weft needing a replay, from the paths the commits change.
// A run this machine's own landing has reached is dropped from the set,
// so no replay sets aside what this machine is landing.
// A hub with no Bolt branch to read, or one origin no longer has, has an empty set.
func readOriginEnded(l *lymcwd.Location, targets []originTarget, outcomes []fabriclayout.BranchOutcome, runs []string) (EndedSet, error) {
	boltIndex := slices.IndexFunc(targets, func(target originTarget) bool { return target.side == fabriclayout.RepoBolt })
	if boltIndex < 0 || outcomes[boltIndex].OriginHead == "" {
		return nil, nil
	}
	boardDir := targets[boltIndex].worktree
	board := gitrepo.New(boardDir)

	candidates := map[string]bool{}
	created := map[string]string{}
	for _, slug := range runs {
		candidates[slug] = true
	}
	for i, target := range targets {
		if outcomes[i].State != fabriclayout.BranchNeedsReplay {
			continue
		}
		repo := gitrepo.New(target.worktree)
		head, err := repo.CurrentSHA()
		if err != nil {
			return nil, err
		}
		ahead, err := repo.CommitsNotIn(head, outcomes[i].OriginHead)
		if err != nil {
			return nil, err
		}
		if target.side == fabriclayout.RepoBolt {
			slugs, stamps, err := localLedgerRuns(repo, head, ahead)
			if err != nil {
				return nil, err
			}
			for _, slug := range slugs {
				candidates[slug] = true
			}
			for slug, stamp := range stamps {
				created[slug] = stamp
			}
			continue
		}
		for _, sha := range ahead {
			detail, err := repo.CommitDetail(sha)
			if err != nil {
				return nil, err
			}
			for _, path := range detail.Paths {
				if slug, found := weftRunSlug(l, path); found {
					candidates[slug] = true
				}
			}
		}
	}
	if len(candidates) == 0 {
		return nil, nil
	}

	slugs := make([]string, 0, len(candidates))
	for slug := range candidates {
		slugs = append(slugs, slug)
	}
	sort.Strings(slugs)
	if err := addLocalCreatedStamps(board, slugs, created); err != nil {
		return nil, err
	}
	ended, err := readEndedSet(boardDir, outcomes[boltIndex].OriginHead, slugs, created)
	if err != nil {
		return nil, err
	}
	return withoutFinalizeEntered(l, ended), nil
}

// addLocalCreatedStamps adds to created the stamp the local run ledger holds for each of slugs that has none yet.
// A slug the local ledger does not hold gets none.
func addLocalCreatedStamps(board *gitrepo.Repo, slugs []string, created map[string]string) error {
	ledger, _, err := runLedger()
	if err != nil {
		return err
	}
	head, err := board.CurrentSHA()
	if err != nil {
		return err
	}
	runs, err := ledgerRunsAt(board, ledger, head)
	if err != nil {
		return fmt.Errorf("%s at %s: %w", ledger.File(), head, err)
	}
	for _, run := range runs {
		if _, stamped := created[run.Slug]; stamped || run.Created.IsZero() || !slices.Contains(slugs, run.Slug) {
			continue
		}
		created[run.Slug] = run.Created.UTC().Format(time.RFC3339)
	}
	return nil
}
