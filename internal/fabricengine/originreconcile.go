// originreconcile.go holds the origin reconcile's judging half.
// It declares the types a call returns and the two network reads that bring this machine's view of origin up to date.
// It judges each local branch against origin's head and classifies an entry from its branches' states.

package fabricengine

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// originRemote is the remote every reconcile call reads.
const originRemote = "origin"

// defaultAttemptDeadline bounds one remote call of a reconcile whose options name no deadline.
const defaultAttemptDeadline = 20 * time.Second

// cleanupRerun is the re-entry a printed stop of a dirty, gone or busy branch appends to its way forward.
const cleanupRerun = "then re-run lyx cleanup --apply from the prime"

// ReconcileScope names what one reconcile judges.
// An empty Run is the hub scope: Bolt, the prime's code and records branches, and those of every local pair.
// A slug is the run scope: Bolt, the run's parent branch on both sides, read from the pair's origin record, and the run's own pair.
// JudgeOnly judges and fetches objects but moves no ref, for a dry run.
type ReconcileScope struct {
	Run       string
	JudgeOnly bool
}

// ReconcileOptions bounds a reconcile's remote calls, every field injectable.
// The zero value makes one attempt under a 20-second deadline.
// RetryBound and RetryInterval together name how many times the pair of ls-remote calls is retried after a transport failure: one retry per whole interval in the bound.
// AttemptDeadline bounds each remote call, and Sleep waits out one interval; nil sleeps for real.
type ReconcileOptions struct {
	RetryBound      time.Duration
	RetryInterval   time.Duration
	AttemptDeadline time.Duration
	Sleep           func(time.Duration)
}

// attempts returns how many times the ls-remote pair runs at most.
func (o ReconcileOptions) attempts() int {
	if o.RetryInterval <= 0 || o.RetryBound <= 0 {
		return 1
	}
	return 1 + int(o.RetryBound/o.RetryInterval)
}

// deadline returns the bound of one remote call.
func (o ReconcileOptions) deadline() time.Duration {
	if o.AttemptDeadline <= 0 {
		return defaultAttemptDeadline
	}
	return o.AttemptDeadline
}

// sleep waits out one retry interval.
func (o ReconcileOptions) sleep() {
	if o.Sleep != nil {
		o.Sleep(o.RetryInterval)
		return
	}
	time.Sleep(o.RetryInterval)
}

// EndedTeardown asks the teardown seam to remove the local pair of an entry origin has ended.
// Force marks the case where the pair's uncommitted work is snapshotted and removed instead of refusing the removal.
type EndedTeardown struct {
	Slug  string
	Force bool
}

// TeardownReport is what the teardown seam reports about one removal: the tags it kept work under, the reasons it kept anything, whether it was busy and the error it stopped on.
// It embeds MutationRecord, filled from the removal's own record.
type TeardownReport struct {
	MutationRecord
	Tags  []string
	Kept  []string
	Busy  bool
	Error error
}

// OriginSeams carries what a reconcile reaches only by injection, since fabricengine knows neither a run's driver nor a parent lock nor pair teardown.
// LivePair reports whether slug's driver strand, run lock, loop lock or hub pause is live.
// PrimeParentLockFree reads once whether the prime branch's parent lock record is free.
// Teardown removes the local pair of an ended entry.
type OriginSeams struct {
	LivePair            func(slug string) (bool, error)
	PrimeParentLockFree func() (bool, error)
	Teardown            func(EndedTeardown) (TeardownReport, error)
}

// BranchState is the verdict on one local branch against origin's head for it.
type BranchState string

// The verdicts, the first group from judging alone and the rest from the moves that follow it.
const (
	// BranchLevel means the branch and origin's head are the same commit.
	BranchLevel BranchState = "level"
	// BranchFastForward means the local tip is an ancestor of origin's head and nothing local is lost by moving to it.
	BranchFastForward BranchState = "fast_forward"
	// BranchAhead means origin's head is an ancestor of the local tip.
	BranchAhead BranchState = "ahead"
	// BranchRewound means origin reset the branch back past commits this machine holds.
	BranchRewound BranchState = "rewound"
	// BranchRewritten means origin replaced the branch's history with commits this machine never held.
	BranchRewritten BranchState = "rewritten"
	// BranchDiverged means a code branch both sides moved, or a records branch whose replay stopped on a conflict.
	BranchDiverged BranchState = "diverged"
	// BranchNeedsReplay means a records branch both sides moved, which a moving reconcile replays.
	BranchNeedsReplay BranchState = "needs_replay"
	// BranchGone means origin no longer has the branch.
	BranchGone BranchState = "gone"
	// BranchDirty means an uncommitted change blocks the move.
	BranchDirty BranchState = "dirty"
	// BranchBusy means a lock on the branch is held.
	BranchBusy BranchState = "busy"
	// BranchLive means the branch belongs to a pair that is live, which a reconcile never moves.
	BranchLive BranchState = "live"
)

// stops reports whether a branch in this state stops its entry or the scope.
func (s BranchState) stops() bool {
	switch s {
	case BranchRewound, BranchRewritten, BranchDiverged, BranchGone, BranchDirty, BranchBusy, BranchLive:
		return true
	}
	return false
}

// BranchOutcome is the verdict on one judged branch, with what a reader of a stop needs.
// Repo is the repository side: "code", "records" or "bolt".
// Worktree is where the branch is checked out, and CodeWorktree is the pair's code worktree, where every fabric verb of a records branch runs.
// Paths are the conflicting or dirty paths, and Commits those a rewind dropped or a replay stopped on.
// BusyLock names the held lock of a busy branch, and PushLockHeld is set when it was the push lock, which a parked push holds.
// WayForward is the one line every reader of a stop takes, the repair step alone, with no re-entry verb.
// Moved is set by a moving reconcile once it brought the branch up to origin's head, State then reading level, or ahead when replayed commits sit on it.
type BranchOutcome struct {
	Branch       string
	Repo         string
	Slug         string
	Prime        bool
	Worktree     string
	CodeWorktree string
	State        BranchState
	OriginHead   string
	Paths        []string
	Commits      []string
	BusyLock     string
	PushLockHeld bool
	WayForward   string
	Moved        bool
}

// EntryState is the verdict on one entry, the run a pair carries.
type EntryState string

// The verdicts of an entry.
const (
	// EntryActive means nothing about the entry stops it.
	EntryActive EntryState = "active"
	// EntryEnded means origin's board shows the entry done or removed.
	EntryEnded EntryState = "ended"
	// EntryStopped means a branch of the entry or a records stop keeps it from carrying on.
	EntryStopped EntryState = "stopped"
)

// EntryOutcome is the verdict on one judged entry.
// Busy marks an ended entry whose driver or branch is busy, so its pair is not yet removed, and WayForward then names what lets the next run remove it.
type EntryOutcome struct {
	Slug       string
	State      EntryState
	Busy       bool
	WayForward string
}

// OriginReport is what one reconcile did: the verdict on every judged branch and entry, and whether origin could be reached at all.
// Unreachable means a transport failure outlasted the retry bound, so nothing was judged against origin or moved.
// It embeds MutationRecord, filled by the moves.
type OriginReport struct {
	MutationRecord
	Unreachable bool
	Branches    []BranchOutcome
	Entries     []EntryOutcome
}

// Summary renders the report in a few lines for an operator: what moved, what stopped with its way forward, which entries ended and whether origin was unreachable.
// A stop of a dirty, gone or busy-on-a-write-lock branch is followed by the re-entry that retries it.
// An empty report renders as the empty string.
func (r OriginReport) Summary() string {
	var lines []string
	if r.Unreachable {
		lines = append(lines, "unreachable: origin could not be reached, so nothing was judged against it or moved")
	}

	var moved []string
	for _, branch := range r.Branches {
		if branch.Moved {
			moved = append(moved, branch.Branch)
		}
	}
	if len(moved) > 0 {
		lines = append(lines, "moved: "+strings.Join(moved, ", "))
	}

	for _, branch := range r.Branches {
		if !branch.State.stops() {
			continue
		}
		line := fmt.Sprintf("stopped: %s (%s): %s", branch.Branch, branch.State, branch.WayForward)
		if branch.rerunsCleanup() {
			line += "; " + cleanupRerun
		}
		lines = append(lines, line)
	}

	var ended []string
	for _, entry := range r.Entries {
		switch {
		case entry.State == EntryEnded && entry.Busy:
			lines = append(lines, fmt.Sprintf("ended, not yet removed: %s: %s", entry.Slug, entry.WayForward))
		case entry.State == EntryEnded:
			ended = append(ended, entry.Slug)
		}
	}
	if len(ended) > 0 {
		lines = append(lines, "ended: "+strings.Join(ended, ", "))
	}
	return strings.Join(lines, "\n")
}

// rerunsCleanup reports whether the way forward of this stop is followed by a re-run of `lyx cleanup --apply`.
func (b BranchOutcome) rerunsCleanup() bool {
	switch b.State {
	case BranchDirty, BranchGone:
		return true
	case BranchBusy:
		return !b.PushLockHeld
	}
	return false
}

// stopWayForward returns the repair step for a stopped branch, built through the failed-push helpers where one applies.
// A state that does not stop returns the empty string.
func (b BranchOutcome) stopWayForward() string {
	record := FailedPush{Repo: b.Repo, RepoPath: b.Worktree, CodeWorktree: b.CodeWorktree, Branch: b.Branch, Prime: b.Prime}
	switch b.State {
	case BranchRewound:
		return rewoundWayForward(b.Worktree, b.Branch, b.OriginHead, b.Commits)
	case BranchRewritten:
		return fmt.Sprintf("`lyx fabric pull` in %s", b.Worktree)
	case BranchDiverged:
		if b.Repo == failedPushSideCode {
			return codeDivergedWayForward(b.Worktree, b.Branch, b.Prime)
		}
		return recordsConflictWayForward(b.Worktree, b.Branch, b.Paths, b.Commits, pushStep(record))
	case BranchGone:
		if b.Slug != "" {
			record.Class = PushFailureGone
			return FailedPushWayForward(record)
		}
		return fmt.Sprintf("origin no longer has branch %s; run %s to publish it again", b.Branch, pushStep(record))
	case BranchDirty:
		return b.dirtyWayForward()
	case BranchBusy:
		if b.PushLockHeld && b.Slug != "" {
			return fmt.Sprintf("`lyx resume %s`", b.Slug)
		}
		return fmt.Sprintf("wait for the holder of the %s lock to end", b.BusyLock)
	case BranchLive:
		return "no step now: the run's own Merge-In judges and moves its pair, and `lyx cleanup --apply` re-judges it once the run has ended"
	}
	return ""
}

// dirtyWayForward names where each uncommitted path of a dirty branch is committed or restored.
// A records path under the record pathspec is committed by `lyx fabric commit` in the pair's code worktree, since every fabric verb is refused in the records sibling.
// Any path, one outside that pathspec included, is committed or restored by git in the worktree the branch is checked out in.
func (b BranchOutcome) dirtyWayForward() string {
	paths := joinOrNone(b.Paths)
	git := fmt.Sprintf("`git commit` or `git checkout -- <path>` in %s", b.Worktree)
	if b.Repo == failedPushSideRecords {
		return fmt.Sprintf("commit the uncommitted paths (%s) with `lyx fabric commit` in %s, or any of them with %s", paths, b.CodeWorktree, git)
	}
	return fmt.Sprintf("commit or restore the uncommitted paths (%s) with %s", paths, git)
}

// entryFacts are the facts classifyEntry judges an entry from.
// Ended is whether origin's board shows the entry done or removed.
// Branches are the verdicts on the entry's judged branches.
// D3Stop is whether the records replay stopped on the prime's records branch or Bolt for this entry's sake.
type entryFacts struct {
	Ended    bool
	Branches []BranchState
	D3Stop   bool
}

// classifyEntry is the one switch that turns an entry's facts into its verdict.
// An ended entry is ended whatever its branches say; otherwise a D3 stop, or a branch that stops, makes it stopped; every other entry is active.
func classifyEntry(f entryFacts) EntryState {
	if f.Ended {
		return EntryEnded
	}
	if f.D3Stop || slices.ContainsFunc(f.Branches, BranchState.stops) {
		return EntryStopped
	}
	return EntryActive
}

// originCallHook sees the arguments of every remote call a reconcile makes, just before it runs.
// It is nil in production; a test sets it through SetOriginCallHookForTest.
var originCallHook func(args []string)

// callOriginHook hands the arguments of one remote call to the hook, if any.
func callOriginHook(args []string) {
	if originCallHook != nil {
		originCallHook(args)
	}
}

// lsRemotePair runs `git ls-remote origin` for the code repository and the records repository in parallel, each under the attempt deadline, and returns each one's branch heads by branch name.
// While either call fails with a transport failure and attempts remain, the pair is retried together after one interval.
// A failure that is not a transport failure is never retried; it is returned as soon as it is seen, naming the repository and git's error line.
func lsRemotePair(warpDir, recordsDir string, opts ReconcileOptions) (warp, records map[string]string, err error) {
	for attempt := 1; ; attempt++ {
		warp, records, err = lsRemoteOnce(warpDir, recordsDir, opts.deadline())
		if err == nil || !gitexec.IsTransportFailure(err) || attempt >= opts.attempts() {
			return warp, records, err
		}
		opts.sleep()
	}
}

// lsRemoteOnce runs the two ls-remote calls once, in parallel.
// A non-transport failure wins over a transport one, so a caller retrying on a transport failure never retries a refusal.
func lsRemoteOnce(warpDir, recordsDir string, deadline time.Duration) (warp, records map[string]string, err error) {
	var warpErr, recordsErr error
	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer wait.Done()
		warp, warpErr = lsRemoteHeads(failedPushSideCode, warpDir, deadline)
	}()
	go func() {
		defer wait.Done()
		records, recordsErr = lsRemoteHeads(failedPushSideRecords, recordsDir, deadline)
	}()
	wait.Wait()

	switch {
	case warpErr != nil && !gitexec.IsTransportFailure(warpErr):
		return nil, nil, warpErr
	case recordsErr != nil && !gitexec.IsTransportFailure(recordsErr):
		return nil, nil, recordsErr
	case warpErr != nil:
		return nil, nil, warpErr
	case recordsErr != nil:
		return nil, nil, recordsErr
	}
	return warp, records, nil
}

// lsRemoteHeads returns origin's branch heads of the repository at dir by branch name.
func lsRemoteHeads(side, dir string, deadline time.Duration) (map[string]string, error) {
	args := []string{"ls-remote", "--heads", originRemote}
	callOriginHook(args)
	out, err := gitexec.RunWithin(args, dir, deadline)
	if err != nil {
		return nil, fmt.Errorf("read origin's branches of the %s repository at %s: %w", side, dir, err)
	}
	heads := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		sha, ref, found := strings.Cut(strings.TrimSpace(line), "\t")
		name, isBranch := strings.CutPrefix(ref, "refs/heads/")
		if found && isBranch {
			heads[name] = sha
		}
	}
	return heads, nil
}

// fetchMovedHeads fetches, in one fetch, the heads of heads, which map a branch name to origin's head for it, and which a caller limits to those whose head differs from the local tracking ref.
// It updates each branch's tracking ref, forcing it since origin may have rewound or rewritten the branch.
// With judgeOnly it fetches the objects by SHA into no ref and writes no FETCH_HEAD, so a dry run moves nothing.
// No head makes no call.
func fetchMovedHeads(dir string, heads map[string]string, judgeOnly bool) error {
	if len(heads) == 0 {
		return nil
	}
	branches := make([]string, 0, len(heads))
	for branch := range heads {
		branches = append(branches, branch)
	}
	sort.Strings(branches)

	args := []string{"fetch"}
	if judgeOnly {
		args = append(args, "--refmap=", "--no-write-fetch-head", originRemote)
		for _, branch := range branches {
			args = append(args, heads[branch])
		}
	} else {
		args = append(args, originRemote)
		for _, branch := range branches {
			args = append(args, fmt.Sprintf("+refs/heads/%s:%s", branch, trackingRefOf(branch)))
		}
	}
	callOriginHook(args)
	if _, err := gitexec.Run(args, dir); err != nil {
		return fmt.Errorf("fetch the moved branches %s from origin into %s: %w", strings.Join(branches, " "), dir, err)
	}
	return nil
}

// trackingRefOf returns the remote-tracking ref of a branch on origin.
func trackingRefOf(branch string) string {
	return "refs/remotes/" + originRemote + "/" + branch
}

// judgeBranch judges the local branch of repoDir against originHead, the head origin holds for it, which is empty when origin has no such branch.
// A code branch is read through the reflog of its tracking ref.
// An origin that reset it past commits this machine holds reads rewound, taking precedence over ahead, and one that replaced its history with commits this machine never held reads rewritten.
// A records branch neither side contains reads needs_replay, which a moving reconcile replaces with the result of its replay.
func judgeBranch(repoDir, branch, originHead string, records bool) (BranchOutcome, error) {
	outcome := BranchOutcome{Branch: branch, OriginHead: originHead}
	if originHead == "" {
		outcome.State = BranchGone
		return outcome, nil
	}

	repo := gitrepo.New(repoDir)
	localTip, err := repo.RefSHA("refs/heads/" + branch)
	if err != nil {
		return BranchOutcome{}, fmt.Errorf("judge branch %s in %s: %w", branch, repoDir, err)
	}
	if localTip == originHead {
		outcome.State = BranchLevel
		return outcome, nil
	}

	kind := rewriteNone
	if !records {
		earlier, err := trackingReflog(repoDir, trackingRefOf(branch))
		if err != nil {
			return BranchOutcome{}, fmt.Errorf("judge branch %s in %s: %w", branch, repoDir, err)
		}
		var dropped []string
		kind, dropped, err = classifyRewrite(repo, localTip, originHead, earlier)
		if err != nil {
			return BranchOutcome{}, fmt.Errorf("judge branch %s in %s: %w", branch, repoDir, err)
		}
		if kind == rewriteRewound {
			outcome.State = BranchRewound
			outcome.Commits = dropped
			return outcome, nil
		}
	}

	behind, err := repo.IsAncestor(localTip, originHead)
	if err != nil {
		return BranchOutcome{}, fmt.Errorf("judge branch %s in %s: %w", branch, repoDir, err)
	}
	if behind {
		outcome.State = BranchFastForward
		return outcome, nil
	}
	ahead, err := repo.IsAncestor(originHead, localTip)
	if err != nil {
		return BranchOutcome{}, fmt.Errorf("judge branch %s in %s: %w", branch, repoDir, err)
	}
	switch {
	case ahead:
		outcome.State = BranchAhead
	case records:
		outcome.State = BranchNeedsReplay
	case kind == rewriteRewritten:
		outcome.State = BranchRewritten
	default:
		outcome.State = BranchDiverged
	}
	return outcome, nil
}

// originTarget is one local branch a reconcile judges, with where it lives.
type originTarget struct {
	side         string
	branch       string
	repoDir      string
	worktree     string
	codeWorktree string
	slug         string
	prime        bool
	tracking     string
}

// originTargets lists the branches of the scope that have an origin tracking ref, since a branch never pushed has no origin head to judge against.
// It also returns the slug of every pair the scope covers, for classifying their entries, and the repository directories the network calls run in.
func originTargets(l *lyxcwd.Location, scope ReconcileScope) (targets []originTarget, pairs []string, warpDir, recordsDir string, err error) {
	warpEntries, err := List(l.AnchorPath())
	if err != nil {
		return nil, nil, "", "", err
	}
	recordsDir, err = RecordsRepoRoot(l)
	if err != nil {
		return nil, nil, "", "", err
	}
	recordsEntries, err := List(recordsDir)
	if err != nil {
		return nil, nil, "", "", err
	}

	wanted := func(entry WorktreeEntry) bool { return true }
	if scope.Run != "" {
		origin, found, err := ReadOriginFor(l, scope.Run)
		if err != nil {
			return nil, nil, "", "", err
		}
		own := WorktreePath(l, scope.Run)
		if !slices.ContainsFunc(warpEntries, func(entry WorktreeEntry) bool { return samePath(entry.Path, own) }) {
			return nil, nil, "", "", fmt.Errorf("fabricengine: origin reconcile of run %s: no pair worktree %s", scope.Run, own)
		}
		wanted = func(entry WorktreeEntry) bool {
			return samePath(entry.Path, own) || (found && entry.Branch == origin.ParentBranch)
		}
	}

	for _, entry := range warpEntries {
		if entry.Main {
			warpDir = filepath.FromSlash(entry.Path)
		}
		if entry.Prunable || entry.Branch == "" || entry.Branch == "(detached)" || !wanted(entry) {
			continue
		}
		worktree := filepath.FromSlash(entry.Path)
		name := filepath.Base(filepath.Clean(worktree))
		slug := ""
		if !entry.Main {
			slug = name
			pairs = append(pairs, slug)
		}
		targets = append(targets, originTarget{side: failedPushSideCode, branch: entry.Branch, worktree: worktree, codeWorktree: worktree, slug: slug, prime: entry.Main})

		sibling := RecordsWorktreePath(l, name)
		if i := slices.IndexFunc(recordsEntries, func(candidate WorktreeEntry) bool { return samePath(candidate.Path, sibling) }); i >= 0 && recordsEntries[i].Branch != "" {
			targets = append(targets, originTarget{side: failedPushSideRecords, branch: recordsEntries[i].Branch, worktree: sibling, codeWorktree: worktree, slug: slug, prime: entry.Main})
		}
	}

	board := BoardDir(l.HubPath)
	if i := slices.IndexFunc(recordsEntries, func(candidate WorktreeEntry) bool { return samePath(candidate.Path, board) }); i >= 0 && recordsEntries[i].Branch != "" {
		targets = append(targets, originTarget{side: failedPushSideBolt, branch: recordsEntries[i].Branch, worktree: board})
	}

	kept := targets[:0]
	for _, target := range targets {
		target.repoDir = warpDir
		if target.side != failedPushSideCode {
			target.repoDir = recordsDir
		}
		sha, err := gitrepo.New(target.repoDir).RefSHA(trackingRefOf(target.branch))
		if errors.Is(err, gitrepo.ErrRefNotFound) {
			continue
		}
		if err != nil {
			return nil, nil, "", "", err
		}
		target.tracking = sha
		kept = append(kept, target)
	}
	return kept, pairs, warpDir, recordsDir, nil
}

// samePath reports whether a porcelain worktree path and a path built by fabric name the same directory.
func samePath(porcelain, built string) bool {
	return filepath.Clean(filepath.FromSlash(porcelain)) == filepath.Clean(built)
}

// ReconcileOrigin judges this machine's local branches against origin and returns the verdict on each, and on each entry whose pair has a batten run record.
// It runs one ls-remote per repository and fetches only the heads origin moved.
// A judge-only scope stops there.
// Any other scope then reads the ended set and moves each branch behind origin: a fast-forward of its checkout, or a replay of a records branch's local commits.
// A branch a live pair, a held lock or a dirty path keeps from moving is reported as such and left where it was, and no branch is ever reset, rebased or pushed.
// A transport failure that outlasts the retry bound is the report's Unreachable, with nothing judged against origin and nothing moved, and a nil error.
// Any other failure of ls-remote, or a failed fetch after both ls-remote calls answered, is returned as the error, with nothing judged or moved.
// seams is what the moves reach by injection.
func ReconcileOrigin(l *lyxcwd.Location, scope ReconcileScope, seams OriginSeams, opts ReconcileOptions) (res OriginReport, err error) {
	rec := NewMutations(l.HubPath)
	defer func() { res.Mutations = rec.Snapshot() }()

	targets, pairs, warpDir, recordsDir, err := originTargets(l, scope)
	if err != nil {
		return res, err
	}

	warpHeads, recordsHeads, err := lsRemotePair(warpDir, recordsDir, opts)
	if gitexec.IsTransportFailure(err) {
		res.Unreachable = true
		return res, nil
	}
	if err != nil {
		return res, err
	}

	moved := map[string]map[string]string{warpDir: {}, recordsDir: {}}
	for _, target := range targets {
		heads := recordsHeads
		if target.side == failedPushSideCode {
			heads = warpHeads
		}
		if head, found := heads[target.branch]; found && head != target.tracking {
			moved[target.repoDir][target.branch] = head
		}
	}
	for _, dir := range []string{warpDir, recordsDir} {
		if err := fetchMovedHeads(dir, moved[dir], scope.JudgeOnly); err != nil {
			return res, err
		}
	}

	for _, target := range targets {
		heads := recordsHeads
		if target.side == failedPushSideCode {
			heads = warpHeads
		}
		outcome, err := judgeBranch(target.repoDir, target.branch, heads[target.branch], target.side != failedPushSideCode)
		if err != nil {
			return res, err
		}
		outcome.Repo = target.side
		outcome.Slug = target.slug
		outcome.Prime = target.prime
		outcome.Worktree = target.worktree
		outcome.CodeWorktree = target.codeWorktree
		outcome.WayForward = outcome.stopWayForward()
		res.Branches = append(res.Branches, outcome)
	}

	recorded, err := recordedPairs(l, pairs)
	if err != nil {
		return res, err
	}
	if !scope.JudgeOnly {
		ended, err := readOriginEnded(l, targets, res.Branches, recorded)
		if err != nil {
			return res, err
		}
		mover := &originMover{rec: rec, l: l, scope: scope, seams: seams, ended: ended, targets: targets, outcomes: res.Branches, live: map[string]bool{}}
		if err := mover.moveAll(pairs); err != nil {
			return res, err
		}
	}

	res.Entries = classifyEntries(scope, recorded, res.Branches)
	return res, nil
}

// classifyEntries returns the verdict on each of the recorded pairs, from the verdicts on its branches.
// A stop on Bolt or on the prime's records branch stops every entry, a busy one excepted.
// A busy records branch of the run scope's own pair stops nothing, since the run's own driver holds it.
// A stopped entry carries the way forward of the branch that stopped it.
func classifyEntries(scope ReconcileScope, recorded []string, branches []BranchOutcome) []EntryOutcome {
	d3Stop := false
	d3WayForward := ""
	for _, branch := range branches {
		shared := branch.Repo == failedPushSideBolt || (branch.Repo == failedPushSideRecords && branch.Prime)
		if shared && branch.State.stops() && branch.State != BranchBusy && !d3Stop {
			d3Stop = true
			d3WayForward = branch.WayForward
		}
	}

	var entries []EntryOutcome
	for _, slug := range recorded {
		facts := entryFacts{D3Stop: d3Stop}
		wayForward := ""
		for _, branch := range branches {
			if branch.Slug != slug || (scope.Run == slug && branch.Repo == failedPushSideRecords && branch.State == BranchBusy) {
				continue
			}
			facts.Branches = append(facts.Branches, branch.State)
			if wayForward == "" && branch.State.stops() {
				wayForward = branch.WayForward
			}
		}
		state := classifyEntry(facts)
		if state != EntryStopped {
			wayForward = ""
		} else if wayForward == "" {
			wayForward = d3WayForward
		}
		entries = append(entries, EntryOutcome{Slug: slug, State: state, WayForward: wayForward})
	}
	return entries
}
