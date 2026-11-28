// landlocally_integration_test.go drives a local landing over a hubforge hub with bare origins:
// the seam that records it, and Finalize through landingDeps.

package loomcli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/pairteardown"
	"github.com/Knatte18/loomyard/internal/pullrequest"
	"github.com/Knatte18/loomyard/internal/pushledger"
	"github.com/Knatte18/loomyard/internal/runpusher"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/summaryparser"
	"github.com/Knatte18/loomyard/internal/testkit/prkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// atFinalize puts the run slug at its Finalize row, with a status file and a change description.
func (f *parentFixture) atFinalize(slug string) *lymcwd.Location {
	f.t.Helper()
	pair := f.location(slug)
	status := shedengine.Status{State: shedengine.StateRunning, CurrentProducer: "Finalize"}
	if err := state.WriteJSON(shedrun.StatusFile(pair, shedrun.SelfRunID), shedrun.StatusLock(pair, shedrun.SelfRunID), status); err != nil {
		f.t.Fatalf("write the run status: %v", err)
	}
	description := summaryparser.Path(loomengine.LandingDir(pair))
	if err := os.MkdirAll(filepath.Dir(description), 0o755); err != nil {
		f.t.Fatalf("create the landing directory: %v", err)
	}
	if err := os.WriteFile(description, []byte("# Land "+slug+"\n\nThe work of "+slug+".\n"), 0o644); err != nil {
		f.t.Fatalf("write the change description: %v", err)
	}
	return pair
}

// work commits the task's own file on the pair of slug.
func (f *parentFixture) work(slug string) {
	f.t.Helper()
	gitkit.CommitFile(f.t, f.h.PairCodeWorktree(slug), slug+".txt", slug+"\n", "the work of "+slug)
}

// finalize builds Finalize over the production landing wiring of the run slug.
// The verify gate and the engine ledger are left unwired, since no session runs here.
func (f *parentFixture) finalize(slug string, squash bool) (*landingshed.Finalize, landingshed.Deps) {
	f.t.Helper()
	pair := f.atFinalize(slug)
	cfg := landingshed.Config{Squash: squash, FinalizeLostRaces: 1, Conflict: "claude:test-model", ConflictTimeoutMin: 1}
	statusFile, statusLock := shedrun.StatusFile(pair, shedrun.SelfRunID), shedrun.StatusLock(pair, shedrun.SelfRunID)
	deps := landingDeps(pair, shedrun.RecipeTapestry, websterengine.Geometry{StencilsDir: f.t.TempDir()}, f.branches[slug], f.branch,
		false, modelspec.Registry{}, &shuttleengine.Runner{}, cfg, "", nil, nil, nil, planVerifySource(pair), parentLockDeps(pair, shedrun.SelfRunID, statusFile, statusLock))
	deps.GateKey = "finalize"
	deps.VerifyCommand = nil
	deps.EnginesRun = nil
	fz, err := landingshed.NewFinalize(deps)
	if err != nil {
		f.t.Fatalf("NewFinalize() error = %v", err)
	}
	return fz, deps
}

// requireLands runs Finalize of the run slug and fails unless it is Done.
func (f *parentFixture) requireLands(slug string, squash bool) landingshed.Deps {
	f.t.Helper()
	fz, deps := f.finalize(slug, squash)
	outcome, pointer, err := fz.Call(context.Background())
	if err != nil || outcome != shedengine.Done {
		f.t.Fatalf("Call() = %q, %+v, %v; want Done", outcome, pointer, err)
	}
	return deps
}

func (f *parentFixture) primeHead() string {
	f.t.Helper()
	return gitkit.RevParse(f.t, f.h.PrimeWorktree(), "HEAD")
}

// requireTrailerMerge fails unless the prime's head is a merge commit carrying the landing trailer of slug.
func (f *parentFixture) requireTrailerMerge(slug string) {
	f.t.Helper()
	parents := strings.Fields(gitkit.Git(f.t, f.h.PrimeWorktree(), "log", "-1", "--format=%P"))
	message := gitkit.Git(f.t, f.h.PrimeWorktree(), "log", "-1", "--format=%B")
	if trailer := fabriclayout.LandingTrailerKey + ": " + slug; len(parents) != 2 || !strings.Contains(message, trailer) {
		f.t.Errorf("prime head has %d parents and message %q; want a merge commit carrying %q", len(parents), message, trailer)
	}
}

func TestLandingDeps_LocalLanding(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	t.Run("MergesRecordsTheLandingAndPushesNothing", func(t *testing.T) {
		t.Parallel()
		f := newParentFixture(t)
		f.addRun("one", "", false)
		f.work("one")
		originCode := f.originTip(f.branch)
		originRecords := gitkit.Git(t, f.h.RecordsBare, "for-each-ref", "refs/heads")

		deps := f.requireLands("one", true)

		entry := f.entry(pushledger.LandingKey)
		records := gitkit.RevParse(t, f.h.PairRecordsSibling("one"), "HEAD")
		if len(entry.Landings) != 1 || entry.Landings[0].Run != "one" || entry.Landings[0].Tip != f.primeHead() ||
			entry.Landings[0].RecordsCommit != records || entry.Landings[0].ParentBranch != f.branch {
			t.Errorf("landings = %+v; want one of run one at %s holding %s", entry.Landings, f.primeHead(), records)
		}
		if got := f.originTip(f.branch); got != originCode {
			t.Errorf("origin parent branch = %s; want it unchanged at %s", got, originCode)
		}
		if got := gitkit.Git(t, f.h.RecordsBare, "for-each-ref", "refs/heads"); got != originRecords {
			t.Errorf("origin records branches = %q; want them unchanged: %q", got, originRecords)
		}
		if f.done("one") {
			t.Error("the board entry is done; want the mark left to the parent's pusher")
		}
		if got, err := deps.ParentPushState(); err != nil || got != (landingshed.ParentPush{Owed: true}) {
			t.Errorf("ParentPushState() = %+v, %v; want owed and not refused", got, err)
		}
		refusal := fabriclayout.FailedPush{Repo: fabriclayout.RepoCode, RepoPath: f.h.PrimeWorktree(), Branch: f.branch, Prime: true, Class: fabriclayout.PushFailureHeldBack, WayForward: "run the recorded step"}
		if err := fabricengine.RecordPushFailure(f.h.Path, refusal, time.Now()); err != nil {
			t.Fatalf("RecordPushFailure: %v", err)
		}
		want := landingshed.ParentPush{Owed: true, Refused: true, WayForward: "run the recorded step"}
		if got, err := deps.ParentPushState(); err != nil || got != want {
			t.Errorf("ParentPushState() = %+v, %v; want %+v", got, err, want)
		}
	})

	t.Run("AnUnreachableOriginIsATransientStopAndLandsOnceOriginAnswers", func(t *testing.T) {
		t.Parallel()
		f := newParentFixture(t)
		f.addRun("one", "", false)
		f.work("one")
		fz, _ := f.finalize("one", true)
		before := f.primeHead()
		repos := []string{f.h.PrimeWorktree(), f.h.PrimeRecords()}
		urls := make([]string, len(repos))
		for i, repo := range repos {
			urls[i] = strings.TrimSpace(gitkit.Git(t, repo, "config", "--get", "remote.origin.url"))
			// A refused connection reads as unreachable, where a missing directory reads as a broken remote.
			gitkit.Git(t, repo, "remote", "set-url", "origin", "http://127.0.0.1:1/x.git")
		}

		outcome, _, err := fz.Call(context.Background())

		if outcome != "" || shedengine.TransientOf(err) != shedengine.TransientGitTransport {
			t.Fatalf("Call() = %q, %v; want a git-transport transient stop", outcome, err)
		}
		if f.primeHead() != before || len(f.entry(pushledger.LandingKey).Landings) != 0 {
			t.Errorf("prime head = %s, landings = %+v; want nothing merged or recorded", f.primeHead(), f.entry(pushledger.LandingKey).Landings)
		}
		for i, repo := range repos {
			gitkit.Git(t, repo, "remote", "set-url", "origin", urls[i])
		}
		if outcome, pointer, err := fz.Call(context.Background()); err != nil || outcome != shedengine.Done {
			t.Fatalf("Call() with origin back = %q, %+v, %v; want Done", outcome, pointer, err)
		}
		if landings := f.entry(pushledger.LandingKey).Landings; len(landings) != 1 || landings[0].Tip != f.primeHead() {
			t.Errorf("landings = %+v; want the one landing at %s", landings, f.primeHead())
		}
	})

	t.Run("AResumeOverAParentHoldingTheWorkRecordsItsHead", func(t *testing.T) {
		t.Parallel()
		f := newParentFixture(t)
		f.addRun("one", "", false)
		tip := f.merge("one", f.h.PrimeWorktree())

		f.requireLands("one", true)

		if landings := f.entry(pushledger.LandingKey).Landings; len(landings) != 1 || landings[0].Tip != tip || f.primeHead() != tip {
			t.Errorf("landings = %+v, prime head = %s; want one at %s with nothing new merged", landings, f.primeHead(), tip)
		}
	})

	t.Run("AParentAheadByARecordedTipTakesAnotherLanding", func(t *testing.T) {
		t.Parallel()
		f := newParentFixture(t)
		f.addRun("one", "", false)
		f.addRun("two", "", false)
		f.work("one")
		f.work("two")
		f.requireLands("one", true)

		f.requireLands("two", true)

		if landings := f.entry(pushledger.LandingKey).Landings; len(landings) != 2 || landings[1].Run != "two" || landings[1].Tip != f.primeHead() {
			t.Errorf("landings = %+v; want one and then two at %s", landings, f.primeHead())
		}
	})

	t.Run("AnUnrecordedCommitBelowTheOwedTipIsStuck", func(t *testing.T) {
		t.Parallel()
		f := newParentFixture(t)
		f.addRun("one", "", false)
		f.addRun("two", "", false)
		f.work("two")
		gitkit.CommitFile(t, f.h.PrimeWorktree(), "stray.txt", "stray\n", "an unrecorded commit")
		f.land("one", runpusher.LandingTarget(f.h.Location))
		before := f.primeHead()

		fz, _ := f.finalize("two", true)
		outcome, pointer, err := fz.Call(context.Background())

		if err != nil || outcome != shedengine.Stuck || !strings.Contains(pointer.Reason, "is ahead of its upstream") {
			t.Errorf("Call() = %q, %+v, %v; want Stuck on the parent ahead of its upstream", outcome, pointer, err)
		}
		if f.primeHead() != before {
			t.Errorf("prime head = %s; want it unmoved at %s", f.primeHead(), before)
		}
	})

	t.Run("WithoutSquashEveryLandingIsAMergeCommitCarryingTheTrailer", func(t *testing.T) {
		t.Parallel()
		f := newParentFixture(t)
		f.addRun("one", "", false)
		f.addRun("two", "", false)
		// The task branch holds all of the parent, so a plain merge of it would fast-forward and commit nothing.
		f.work("one")
		f.work("two")

		f.requireLands("one", false)
		f.requireTrailerMerge("one")
		f.requireLands("two", false)
		f.requireTrailerMerge("two")

		if landings := f.entry(pushledger.LandingKey).Landings; len(landings) != 2 || landings[1].Tip != f.primeHead() {
			t.Errorf("landings = %+v; want both recorded, the second at %s", landings, f.primeHead())
		}
	})
}

// TestLandingDeps_ParentPushState runs Finalize through landingDeps while the parent branch owes or refuses a push.
// A landing through GitHub waits or halts without merging there, and a local landing lands.
func TestLandingDeps_ParentPushState(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	// setup builds a hub with the run one at Finalize and a second run other, whose landing the parent can owe.
	setup := func(t *testing.T) (*parentFixture, landingshed.Deps, *prkit.API) {
		t.Helper()
		f := newParentFixture(t)
		f.addRun("one", "", false)
		f.addRun("other", "", false)
		f.work("one")
		_, deps := f.finalize("one", true)
		api := &prkit.API{}
		deps.PullRequest.API = func(string) (pullrequest.API, error) { return api, nil }
		return f, deps, api
	}
	onGitHub := func(deps *landingshed.Deps) {
		deps.PullRequest.Origin = func() (landingshed.Origin, error) {
			return landingshed.Origin{URL: "https://github.com/acme/widgets.git", Upstream: true}, nil
		}
	}
	trail := func(deps *landingshed.Deps) {
		onGitHub(deps)
		deps.PullRequest.Mode = func() (string, error) { return "trail", nil }
	}
	recorded := func(t *testing.T, deps *landingshed.Deps, api *prkit.API) {
		t.Helper()
		onGitHub(deps)
		api.Script(prkit.MethodStatus, prkit.Answer{Status: pullrequest.Status{State: pullrequest.StateOpen}})
		record := pullrequest.Record{Number: 7, URL: "https://github.com/acme/widgets/pull/7"}
		if err := pullrequest.WriteRecord(deps.PullRequest.RecordPath, deps.PullRequest.RecordLockPath, record); err != nil {
			t.Fatalf("WriteRecord: %v", err)
		}
		if err := deps.PullRequest.CommitRecord(); err != nil {
			t.Fatalf("CommitRecord: %v", err)
		}
	}
	call := func(t *testing.T, deps landingshed.Deps) (shedengine.Outcome, shedengine.OutputPointer, error) {
		t.Helper()
		fz, err := landingshed.NewFinalize(deps)
		if err != nil {
			t.Fatalf("NewFinalize() error = %v", err)
		}
		return fz.Call(context.Background())
	}
	requireUntouchedGitHub := func(t *testing.T, api *prkit.API, methods ...string) {
		t.Helper()
		for _, method := range methods {
			if calls := api.Calls(method); len(calls) != 0 {
				t.Errorf("GitHub %s calls = %v; want none", method, calls)
			}
		}
	}
	requireWaits := func(t *testing.T, outcome shedengine.Outcome, err error) {
		t.Helper()
		if outcome != "" || shedengine.TransientOf(err) != shedengine.TransientGitTransport {
			t.Errorf("Call() = %q, %v; want a git-transport transient stop", outcome, err)
		}
	}
	// A call that stopped before the lock never reached the pull-request step, which asks GitHub under the lock.
	beforeLock := []string{prkit.MethodStatus, prkit.MethodCreate, prkit.MethodMerge}

	for name, arrange := range map[string]func(*testing.T, *landingshed.Deps, *prkit.API){
		"ARecordedPullRequestWaitsForTheOwedPushBeforeTheLock": recorded,
		"ATrailLandingWithNoRecordWaitsForTheOwedPushBeforeTheLock": func(_ *testing.T, deps *landingshed.Deps, _ *prkit.API) {
			trail(deps)
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f, deps, api := setup(t)
			arrange(t, &deps, api)
			f.land("other", runpusher.LandingTarget(f.h.Location))
			before := f.primeHead()

			outcome, _, err := call(t, deps)

			requireWaits(t, outcome, err)
			requireUntouchedGitHub(t, api, beforeLock...)
			if f.primeHead() != before {
				t.Errorf("prime head = %s; want it unmoved at %s", f.primeHead(), before)
			}
		})
	}

	t.Run("ARefusedPushHaltsNamingItsWayForwardAndTheResume", func(t *testing.T) {
		t.Parallel()
		f, deps, api := setup(t)
		recorded(t, &deps, api)
		f.land("other", runpusher.LandingTarget(f.h.Location))
		refusal := fabriclayout.FailedPush{Repo: fabriclayout.RepoCode, RepoPath: f.h.PrimeWorktree(), Branch: f.branch, Prime: true, Class: fabriclayout.PushFailureHeldBack, WayForward: "run the recorded step"}
		if err := fabricengine.RecordPushFailure(f.h.Path, refusal, time.Now()); err != nil {
			t.Fatalf("RecordPushFailure: %v", err)
		}
		before := f.primeHead()

		outcome, pointer, err := call(t, deps)

		if err != nil || outcome != shedengine.Stuck || !pointer.Halt {
			t.Fatalf("Call() = %q, %+v, %v; want a blocked halt", outcome, pointer, err)
		}
		if !strings.Contains(pointer.Reason, "run the recorded step") || !strings.HasSuffix(pointer.Reason, "run `lym loom resume` in the task worktree") {
			t.Errorf("reason = %q; want the recorded way forward, ending in the resume", pointer.Reason)
		}
		requireUntouchedGitHub(t, api, beforeLock...)
		if f.primeHead() != before {
			t.Errorf("prime head = %s; want it unmoved at %s", f.primeHead(), before)
		}
	})

	t.Run("ALandingRecordedUnderTheLockStopsBeforeTheGitHubMergeAndFreesTheLock", func(t *testing.T) {
		t.Parallel()
		f, deps, api := setup(t)
		trail(&deps)
		// The pull-request step opens the GitHub client under the parent lock, so the landing is recorded there.
		deps.PullRequest.API = func(string) (pullrequest.API, error) {
			f.land("other", runpusher.LandingTarget(f.h.Location))
			return api, nil
		}

		outcome, _, err := call(t, deps)

		requireWaits(t, outcome, err)
		requireUntouchedGitHub(t, api, prkit.MethodCreate, prkit.MethodMerge)
		if _, held, err := deps.Lock.Store.Read(f.branch); err != nil || held {
			t.Errorf("parent lock held = %v, %v; want it released", held, err)
		}
		if landings := f.entry(pushledger.LandingKey).Landings; len(landings) != 1 || landings[0].Run != "other" {
			t.Errorf("landings = %+v; want that of run other alone", landings)
		}
	})

	t.Run("ALocalLandingLandsOnAnOwedParent", func(t *testing.T) {
		t.Parallel()
		f, deps, api := setup(t)
		f.land("other", runpusher.LandingTarget(f.h.Location))

		outcome, pointer, err := call(t, deps)

		if err != nil || outcome != shedengine.Done {
			t.Fatalf("Call() = %q, %+v, %v; want Done", outcome, pointer, err)
		}
		requireUntouchedGitHub(t, api, beforeLock...)
		if landings := f.entry(pushledger.LandingKey).Landings; len(landings) != 2 || landings[1].Run != "one" {
			t.Errorf("landings = %+v; want other and then one", landings)
		}
	})
}

func TestLandLocally(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	f := newParentFixture(t)
	f.addRun("one", "", false)
	pair := f.atFinalize("one")
	tip := f.merge("one", f.h.PrimeWorktree())
	recordsDir := f.h.PairRecordsSibling("one")
	before := gitkit.RevParse(t, recordsDir, "HEAD")

	release, err := fabricengine.HoldPairPushLock(pair)
	if err != nil {
		t.Fatalf("HoldPairPushLock: %v", err)
	}
	landed := make(chan error, 1)
	go func() { landed <- landLocally(pair, "one")(tip) }()
	// Nothing signals a blocked lock wait, so a short wait gives a call that ignores the lock time to commit.
	select {
	case err := <-landed:
		release()
		t.Fatalf("landLocally() returned %v with the push lock held; want it to wait", err)
	case <-time.After(300 * time.Millisecond):
	}
	if got := gitkit.RevParse(t, recordsDir, "HEAD"); got != before {
		t.Errorf("records head = %s with the push lock held; want no commit past %s", got, before)
	}
	release()
	if err := <-landed; err != nil {
		t.Fatalf("landLocally() = %v; want nil", err)
	}

	records := gitkit.RevParse(t, recordsDir, "HEAD")
	if got := gitkit.Git(t, recordsDir, "log", "-1", "--format=%s"); got != "loom: status checkpoint for one" {
		t.Errorf("records head subject = %q; want the status checkpoint", got)
	}
	entry := f.entry(pushledger.LandingKey)
	if len(entry.Landings) != 1 || entry.Landings[0].Tip != tip || entry.Landings[0].RecordsCommit != records || !entry.PushNow {
		t.Errorf("landing entry = %+v; want the landing at %s holding %s, marked to push now", entry, tip, records)
	}
	if got := f.pairEntry("one"); !got.PushNow || got.Slug != "one" || got.Worktree != pair.WorktreePath() {
		t.Errorf("pair entry = %+v; want it marked to push now, naming the pair", got)
	}

	// A resume records the landing again over a newer records commit, and the first stays the held one.
	gitkit.CommitFile(t, recordsDir, "resumed.txt", "resumed\n", "a records commit of a resume")
	if err := landLocally(pair, "one")(tip); err != nil {
		t.Fatalf("landLocally() again = %v; want nil", err)
	}
	if landings := f.entry(pushledger.LandingKey).Landings; len(landings) != 1 || landings[0].RecordsCommit != records {
		t.Errorf("landings after the second record = %+v; want one still holding %s", landings, records)
	}

	if _, err := pairteardown.PushRun(f.h.Location, pair, "one", pairteardown.RunPush{LockWait: fabriclayout.StatusPushLockWait, InsidePair: true}); err != nil {
		t.Fatalf("PushRun() = %v; want nil", err)
	}
	recordsRef := "refs/heads/" + fabriclayout.RecordsBranchName(f.branches["one"])
	if got, want := gitkit.RevParse(t, f.h.RecordsBare, recordsRef), gitkit.RevParse(t, recordsDir, records+"~1"); got != want {
		t.Errorf("origin records tip = %s; want %s, short of the status commit", got, want)
	}

	// The prime records no parent pair, so a landing recorded from it has nowhere to go.
	if err := landLocally(f.h.Location, "none")(tip); err == nil || !strings.Contains(err.Error(), "way forward:") {
		t.Errorf("landLocally() from the prime = %v; want a refusal naming a way forward", err)
	}
}

// TestWriteLedgerLanding_AChainOfOwnTipsReadsAsOwed lands three runs on the prime as a chain of landing commits.
// Each run's ledger landing carries its own landing commit, and a run recorded again keeps that tip.
func TestWriteLedgerLanding_AChainOfOwnTipsReadsAsOwed(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	f := newParentFixture(t)
	slugs := []string{"one", "two", "three"}
	tips, records := map[string]string{}, map[string]string{}
	for _, slug := range slugs {
		f.addRun(slug, "", false)
	}
	for _, slug := range slugs {
		tips[slug] = f.merge(slug, f.h.PrimeWorktree())
		records[slug] = gitkit.CommitFile(t, f.h.PairRecordsSibling(slug), "finalize.txt", slug+"\n", "finalize records")
	}
	target := runpusher.LandingTarget(f.h.Location)
	pairOne := f.location("one")
	aheadByOwed := parentAheadByOwed(pairOne)
	requireAheadByOwed := func(when string, want bool) {
		t.Helper()
		if got, err := aheadByOwed(); err != nil || got != want {
			t.Errorf("parentAheadByOwed() %s = %v, %v; want %v", when, got, err, want)
		}
	}
	for _, slug := range slugs {
		if err := writeLedgerLanding(target, f.branch, slug, tips[slug], records[slug]); err != nil {
			t.Fatalf("writeLedgerLanding(%s) = %v; want nil", slug, err)
		}
	}
	requireAheadByOwed("with every landing commit recorded", true)

	f.atFinalize("one")
	if err := landLocally(pairOne, "one")(tips["one"]); err != nil {
		t.Fatalf("landLocally(one) = %v; want nil", err)
	}
	requireAheadByOwed("after run one is recorded again", true)
	landings := f.entry(pushledger.LandingKey).Landings
	if len(landings) != len(slugs) {
		t.Fatalf("landings = %+v; want one per run", landings)
	}
	for _, landing := range landings {
		if landing.Tip != tips[landing.Run] || landing.RecordsCommit != records[landing.Run] || landing.ParentBranch != f.branch {
			t.Errorf("landing = %+v; want run %s at its own tip %s holding %s on %s", landing, landing.Run, tips[landing.Run], records[landing.Run], f.branch)
		}
	}
}
