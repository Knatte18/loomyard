// originreconcile.go declares the options and seams a reconcile call takes and holds the reconcile itself.
// It runs the network calls that bring this machine's view of origin up to date.
// It also reads what a branch's verdict against origin's head needs.

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

	"github.com/Knatte18/loomyard/internal/fabriccore"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// originRemote is the remote every reconcile call reads.
const originRemote = "origin"

// defaultAttemptDeadline bounds one remote call of a reconcile whose options name no deadline.
const defaultAttemptDeadline = 20 * time.Second

// ReconcileOptions bounds a reconcile's remote calls, every field injectable.
// The zero value makes one attempt under the default attempt deadline.
// RetryBound and RetryInterval together bound how often the ls-remote pair is retried after a transport failure.
// There is one retry per whole interval in the bound.
// AttemptDeadline bounds each remote call.
// PushLockWait bounds how long a records branch move waits while only its push lock is held, polling once a second;
// zero reads a held push lock as busy at once.
// Sleep waits out one retry interval or one push-lock poll; nil sleeps for real.
// CallHook sees the arguments of every remote call just before it runs.
// VerdictHook runs just before each branch verdict.
// Both are nil in production.
type ReconcileOptions struct {
	RetryBound      time.Duration
	RetryInterval   time.Duration
	AttemptDeadline time.Duration
	PushLockWait    time.Duration
	Sleep           func(time.Duration)
	CallHook        func(args []string)
	VerdictHook     func()
}

// callHook hands the arguments of one remote call to CallHook, if set.
func (o ReconcileOptions) callHook(args []string) {
	if o.CallHook != nil {
		o.CallHook(args)
	}
}

// verdictHook runs VerdictHook, if set.
func (o ReconcileOptions) verdictHook() {
	if o.VerdictHook != nil {
		o.VerdictHook()
	}
}

// attempts returns how many times the ls-remote pair runs at most.
func (o ReconcileOptions) attempts() int {
	if o.RetryInterval <= 0 || o.RetryBound <= 0 {
		return 1
	}
	return 1 + int(o.RetryBound/o.RetryInterval)
}

// The retry bound and interval of a reconcile that a run's start, resume or leftover teardown performs.
const (
	runStartRetryBound    = time.Minute
	runStartRetryInterval = 10 * time.Second
)

// RunStartReconcileOptions returns the options of an origin reconcile that a run performs.
// That covers a run's start, its resume and its leftover teardown.
// An unreachable origin is retried every ten seconds for a minute before the reconcile reports it unreachable.
// The retries run under the hub's remote-refs locks, so the bound is the shortest that still waits out a brief drop.
func RunStartReconcileOptions() ReconcileOptions {
	return ReconcileOptions{RetryBound: runStartRetryBound, RetryInterval: runStartRetryInterval}
}

// deadline returns the bound of one remote call.
func (o ReconcileOptions) deadline() time.Duration {
	if o.AttemptDeadline <= 0 {
		return defaultAttemptDeadline
	}
	return o.AttemptDeadline
}

// sleepFor waits out d through Sleep, or for real when Sleep is nil.
func (o ReconcileOptions) sleepFor(d time.Duration) {
	if o.Sleep != nil {
		o.Sleep(d)
		return
	}
	time.Sleep(d)
}

// OriginSeams carries what a reconcile reaches only by injection.
// fabricengine knows neither a run's driver, nor a parent lock, nor pair teardown.
// LivePair reports whether slug's driver strand, run lock, loop lock or hub pause is live.
// PrimeParentLockFree reads once whether the prime branch's parent lock record is free.
// Teardown removes the local pair of an ended entry.
// OwnerOf answers whether another machine owns an entry; nil reads every entry as owned here.
type OriginSeams struct {
	LivePair            func(slug string) (bool, error)
	PrimeParentLockFree func() (bool, error)
	Teardown            func(fabriclayout.EndedTeardown) (fabriclayout.TeardownReport, error)
	OwnerOf             OwnerReader
}

// OwnerReader answers the owner question over the run ledger's file of the hub at hubPath.
// It returns the effective owner of slug and the subject `lym adopt` takes to move it.
// It also reports whether that owner is another machine.
// An error, including this machine having no name, is returned to the reconcile asking it,
// so no owner is ever compared with an empty name.
type OwnerReader func(hubPath string, ledger []byte, slug string) (owner, subject string, elsewhere bool, err error)

// branchStopWayForward returns the repair step for a stopped branch.
// It reads from disk the directories the step names, and a blank anchor marker for an anchor stop with no anchor.
// A state that does not stop returns the empty string.
func branchStopWayForward(b fabriclayout.BranchOutcome) string {
	if !b.State.Stops() {
		return b.MoveWayForward
	}
	emptyMarkerRefusal := ""
	if b.State == fabriclayout.BranchAnchorStopped && b.Anchor == "" {
		if err := refuseEmptyOwnMarker(b.Worktree); err != nil {
			emptyMarkerRefusal = err.Error()
		}
	}
	return fabriccore.BranchStopWayForward(b, verbDirsOf(b.Worktree, b.CodeWorktree), emptyMarkerRefusal)
}

// lsRemoteOnce runs the two ls-remote calls once, in parallel.
// A non-transport failure wins over a transport one, so a retrying caller never retries a refusal.
func lsRemoteOnce(warpDir, recordsDir string, opts ReconcileOptions) (warp, records map[string]string, err error) {
	var warpErr, recordsErr error
	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer wait.Done()
		warp, warpErr = lsRemoteHeads(fabriclayout.RepoCode, warpDir, opts)
	}()
	go func() {
		defer wait.Done()
		records, recordsErr = lsRemoteHeads(fabriclayout.RepoRecords, recordsDir, opts)
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

// lsRemoteHeads returns origin's branch heads of the repository at dir by branch name, under opts' attempt deadline.
func lsRemoteHeads(side, dir string, opts ReconcileOptions) (map[string]string, error) {
	ns, err := originNamespaceAt(dir)
	if err != nil {
		return nil, err
	}
	args := []string{"ls-remote", originRemote, ns.Branch("*")}
	opts.callHook(args)
	out, err := gitexec.RunWithin(args, dir, opts.deadline())
	if err != nil {
		return nil, fmt.Errorf("read origin's branches of the %s repository at %s: %w", side, dir, err)
	}
	heads := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		sha, ref, found := strings.Cut(strings.TrimSpace(line), "\t")
		if !found {
			continue
		}
		if local, mapped := ns.LocalRef(ref); mapped {
			if name, isBranch := strings.CutPrefix(local, "refs/heads/"); isBranch {
				heads[name] = sha
			}
		}
	}
	return heads, nil
}

// fetchMovedHeads fetches the branches named in heads in one fetch, and updates each one's tracking ref.
// heads maps a branch name to origin's head for it.
// A caller passes only the branches whose head differs from the local tracking ref.
// The tracking ref is forced, since origin may have rewound or rewritten the branch.
// The caller holds the repository's remote-refs lock, which this fetch does not take.
// With `judgeOnly` it fetches the objects by SHA into no ref and writes no FETCH_HEAD, so a dry run moves nothing.
// No head makes no call.
// opts' call hook sees the fetch's arguments.
func fetchMovedHeads(dir string, heads map[string]string, judgeOnly bool, opts ReconcileOptions) error {
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
		ns, err := originNamespaceAt(dir)
		if err != nil {
			return err
		}
		args = append(args, originRemote)
		for _, branch := range branches {
			args = append(args, fmt.Sprintf("+%s:%s", ns.Branch(branch), trackingRefOf(branch)))
		}
	}
	opts.callHook(args)
	if _, err := gitexec.Run(args, dir); err != nil {
		return fmt.Errorf("fetch the moved branches %s from origin into %s: %w", strings.Join(branches, " "), dir, err)
	}
	return nil
}

// trackingRefOf returns the remote-tracking ref of a branch on origin.
func trackingRefOf(branch string) string {
	return "refs/remotes/" + originRemote + "/" + branch
}

// judgeBranch reads into facts what the verdict on a branch of repoDir against origin's head needs.
// It stops at the first read that settles the verdict, and reads nothing of a branch origin does not hold.
// A code branch is read through the reflog of its tracking ref.
func judgeBranch(repoDir string, facts *fabriccore.BranchFacts) error {
	if facts.OriginHead == "" {
		return nil
	}
	fail := func(err error) error {
		return fmt.Errorf("judge branch %s in %s: %w", facts.Branch, repoDir, err)
	}

	repo := gitrepo.New(repoDir)
	localTip, err := repo.RefSHA("refs/heads/" + facts.Branch)
	if err != nil {
		return fail(err)
	}
	facts.LocalTip = localTip
	if localTip == facts.OriginHead {
		return nil
	}

	if facts.Repo == fabriclayout.RepoCode {
		earlier, err := trackingReflog(repoDir, trackingRefOf(facts.Branch))
		if err != nil {
			return fail(err)
		}
		kind, dropped, err := classifyRewrite(repo, localTip, facts.OriginHead, earlier)
		if err != nil {
			return fail(err)
		}
		if kind == rewriteRewound {
			facts.Rewound, facts.Dropped = true, dropped
			return nil
		}
		facts.Rewritten = kind == rewriteRewritten
	}

	if facts.Behind, err = repo.IsAncestor(localTip, facts.OriginHead); err != nil {
		return fail(err)
	}
	if facts.Behind {
		return nil
	}
	if facts.Ahead, err = repo.IsAncestor(facts.OriginHead, localTip); err != nil {
		return fail(err)
	}
	return nil
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
	// detached reports that the code worktree is detached under a detach marker, whose branch the code target names.
	detached bool
}

// originTargets lists the branches of the scope that have an origin tracking ref.
// A branch never pushed has no origin head to judge against.
// A detached code worktree is listed under the branch its detach marker names.
// One with no marker, or whose marker cannot be read, is left out with its records branch.
// It also returns the slug of every pair the scope covers, for classifying their entries.
// The repository directories the network calls run in are returned as well.
// A run scope whose task worktree is not registered refuses, unless `scope.PairMayBeAbsent` is set.
// With it set, absent is true when the pair is not registered.
// Absent is also true when the registration's directory is gone and not yet pruned.
// In that case the slug still joins pairs, and no warp or records branch of the run or of its parent is listed.
// Bolt's target is still listed.
func originTargets(l *lymcwd.Location, scope fabriclayout.ReconcileScope) (targets []originTarget, pairs []string, warpDir, recordsDir string, absent bool, err error) {
	warpEntries, err := List(l.AnchorPath())
	if err != nil {
		return nil, nil, "", "", false, err
	}
	recordsDir, err = RecordsRepoRoot(l)
	if err != nil {
		return nil, nil, "", "", false, err
	}
	recordsEntries, err := List(recordsDir)
	if err != nil {
		return nil, nil, "", "", false, err
	}

	wanted := func(entry fabriclayout.WorktreeEntry) bool { return true }
	if scope.Run != "" {
		own := WorktreePath(l, scope.Run)
		ownIndex := slices.IndexFunc(warpEntries, func(entry fabriclayout.WorktreeEntry) bool { return samePath(entry.Path, own) })
		registered := ownIndex >= 0
		// A registration whose directory is gone reads as an absent pair only when the scope may find its pair absent.
		absentPair := !registered || (warpEntries[ownIndex].Prunable && scope.PairMayBeAbsent)
		switch {
		case !registered && !scope.PairMayBeAbsent:
			return nil, nil, "", "", false, fmt.Errorf("fabricengine: origin reconcile of run %s: no pair worktree %s", scope.Run, own)
		case absentPair:
			absent = true
			pairs = append(pairs, scope.Run)
			wanted = func(fabriclayout.WorktreeEntry) bool { return false }
		default:
			origin, found, err := ReadOriginFor(l, scope.Run)
			if err != nil {
				return nil, nil, "", "", false, err
			}
			wanted = func(entry fabriclayout.WorktreeEntry) bool {
				return samePath(entry.Path, own) || (found && entry.Branch == origin.ParentBranch)
			}
		}
	}

	for _, entry := range warpEntries {
		if entry.Main {
			warpDir = filepath.FromSlash(entry.Path)
		}
		if entry.Prunable || entry.Branch == "" {
			continue
		}
		detached := entry.Branch == detachedBranch
		if detached {
			// Any detached worktree of the repository lands here, so one that cannot be read fails no reconcile.
			_, branch, marked, err := markedWorktree(filepath.FromSlash(entry.Path))
			if err != nil {
				logger.Warn("fabricengine: a detached worktree's marker cannot be read, so its branches are not judged", "worktree", entry.Path, "cause", err)
			}
			if err != nil || !marked {
				continue
			}
			entry.Branch = branch
		}
		if !wanted(entry) {
			continue
		}
		worktree := filepath.FromSlash(entry.Path)
		name := filepath.Base(filepath.Clean(worktree))
		slug := ""
		if !entry.Main {
			slug = name
			pairs = append(pairs, slug)
		}
		targets = append(targets, originTarget{side: fabriclayout.RepoCode, branch: entry.Branch, worktree: worktree, codeWorktree: worktree, slug: slug, prime: entry.Main, detached: detached})

		sibling := RecordsWorktreePath(l, name)
		if i := slices.IndexFunc(recordsEntries, func(candidate fabriclayout.WorktreeEntry) bool { return samePath(candidate.Path, sibling) }); i >= 0 && recordsEntries[i].Branch != "" {
			targets = append(targets, originTarget{side: fabriclayout.RepoRecords, branch: recordsEntries[i].Branch, worktree: sibling, codeWorktree: worktree, slug: slug, prime: entry.Main, detached: detached})
		}
	}

	board := fabriclayout.BoltDir(l.HubPath)
	if i := slices.IndexFunc(recordsEntries, func(candidate fabriclayout.WorktreeEntry) bool { return samePath(candidate.Path, board) }); i >= 0 && recordsEntries[i].Branch != "" {
		targets = append(targets, originTarget{side: fabriclayout.RepoBolt, branch: recordsEntries[i].Branch, worktree: board})
	}

	kept := targets[:0]
	for _, target := range targets {
		target.repoDir = warpDir
		if target.side != fabriclayout.RepoCode {
			target.repoDir = recordsDir
		}
		sha, err := gitrepo.New(target.repoDir).RefSHA(trackingRefOf(target.branch))
		if errors.Is(err, gitrepo.ErrRefNotFound) {
			continue
		}
		if err != nil {
			return nil, nil, "", "", false, err
		}
		target.tracking = sha
		kept = append(kept, target)
	}
	return kept, pairs, warpDir, recordsDir, absent, nil
}

// samePath reports whether a porcelain worktree path and a path built by fabric name the same directory.
func samePath(porcelain, built string) bool {
	return filepath.Clean(filepath.FromSlash(porcelain)) == filepath.Clean(built)
}

// ReconcileOrigin judges this machine's local branches against origin.
// It runs one ls-remote per repository and fetches only the heads origin moved.
// It holds each repository's remote-refs lock from the ls-remote through the branch verdicts,
// so no push's tracking-ref write lands between them and a push in flight is never judged a rewind.
// It returns the verdict on each branch, and on each entry whose pair has a batten run record.
// A pair whose code worktree is detached under a detach marker is judged by the marker's branch,
// and no move touches its code branch.
// It reads the ended set and tears down, through the seam, the local pair of each run origin has ended.
// That happens before any branch moves.
// A judge-only scope reports those teardowns and runs none.
// It also asks the owner seam which recorded entries another machine owns, over origin's Bolt tip.
// It reports those as owned_elsewhere.
// A judge-only scope stops there.
// Any other scope then moves each branch behind origin.
// A branch moves by a fast-forward of its checkout, or by a replay of a records branch's local commits.
// An entry owned elsewhere has its fast-forwards moved and its records branch left unreplayed.
// A branch a live pair, a held lock or a dirty path keeps from moving is reported as such and left where it was;
// a held records push lock is first waited on for up to opts' PushLockWait.
// No branch is ever reset, rebased or pushed.
// A moving scope also clears the failed-push record of each branch that is absent locally.
// A transport failure that outlasts the retry bound sets the report's Unreachable.
// Nothing is judged against origin or moved in that case.
// The error is nil in that case.
// Any other failure of ls-remote, or a failed fetch after both ls-remote calls answered, is returned as the error.
// Nothing is judged or moved in that case.
// seams is what the moves and the teardowns reach by injection.
// It refuses before it reads origin when a required seam it would use is not installed.
func ReconcileOrigin(l *lymcwd.Location, scope fabriclayout.ReconcileScope, seams OriginSeams, opts ReconcileOptions) (res fabriclayout.OriginReport, err error) {
	rec := NewMutations(l.HubPath)
	defer func() {
		res.Mutations = fabriclayout.MutationsOf(append(res.Mutations.Entries(), rec.Snapshot().Entries()...))
	}()

	reader, snapshot, err := judgeAgainstOrigin(l, scope, opts)
	if err != nil {
		return res, err
	}
	if !snapshot.Stores[fabriccore.StoreCode].OriginRead {
		res.Unreachable = true
		return res, nil
	}
	for _, facts := range snapshot.Reconcile.Targets {
		outcome := fabriccore.JudgeBranch(facts)
		outcome.WayForward = branchStopWayForward(outcome)
		res.Branches = append(res.Branches, outcome)
	}
	targets, pairs, pairAbsent := reader.targets, snapshot.Reconcile.Pairs, snapshot.Reconcile.PairAbsent

	recorded, err := recordedPairs(l, pairs)
	if err != nil {
		return res, err
	}
	runs, err := recordedRuns(l, scope)
	if err != nil {
		return res, err
	}
	owners, subjects, err := readOriginOwners(l, seams, targets, res.Branches, recorded)
	if err != nil {
		return res, err
	}
	ended, err := readOriginEnded(l, targets, res.Branches, runs)
	if err != nil {
		return res, err
	}
	if err := reconcileEnded(l, scope, seams, onlyRuns(ended, runs), &res); err != nil {
		return res, err
	}
	endedEntries := res.Entries
	res.Entries = nil

	if !scope.JudgeOnly {
		mover := &originMover{rec: rec, l: l, scope: scope, seams: seams, opts: opts, ended: ended, targets: targets, outcomes: res.Branches, live: map[string]bool{}, ownedByEnded: endedLocalSlugs(endedEntries, scope.Run), owners: owners}
		if err := mover.moveAll(pairs); err != nil {
			return res, err
		}
		res.Branches = fabriccore.WithoutRemovedPairs(res.Branches, endedEntries)
		if err := clearStalePushFailures(l.HubPath); err != nil {
			return res, err
		}
	}

	res.Entries = fabriccore.MergeEndedEntries(fabriccore.ClassifyEntries(scope, recorded, res.Branches, owners, subjects), endedEntries)
	if pairAbsent {
		for i := range res.Entries {
			if res.Entries[i].Slug == scope.Run {
				res.Entries[i].PairAbsent = true
			}
		}
	}
	return res, nil
}

// judgeAgainstOrigin runs the rule that judges the scope's branches against origin.
// It returns the reader, holding the targets, and the snapshot the rule's phases read;
// a snapshot whose origin was never read is of an unreachable origin.
// It holds both repositories' remote-refs locks, the code repository's first, from the read of origin until it returns.
// A fetch of either repository therefore finishes before origin is read, or starts after every branch's reads.
// The same holds for the tracking-ref write that follows a push's transfer.
// The locks are held through the retries of an unreachable origin, bounded by opts' RetryBound,
// so it blocks every fetch and every push's tracking-ref write of both repositories for that bound.
func judgeAgainstOrigin(l *lymcwd.Location, scope fabriclayout.ReconcileScope, opts ReconcileOptions) (*reconcileReader, fabriccore.Snapshot, error) {
	reader := &reconcileReader{l: l, scope: scope, opts: opts}
	var releases []func() error
	releaseAll := func() {
		for _, release := range slices.Backward(releases) {
			_ = release()
		}
		releases = nil
	}
	// The directories to lock are the ones the pre-lock read found.
	var codeDir, recordsDir string
	read := func(phase fabriccore.Phase, fields []fabriccore.Field, snapshot *fabriccore.Snapshot) error {
		err := reader.read(phase, fields, snapshot)
		codeDir, recordsDir = snapshot.Reconcile.CodeDir, snapshot.Reconcile.RecordsDir
		return err
	}
	lock := func(phase fabriccore.Phase) (func(), error) {
		if phase != fabriccore.PhaseOrigin {
			return func() {}, nil
		}
		for _, dir := range []string{codeDir, recordsDir} {
			release, err := gitrepo.New(dir).HoldRemoteRefs()
			if err != nil {
				releaseAll()
				return nil, err
			}
			releases = append(releases, release)
		}
		return releaseAll, nil
	}
	start := fabriccore.Answer{Phase: fabriccore.PhasePreLock, Read: []fabriccore.Field{fabriccore.FieldSeams, fabriccore.FieldReconcileTargets}}
	snapshot, err := runRule(fabriccore.OriginJudge, start, read, lock, nil, opts.sleepFor)
	return reader, snapshot, err
}

// readOriginOwners asks seams.OwnerOf, once per slug, whether another machine owns the entry.
// It reads the run ledger's file at origin's fetched Bolt tip.
// It returns the owner and the adopt subject of each slug read as owned elsewhere.
// It returns nothing for a hub with no Bolt target, no origin head or no OwnerOf seam.
// It returns ErrReplayerMissing when it would read the ledger and none is registered.
// The tip's objects are present even for a judge-only scope, which fetches them by SHA,
// so a dry run reads owners too and moves nothing.
// A seam error is returned, so no verdict is ever compared with an empty machine name.
func readOriginOwners(l *lymcwd.Location, seams OriginSeams, targets []originTarget, outcomes []fabriclayout.BranchOutcome, slugs []string) (owners, subjects map[string]string, err error) {
	boltIndex := slices.IndexFunc(targets, func(target originTarget) bool { return target.side == fabriclayout.RepoBolt })
	if seams.OwnerOf == nil || len(slugs) == 0 || boltIndex < 0 || outcomes[boltIndex].OriginHead == "" {
		return nil, nil, nil
	}
	ledger, _, err := runLedger()
	if err != nil {
		return nil, nil, err
	}
	ledgerBytes, err := blobAt(gitrepo.New(targets[boltIndex].worktree), outcomes[boltIndex].OriginHead, ledger.File())
	if err != nil {
		return nil, nil, fmt.Errorf("fabricengine: read %s at origin's Bolt tip: %w", ledger.File(), err)
	}

	owners = map[string]string{}
	subjects = map[string]string{}
	for _, slug := range slugs {
		owner, subject, elsewhere, err := seams.OwnerOf(l.HubPath, ledgerBytes, slug)
		if err != nil {
			return nil, nil, fmt.Errorf("fabricengine: read the owner of %s: %w", slug, err)
		}
		if elsewhere {
			owners[slug] = owner
			subjects[slug] = subject
		}
	}
	return owners, subjects, nil
}
