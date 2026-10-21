// originreconcilemoves.go holds the origin reconcile's moving half.
// It fast-forwards or replays the local branches a judging pass found behind origin.
// Each move waits on no lock: the prime lock and a records branch's write and push locks are try-acquired, and the live-pair seam is asked once per pair.
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

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// The locks a busy branch names in BranchOutcome.BusyLock.
const (
	busyPrimeLock       = "prime"
	busyPrimeParentLock = "prime parent"
	busyWeftWriteLock   = "weft write"
	busyWeftPushLock    = "weft push"
	busyBoardWriteLock  = "board write"
	busyBoardPushLock   = "board push"
)

// tryPrimeLock takes the hub's prime lock by one try-acquire, never waiting.
// held reports whether this call now holds it; a lock another holder has returns held false with no error.
// The caller calls release once the prime's branches are moved.
func tryPrimeLock(l *lyxcwd.Location) (release func(), held bool, err error) {
	path := shedrun.PrimeRunLock(l)
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
// On Bolt they are `board.lock` and the push lock in the board checkout, and on a weft they are the weft's write and push locks.
// A held lock returns its name in busy, with pushHeld set when it was the push lock, and nothing held.
func tryRecordsLocks(worktree string, bolt bool) (release func(), busy string, pushHeld bool, err error) {
	writePath := ""
	pushPath := ""
	writeName := busyWeftWriteLock
	pushName := busyWeftPushLock
	if bolt {
		writePath = filepath.Join(worktree, BoardWriteLockFile)
		pushPath = filepath.Join(worktree, gitrepo.PushLockFileName)
		writeName = busyBoardWriteLock
		pushName = busyBoardPushLock
	} else {
		lockDir, err := ensureWeftLockDirAt(worktree)
		if err != nil {
			return nil, "", false, err
		}
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
	return func() {
		_ = pushLock.Release()
		_ = writeLock.Release()
	}, "", false, nil
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

// pathsChangedBetween returns the paths whose content differs between two commits of the repository at dir, renames counted as a delete and an add.
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
func ffWorktree(rec *Mutations, worktree, originHead string) (state BranchState, dirty []string, err error) {
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
		return BranchDirty, dirty, nil
	}
	if err := gitrepo.New(worktree).MergeFFOnly(originHead); err != nil {
		return "", nil, err
	}
	rec.Append(KindRepoAdvanced, worktree, originHead)
	return BranchLevel, nil, nil
}

// reconcileRecordsBranch brings the records branch checked out at worktree up to originHead, under its write lock and then its push lock, each by one try-acquire.
// bolt names the board checkout, replayed by entry; any other records branch is a weft, replayed by commit.
// A held lock returns BranchBusy naming it, and nothing moves.
// A branch behind origin is fast-forwarded, and one both sides moved has its local commits replayed onto origin's with ended told to the replay.
// A replay that stops on a conflict returns BranchDiverged naming the conflicting paths and the local commits, and one an uncommitted change blocks returns BranchDirty;
// a branch the locks found already level or ahead is returned as that, unmoved.
// A moved branch returns BranchLevel, or BranchAhead when replayed commits sit on origin's head, with Moved set.
func reconcileRecordsBranch(rec *Mutations, l *lyxcwd.Location, worktree, originHead string, ended EndedSet, bolt bool) (BranchOutcome, error) {
	var outcome BranchOutcome
	release, busy, pushHeld, err := tryRecordsLocks(worktree, bolt)
	if err != nil {
		return outcome, err
	}
	if busy != "" {
		outcome.State = BranchBusy
		outcome.BusyLock = busy
		outcome.PushLockHeld = pushHeld
		return outcome, nil
	}
	defer release()

	repo := gitrepo.New(worktree)
	head, err := repo.CurrentSHA()
	if err != nil {
		return outcome, err
	}
	if head == originHead {
		outcome.State = BranchLevel
		return outcome, nil
	}
	behind, err := repo.IsAncestor(head, originHead)
	if err != nil {
		return outcome, err
	}
	if behind {
		state, dirty, err := ffWorktree(rec, worktree, originHead)
		if err != nil {
			return outcome, err
		}
		outcome.State = state
		outcome.Paths = dirty
		outcome.Moved = state == BranchLevel
		return outcome, nil
	}
	ahead, err := repo.IsAncestor(originHead, head)
	if err != nil {
		return outcome, err
	}
	if ahead {
		outcome.State = BranchAhead
		return outcome, nil
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
		outcome.State = BranchAhead
		if moved == originHead {
			outcome.State = BranchLevel
		}
		outcome.Moved = true
		return outcome, nil
	case errors.Is(err, ErrReplayDirty):
		outcome.State = BranchDirty
		outcome.Paths, err = dirtyTrackedPaths(worktree)
		return outcome, err
	case errors.Is(err, ErrReplayConflict), errors.As(err, &refusal):
		outcome.State = BranchDiverged
		outcome.Paths = replay.Conflicts
		outcome.Commits, err = repo.CommitsNotIn(head, originHead)
		return outcome, err
	}
	return outcome, err
}

// originMover moves the branches one reconcile judged, in the order its work is done, and reports each move on the outcome it was judged as.
type originMover struct {
	rec      *Mutations
	l        *lyxcwd.Location
	scope    ReconcileScope
	seams    OriginSeams
	ended    EndedSet
	targets  []originTarget
	outcomes []BranchOutcome
	live     map[string]bool
}

// moveAll moves every judged branch that is behind origin: Bolt first, then the prime's branches, the prime's records before its code, then each pair's.
// The prime's branches move only under the prime lock and a free parent lock.
// A run scope moves Bolt and its own pair, and judges the parent branch only.
func (m *originMover) moveAll(pairs []string) error {
	var bolt, prime, pair []int
	for i, target := range m.targets {
		if target.side == failedPushSideBolt {
			bolt = append(bolt, i)
		}
	}
	ofRepo := func(side string, pick func(originTarget) bool) []int {
		var found []int
		for i, target := range m.targets {
			if target.side == side && pick(target) {
				found = append(found, i)
			}
		}
		return found
	}
	isPrime := func(target originTarget) bool { return target.prime }
	prime = append(ofRepo(failedPushSideRecords, isPrime), ofRepo(failedPushSideCode, isPrime)...)
	for _, slug := range pairs {
		inPair := func(target originTarget) bool { return target.slug == slug && !target.prime }
		pair = append(pair, ofRepo(failedPushSideRecords, inPair)...)
		pair = append(pair, ofRepo(failedPushSideCode, inPair)...)
	}

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

// needsMove reports whether the judged branch at index i is one a moving reconcile moves.
func (m *originMover) needsMove(i int) bool {
	state := m.outcomes[i].State
	return state == BranchFastForward || state == BranchNeedsReplay
}

// movePrime moves the prime's branches under the prime lock, taking it only when one of them needs a move.
// A held prime lock, or a prime branch's parent lock a live owner holds, leaves each branch needing a move busy.
func (m *originMover) movePrime(indexes []int) error {
	pending := slices.ContainsFunc(indexes, m.needsMove)
	if !pending || m.scope.Run != "" {
		for _, i := range indexes {
			if err := m.move(i, ""); err != nil {
				return err
			}
		}
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

// move moves the judged branch at index i when it needs a move and nothing stops it, and records the result on its outcome.
// A busy lock name stops the move and marks the branch busy.
func (m *originMover) move(i int, busy string) error {
	outcome := &m.outcomes[i]
	target := m.targets[i]
	if !m.needsMove(i) {
		return nil
	}
	if m.scope.Run != "" && target.side != failedPushSideBolt && target.slug != m.scope.Run {
		return nil
	}

	if busy != "" {
		outcome.State = BranchBusy
		outcome.BusyLock = busy
		outcome.WayForward = outcome.stopWayForward()
		return nil
	}
	if target.slug != "" && target.slug != m.scope.Run {
		live, err := m.pairLive(target.slug)
		if err != nil {
			return err
		}
		if live {
			outcome.State = BranchLive
			outcome.WayForward = outcome.stopWayForward()
			return nil
		}
	}

	if target.side == failedPushSideCode {
		state, dirty, err := ffWorktree(m.rec, target.worktree, outcome.OriginHead)
		if err != nil {
			return err
		}
		outcome.State = state
		outcome.Paths = dirty
		outcome.Moved = state == BranchLevel
		outcome.WayForward = outcome.stopWayForward()
		return nil
	}

	location := m.l
	if target.slug != "" && target.slug != m.scope.Run {
		var err error
		if location, err = lyxcwd.ResolveWorktree(target.codeWorktree); err != nil {
			return err
		}
	}
	moved, err := reconcileRecordsBranch(m.rec, location, target.worktree, outcome.OriginHead, m.ended, target.side == failedPushSideBolt)
	if err != nil {
		return err
	}
	outcome.State = moved.State
	outcome.Paths = moved.Paths
	outcome.Commits = moved.Commits
	outcome.BusyLock = moved.BusyLock
	outcome.PushLockHeld = moved.PushLockHeld
	outcome.Moved = moved.Moved
	outcome.WayForward = outcome.stopWayForward()
	return nil
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
func recordedPairs(l *lyxcwd.Location, pairs []string) ([]string, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	primeName, err := PrimeName(l)
	if err != nil {
		return nil, err
	}
	prime, err := lyxcwd.ResolveWorktree(filepath.Join(l.HubPath, primeName))
	if err != nil {
		return nil, err
	}
	var recorded []string
	for _, slug := range pairs {
		_, found, err := readRunStatusRows(shedrun.StatusFile(prime, slug))
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
// The candidates are the pairs with a batten run record and the runs a local commit origin lacks touches, on Bolt or on a weft needing a replay.
// A run this machine's own landing has reached is dropped from the set, so no replay sets aside what this machine is landing.
// A hub with no Bolt branch to read, or one origin no longer has, has an empty set.
func readOriginEnded(l *lyxcwd.Location, targets []originTarget, outcomes []BranchOutcome, recorded []string) (EndedSet, error) {
	boltIndex := slices.IndexFunc(targets, func(target originTarget) bool { return target.side == failedPushSideBolt })
	if boltIndex < 0 || outcomes[boltIndex].OriginHead == "" {
		return nil, nil
	}
	boardDir := targets[boltIndex].worktree
	board := gitrepo.New(boardDir)

	candidates := map[string]bool{}
	created := map[string]string{}
	for _, slug := range recorded {
		candidates[slug] = true
	}
	for i, target := range targets {
		if outcomes[i].State != BranchNeedsReplay {
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
		if target.side == failedPushSideBolt {
			slugs, stamps, err := localBoardEntries(repo, head, ahead)
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

// addLocalCreatedStamps adds to created the stamp the local board holds for each of slugs that has none yet.
// A slug the local board does not hold gets none.
func addLocalCreatedStamps(board *gitrepo.Repo, slugs []string, created map[string]string) error {
	replay, err := InstalledBoardReplay()
	if err != nil {
		return err
	}
	head, err := board.CurrentSHA()
	if err != nil {
		return err
	}
	data, err := blobAt(board, head, boardFileName)
	if err != nil || len(data) == 0 {
		return err
	}
	entries, err := replay.Entries(data)
	if err != nil {
		return fmt.Errorf("%s at %s: %w", boardFileName, head, err)
	}
	for _, entry := range entries {
		if _, stamped := created[entry.Slug]; stamped || entry.Created.IsZero() || !slices.Contains(slugs, entry.Slug) {
			continue
		}
		created[entry.Slug] = entry.Created.UTC().Format(time.RFC3339)
	}
	return nil
}
