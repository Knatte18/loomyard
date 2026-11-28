// finalize_queue_integration_test.go drives a queued Finalize over a hubforge hub with the production landing wiring.
// Each row replaces the gate command and the two agent session seams, and provokes failures through state.
// A pull-request row also replaces the GitHub API and the pair's origin.
// A row may also wrap a production seam to act as it is read.
// The row whose culprit's fix passes takes the test-file matcher of a recipe with no plan.

package loomcli

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/beamsetting"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/landingqueue"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/pullrequest"
	"github.com/Knatte18/loomyard/internal/pushledger"
	"github.com/Knatte18/loomyard/internal/runpusher"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/permkit"
	"github.com/Knatte18/loomyard/internal/testkit/prkit"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/verifytree"
	"github.com/Knatte18/loomyard/internal/websterengine"
	"github.com/Knatte18/loomyard/internal/yamlengine"
)

// queueHub is a parent fixture with one worked task pair per slug and the parent branch's real landing queue.
type queueHub struct {
	*parentFixture
	stencils string
	store    *landingqueue.Store
}

func newQueueHub(t *testing.T, slugs ...string) *queueHub {
	t.Helper()
	f := newParentFixture(t)
	for _, slug := range slugs {
		f.addRun(slug, "", false)
		f.work(slug)
	}
	return &queueHub{
		parentFixture: f,
		stencils:      stencilkit.Seed(t),
		store:         &landingqueue.Store{Dir: landingqueue.Dir(fabriclayout.BoltDir(f.h.Path)), Branch: f.branch},
	}
}

// deps returns the production landing deps of the run slug with gate as its gate command and idle session seams.
func (q *queueHub) deps(slug string, squash bool, gate string) landingshed.Deps {
	q.t.Helper()
	_, deps := q.finalize(slug, squash)
	deps.StencilsDir = q.stencils
	deps.Config.Fixer = "claude:test-model"
	deps.Config.FixerTimeoutMin = 1
	deps.Config.FinalizeFixAttempts = 1
	deps.VerifyCommand = func(string) (string, error) { return gate, nil }
	deps.TraceFailure = nil
	deps.Shuttle = &shedfake.MergeShuttle{}
	deps.FixerSeats = &spentFixerSeats{}
	return deps
}

func (q *queueHub) call(deps landingshed.Deps) (shedengine.Outcome, shedengine.OutputPointer, error) {
	q.t.Helper()
	fz, err := landingshed.NewFinalize(deps)
	if err != nil {
		q.t.Fatalf("NewFinalize() error = %v", err)
	}
	return fz.Call(context.Background())
}

func (q *queueHub) requireDone(deps landingshed.Deps) {
	q.t.Helper()
	if outcome, pointer, err := q.call(deps); err != nil || outcome != shedengine.Done {
		q.t.Fatalf("Call() = %q, %+v, %v; want Done", outcome, pointer, err)
	}
}

func (q *queueHub) taskHead(slug string) string {
	q.t.Helper()
	return gitkit.RevParse(q.t, q.h.PairCodeWorktree(slug), "HEAD")
}

// enqueue writes the entry of slug at its task head, with outcome and commit when outcome is set.
func (q *queueHub) enqueue(slug string, outcome landingqueue.Outcome, commit string) {
	q.t.Helper()
	entry := landingqueue.Entry{
		Slug: slug, Pair: q.h.PairCodeWorktree(slug), RunID: "earlier", PID: os.Getpid(),
		TaskHead: q.taskHead(slug), JoinedAt: time.Now().UTC(),
	}
	if err := q.store.Join(entry); err != nil {
		q.t.Fatalf("Join(%s): %v", slug, err)
	}
	if outcome == "" {
		return
	}
	if err := q.store.SetOutcome(slug, outcome, commit); err != nil {
		q.t.Fatalf("SetOutcome(%s): %v", slug, err)
	}
}

func (q *queueHub) queued(slug string) (landingqueue.Entry, bool) {
	q.t.Helper()
	entry, found, err := q.store.Read(slug)
	if err != nil {
		q.t.Fatalf("Read(%s): %v", slug, err)
	}
	return entry, found
}

// commitOnParent commits content to name on the parent's checkout and returns the commit.
func (q *queueHub) commitOnParent(name, content string) string {
	q.t.Helper()
	return gitkit.CommitFile(q.t, q.h.PrimeWorktree(), name, content, "parent: "+name)
}

func (q *queueHub) pushParent() {
	q.t.Helper()
	gitkit.Git(q.t, q.h.PrimeWorktree(), "push", "-q")
}

// recordingGate returns a gate command that appends the parent lock records of its run to the returned file.
func (q *queueHub) recordingGate() (command, record string) {
	record = filepath.Join(q.t.TempDir(), "gate.log")
	locks := filepath.ToSlash(parentlock.Dir(fabriclayout.BoltDir(q.h.Path)))
	return fmt.Sprintf("{ echo gate; cat '%s'/*.json; echo; } >> '%s'", locks, filepath.ToSlash(record)), record
}

// gateRuns returns what the recording gate wrote, one line "gate" per run.
func gateRuns(t *testing.T, record string) (runs int, log string) {
	t.Helper()
	data, err := os.ReadFile(record)
	if os.IsNotExist(err) {
		return 0, ""
	}
	if err != nil {
		t.Fatalf("read the gate record: %v", err)
	}
	return strings.Count(string(data), "gate\n"), string(data)
}

// lockHeldBy reports whether the parent lock record names slug.
func lockHeldBy(t *testing.T, deps landingshed.Deps, branch, slug string) bool {
	t.Helper()
	record, found, err := deps.Lock.Store.Read(branch)
	if err != nil {
		t.Fatalf("read the parent lock: %v", err)
	}
	return found && record.Owner.Slug == slug
}

// resolvingShuttle stands in for a conflict session that resolves name in the task worktree of slug.
// It records in underLock whether any session ran while the run held the parent lock.
func (q *queueHub) resolvingShuttle(deps landingshed.Deps, slug, name string, underLock *bool) *shedfake.MergeShuttle {
	return &shedfake.MergeShuttle{
		DuringRun: func(int, shuttleengine.Spec) {
			*underLock = *underLock || lockHeldBy(q.t, deps, q.branch, slug)
		},
		RunFn: func(shuttleengine.Spec) (shuttleengine.Result, error) {
			err := os.WriteFile(filepath.Join(q.h.PairCodeWorktree(slug), name), []byte("resolved\n"), 0o644)
			return shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}, err
		},
	}
}

// spentFixerSeats is a fixer seat runner that answers each turn as spent: it leaves no report and ends done.
type spentFixerSeats struct {
	runs  int
	onRun func()
}

func (s *spentFixerSeats) Run(seatengine.Table) (seatengine.Result, error) {
	s.runs++
	if s.onRun != nil {
		s.onRun()
	}
	return seatengine.Result{Chair: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}, nil
}

// fixingSeats is a fixer seat runner whose one turn runs fix, reports it fixed and has the table's gate judge it.
type fixingSeats struct {
	fix    func()
	report string
}

func (s fixingSeats) Run(table seatengine.Table) (seatengine.Result, error) {
	s.fix()
	if err := os.MkdirAll(filepath.Dir(s.report), 0o755); err != nil {
		return seatengine.Result{}, err
	}
	if err := os.WriteFile(s.report, []byte("verdict: fixed\n"), 0o644); err != nil {
		return seatengine.Result{}, err
	}
	_, err := table.Gate[0].Gate()
	return seatengine.Result{Chair: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}, err
}

// setPullRequestMode sets beam.pull_request of the run slug in its worktree's config layer.
func (q *queueHub) setPullRequestMode(slug, mode string) {
	q.t.Helper()
	pairs := []yamlengine.KV{{Key: dobbyengine.PullRequestKey, Value: mode}}
	if _, err := configengine.Set(fabricengine.ConfigDirs(q.location(slug)), dobbyengine.ConfigSpec(), configengine.LayerWorktree, pairs); err != nil {
		q.t.Errorf("configengine.Set(beam): %v", err)
	}
}

// recordFixerChange records the work commit of slug as a fixer's change of its current Finalize entry, not accepted.
// The row's change cap of zero lines puts it over the caps.
func (q *queueHub) recordFixerChange(slug string) {
	q.t.Helper()
	pair := q.location(slug)
	record := landingshed.FixerAttempts{Spent: 1, Baseline: gitkit.RevParse(q.t, pair.WorktreePath(), "HEAD^")}
	path := landingshed.FixerAttemptsPath(loomengine.LoomDurableDir(pair), loomshed.NameFinalize)
	if err := landingshed.WriteFixerAttempts(path, record); err != nil {
		q.t.Fatalf("WriteFixerAttempts(%s): %v", slug, err)
	}
}

// githubOrigin makes the pair of deps read as one with its origin on GitHub, whose API is api.
func githubOrigin(deps *landingshed.Deps, api *prkit.API) {
	deps.PullRequest.API = func(string) (pullrequest.API, error) { return api, nil }
	deps.PullRequest.Origin = func() (landingshed.Origin, error) {
		return landingshed.Origin{URL: "https://github.com/acme/widgets.git", Upstream: true}, nil
	}
}

// landedCommits records the commits handed to the landing seams of deps, passing each call on.
type landedCommits struct {
	recorded, landed []string
	empty            []bool
	sizes            int
}

func recordLandings(deps *landingshed.Deps) *landedCommits {
	seen := &landedCommits{}
	record, land, size := deps.RecordLanding, deps.LandLocally, deps.RecordLandedSize
	deps.RecordLanding = func(commit string, emptyDiff bool) error {
		seen.recorded = append(seen.recorded, commit)
		seen.empty = append(seen.empty, emptyDiff)
		return record(commit, emptyDiff)
	}
	deps.LandLocally = func(tip string) error {
		seen.landed = append(seen.landed, tip)
		return land(tip)
	}
	deps.RecordLandedSize = func(from, to string) error {
		seen.sizes++
		return size(from, to)
	}
	return seen
}

// TestFinalizeQueue_EntryAndOutcome covers which runs join the queue and what a run does with an outcome or a mark.
// Its session rows use the standing session fakes, shedfake.MergeShuttle and a fixer seat fake.
// That is a stated exception to PATTERN-test-stand-ins, since no recorded session stand-in exists.
func TestFinalizeQueue_EntryAndOutcome(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	t.Run("ATrailRunWritesNoEntryAndLandsThroughItsPullRequest", func(t *testing.T) {
		t.Parallel()
		q := newQueueHub(t, "one")
		q.setPullRequestMode("one", beamsetting.PullRequestTrail)
		deps := q.deps("one", true, "true")
		api := &prkit.API{}
		api.Script(prkit.MethodCreate, prkit.Answer{Ref: pullrequest.Ref{Number: 7, URL: "https://github.com/acme/widgets/pull/7"}})
		githubOrigin(&deps, api)
		queuedAtLanding := true
		land := deps.LandLocally
		deps.LandLocally = func(tip string) error {
			_, queuedAtLanding = q.queued("one")
			return land(tip)
		}

		q.requireDone(deps)

		if queuedAtLanding || len(api.Calls(prkit.MethodMerge)) != 1 {
			t.Errorf("entry at the landing = %v, GitHub merges = %v; want no entry and one merge", queuedAtLanding, api.Calls(prkit.MethodMerge))
		}
	})

	t.Run("AFailedModeReadWritesNoEntryAndHalts", func(t *testing.T) {
		t.Parallel()
		q := newQueueHub(t, "one")
		q.setPullRequestMode("one", beamsetting.PullRequestTrail)
		deps := q.deps("one", true, "true")
		permkit.Chmod(t, configengine.ConfigFile(q.location("one").AnchorPath(), beamsetting.Module), 0)

		outcome, pointer, err := q.call(deps)

		if err != nil || outcome != shedengine.Stuck || !strings.HasPrefix(pointer.Reason, landingshed.LandingStopPrefix) {
			t.Fatalf("Call() = %q, %+v, %v; want a landing stop", outcome, pointer, err)
		}
		if _, found := q.queued("one"); found {
			t.Error("an entry stands; want none")
		}
	})

	for _, tt := range []struct {
		name      string
		squash    bool
		mode      string
		trueMerge bool
	}{
		{name: "ASquashingRunsEntry", squash: true},
		{name: "ANonSquashingRunsEntry", trueMerge: true},
		{name: "AMergeInRunsEntry", squash: true, mode: beamsetting.PullRequestMergeIn, trueMerge: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			q := newQueueHub(t, "one")
			if tt.mode != "" {
				q.setPullRequestMode("one", tt.mode)
			}
			deps := q.deps("one", tt.squash, "true")
			head := q.taskHead("one")
			var entry landingqueue.Entry
			var records string
			record := deps.RecordLanding
			deps.RecordLanding = func(commit string, emptyDiff bool) error {
				entry, _ = q.queued("one")
				records = gitkit.RevParse(t, q.h.PairRecordsSibling("one"), "HEAD")
				return record(commit, emptyDiff)
			}
			start := time.Now().UTC()

			q.requireDone(deps)

			const message = "Land one\n\nThe work of one.\n\n" + fabriclayout.LandingTrailerKey + ": one"
			if entry.Slug != "one" || entry.TaskHead != head || entry.RecordsHead != records || entry.Message != message || entry.TrueMerge != tt.trueMerge {
				t.Errorf("entry = %+v; want slug one at task head %s, records %s, message %q, TrueMerge %v", entry, head, records, message, tt.trueMerge)
			}
			if entry.JoinedAt.Before(start) || entry.JoinedAt.After(time.Now().UTC()) {
				t.Errorf("entry joined at %v; want a time during the call", entry.JoinedAt)
			}
			parents := strings.Fields(gitkit.Git(t, q.h.PrimeWorktree(), "log", "-1", "--format=%P"))
			if (len(parents) == 2) != tt.trueMerge {
				t.Errorf("landing parents = %v; want two only for a true merge (%v)", parents, tt.trueMerge)
			}
		})
	}

	t.Run("AQueueDirectoryThatIsAFileFailsTheCallBeforeAnyMerge", func(t *testing.T) {
		t.Parallel()
		q := newQueueHub(t, "one")
		deps := q.deps("one", true, "true")
		if err := os.MkdirAll(filepath.Dir(q.store.Dir), 0o755); err != nil {
			t.Fatalf("create the queue's parent: %v", err)
		}
		if err := os.WriteFile(q.store.Dir, []byte("not a directory\n"), 0o644); err != nil {
			t.Fatalf("write the queue directory as a file: %v", err)
		}
		before := q.primeHead()

		outcome, _, err := q.call(deps)

		if outcome != "" || err == nil || !strings.Contains(err.Error(), "landing queue") {
			t.Fatalf("Call() = %q, %v; want an error naming the landing queue", outcome, err)
		}
		if q.primeHead() != before {
			t.Errorf("prime head = %s; want it unmoved at %s", q.primeHead(), before)
		}
	})

	t.Run("AFailedEntryGatesAloneUnderTheLockAndKeepsItsOutcomeWhileTheFixerRuns", func(t *testing.T) {
		t.Parallel()
		q := newQueueHub(t, "one")
		q.enqueue("one", landingqueue.OutcomeFailed, q.primeHead())
		gate, record := q.recordingGate()
		deps := q.deps("one", true, gate+"; false")
		var during landingqueue.Outcome
		seats := &spentFixerSeats{onRun: func() {
			entry, _ := q.queued("one")
			during = entry.Outcome
		}}
		deps.FixerSeats = seats

		outcome, pointer, err := q.call(deps)

		if err != nil || outcome != shedengine.Stuck || !strings.HasPrefix(pointer.Reason, landingshed.LandingStopPrefix) {
			t.Fatalf("Call() = %q, %+v, %v; want a landing stop", outcome, pointer, err)
		}
		runs, log := gateRuns(t, record)
		if runs == 0 || !strings.Contains(log, `"one"`) {
			t.Errorf("gate runs = %d, log %q; want the gate run under the run's parent lock", runs, log)
		}
		if seats.runs != 1 || during != landingqueue.OutcomeFailed {
			t.Errorf("fixer runs = %d, outcome during it = %q; want one over a failed entry", seats.runs, during)
		}
	})

	t.Run("AConflictEntryRunsItsSessionUnlockedAndLandsAlone", func(t *testing.T) {
		t.Parallel()
		q := newQueueHub(t, "one")
		q.commitOnParent("one.txt", "parent\n")
		q.pushParent()
		q.enqueue("one", landingqueue.OutcomeConflict, "")
		deps := q.deps("one", true, "true")
		underLock := false
		shuttle := q.resolvingShuttle(deps, "one", "one.txt", &underLock)
		deps.Shuttle = shuttle

		q.requireDone(deps)

		if len(shuttle.Specs) != 1 || underLock {
			t.Errorf("sessions = %d, under the lock = %v; want one with no lock held", len(shuttle.Specs), underLock)
		}
		if got := gitkit.Git(t, q.h.PrimeWorktree(), "show", "HEAD:one.txt"); got != "resolved" {
			t.Errorf("parent one.txt = %q; want the resolved content landed", got)
		}
	})

	for name, outcome := range map[string]landingqueue.Outcome{
		"AMergeUnderTheLockThatNeedsASessionFreesTheLockFirst":             "",
		"AFailedEntrysMergeUnderTheLockThatNeedsASessionFreesTheLockFirst": landingqueue.OutcomeFailed,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			q := newQueueHub(t, "one")
			if outcome != "" {
				q.enqueue("one", outcome, q.primeHead())
			}
			deps := q.deps("one", true, "true")
			// The parent gains a conflicting commit between the unlocked merge and the one under the lock.
			beforeSync(&deps, func() {
				q.commitOnParent("one.txt", "parent\n")
				q.pushParent()
			})
			underLock := false
			shuttle := q.resolvingShuttle(deps, "one", "one.txt", &underLock)
			deps.Shuttle = shuttle
			// Another run takes the freed lock during the session, so the run has to wait for it again.
			self := proc.SelfRecord()
			other := parentlock.Owner{Slug: "other", Pair: t.TempDir(), RunID: "other", PID: self.PID, StartTime: self.StartTime}
			during := shuttle.DuringRun
			shuttle.DuringRun = func(call int, spec shuttleengine.Spec) {
				during(call, spec)
				if attempt, err := deps.Lock.Store.TryAcquire(q.branch, other); err != nil || !attempt.Granted {
					t.Errorf("TryAcquire() = %+v, %v; want the freed lock granted", attempt, err)
				}
			}
			var front []bool
			paused := deps.Lock.PauseRequested
			deps.Lock.PauseRequested = func() (bool, error) {
				locks, err := deps.Lock.Store.List()
				if err != nil {
					t.Errorf("List() = %v", err)
				}
				for _, held := range locks {
					for _, ticket := range held.Waiters {
						front = append(front, ticket.Front)
					}
				}
				if _, err := deps.Lock.Store.Release(q.branch, other); err != nil {
					t.Errorf("Release() = %v", err)
				}
				return paused()
			}

			q.requireDone(deps)

			if len(shuttle.Specs) != 1 || underLock {
				t.Errorf("sessions = %d, under the lock = %v; want one with no lock held", len(shuttle.Specs), underLock)
			}
			if len(front) == 0 || slices.Contains(front, false) {
				t.Errorf("front flags of the retake's tickets = %v; want a ticket at the front", front)
			}
		})
	}

	t.Run("ARunWithAFixerChangeLeadsNoBatch", func(t *testing.T) {
		t.Parallel()
		q := newQueueHub(t, "one", "two")
		q.join("two", true)
		deps := q.leader("one", 5, "true")
		q.recordFixerChange("one")

		outcome, pointer, err := q.call(deps)

		if err != nil || outcome != shedengine.Stuck || !strings.HasPrefix(pointer.Reason, landingshed.LandingStopPrefix) {
			t.Fatalf("Call() = %q, %+v, %v; want a landing stop", outcome, pointer, err)
		}
		if q.parentHolds("one.txt") || q.parentHolds("two.txt") {
			t.Errorf("parent holds one = %v, two = %v; want neither landed", q.parentHolds("one.txt"), q.parentHolds("two.txt"))
		}
		// The fix review runs with the lock freed, so the run's own entry is kept out of batches first.
		q.requireOutcome("one", landingqueue.OutcomeFailed)
		q.requireOutcome("two", "")
	})

	t.Run("AnotherLeaderLeavesARunWhoseOwnFixerWorks", func(t *testing.T) {
		t.Parallel()
		q := newQueueHub(t, "one", "two")
		deps := q.leader("one", 5, "false")
		deps.FixerSeats = &finalizingSeats{first: func() { q.requireDone(q.leader("two", 5, "true")) }}

		if outcome, pointer, err := q.call(deps); err != nil || outcome != shedengine.Stuck {
			t.Fatalf("Call() = %q, %+v, %v; want Stuck", outcome, pointer, err)
		}

		if q.parentHolds("one.txt") || !q.parentHolds("two.txt") {
			t.Errorf("parent holds one = %v, two = %v; want two alone", q.parentHolds("one.txt"), q.parentHolds("two.txt"))
		}
		q.requireOutcome("one", landingqueue.OutcomeFailed)
	})

	for _, tt := range []struct {
		name string
		// arrange changes what the parent's upstream or the mark's commit reads as.
		arrange      func(q *queueHub, commit *string)
		pushSkipped  bool
		wantLandings int
	}{
		{name: "ABackstopOverAParentWithNoUpstreamWritesTheLanding", arrange: func(q *queueHub, _ *string) {
			gitkit.Git(q.t, q.h.PrimeWorktree(), "branch", "--unset-upstream")
		}, wantLandings: 1},
		{name: "ABackstopOverAPrunedTrackingRefWritesNone", arrange: func(q *queueHub, _ *string) {
			gitkit.Git(q.t, q.h.PrimeWorktree(), "update-ref", "-d", "refs/remotes/origin/"+q.branch)
		}},
		{name: "ABackstopOverACommitTheRepositoryLacksWritesNone", arrange: func(_ *queueHub, commit *string) {
			*commit = strings.Repeat("1", 40)
		}},
		{name: "ABackstopWithPushesSkippedWritesNone", pushSkipped: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			q := newQueueHub(t, "one")
			commit := q.commitOnParent("moved.txt", "moved\n")
			if tt.arrange != nil {
				tt.arrange(q, &commit)
			}
			mark := landingqueue.MoveMark{Members: []landingqueue.MarkMember{{Slug: "other", Commit: commit, Records: commit}}}
			if err := q.store.WriteMark(mark); err != nil {
				t.Fatalf("WriteMark: %v", err)
			}
			pair := q.location("one")
			lock := parentLockDeps(pair, shedrun.SelfRunID, shedrun.StatusFile(pair, shedrun.SelfRunID), shedrun.StatusLock(pair, shedrun.SelfRunID))
			deps := landingDeps(pair, shedrun.RecipeTapestry, websterengine.Geometry{}, q.branches["one"], q.branch, tt.pushSkipped,
				modelspec.Registry{}, &shuttleengine.Runner{}, landingshed.Config{}, "", nil, nil, nil, planVerifySource(pair), lock)

			_, err := deps.Queue.Backstop(deps.Backstop)

			if err != nil {
				t.Fatalf("Backstop() error = %v", err)
			}
			if landings := q.entry(pushledger.LandingKey).Landings; len(landings) != tt.wantLandings {
				t.Errorf("landings = %+v; want %d", landings, tt.wantLandings)
			}
			if _, found, err := q.store.ReadMark(); err != nil || found {
				t.Errorf("move mark stands = %v, %v; want it removed", found, err)
			}
		})
	}

	t.Run("ABackstopThatCannotWriteALandingEndsTheCallBeforeAnyMergeUnderTheLock", func(t *testing.T) {
		t.Parallel()
		q := newQueueHub(t, "one")
		moved := q.commitOnParent("other.txt", "other\n")
		mark := landingqueue.MoveMark{Members: []landingqueue.MarkMember{{Slug: "other", Commit: moved, Records: moved}}}
		if err := q.store.WriteMark(mark); err != nil {
			t.Fatalf("WriteMark: %v", err)
		}
		gate, record := q.recordingGate()
		deps := q.deps("one", true, gate)
		entry := pushledger.EntryPath(q.h.Path, pushledger.LandingKey)
		ledger := filepath.Dir(entry)
		if err := os.MkdirAll(ledger, 0o755); err != nil {
			t.Fatalf("create the ledger: %v", err)
		}
		// With its lock files present the ledger still reads, and no entry can be written into it.
		for _, lockFile := range []string{entry + ".lock", filepath.Join(ledger, "ledger.lock")} {
			if err := os.WriteFile(lockFile, nil, 0o644); err != nil {
				t.Fatalf("create the ledger lock: %v", err)
			}
		}
		permkit.Chmod(t, ledger, 0o500)
		head := q.taskHead("one")

		outcome, _, err := q.call(deps)

		if outcome != "" || err == nil || !strings.Contains(err.Error(), "write the landing of other") {
			t.Fatalf("Call() = %q, %v; want the backstop's write error", outcome, err)
		}
		runs, _ := gateRuns(t, record)
		if q.primeHead() != moved || q.taskHead("one") != head || runs != 0 {
			t.Errorf("prime head = %s, task head = %s, gate runs = %d; want %s, %s and no gate", q.primeHead(), q.taskHead("one"), runs, moved, head)
		}
	})

	// A parent with no upstream has its tip read under the parent lock, before the unlocked merge.
	unsetUpstream := func(q *queueHub) { gitkit.Git(q.t, q.h.PrimeWorktree(), "branch", "--unset-upstream") }

	t.Run("TheLockedTipReadFinishesAStandingMarkBeforeItReadsTheTip", func(t *testing.T) {
		t.Parallel()
		q := newQueueHub(t, "one")
		unsetUpstream(q)
		mark := landingqueue.MoveMark{}
		for _, slug := range []string{"two", "three"} {
			commit := q.commitOnParent(slug+".txt", slug+"\n")
			mark.Members = append(mark.Members, landingqueue.MarkMember{Slug: slug, Commit: commit, Records: commit})
		}
		if err := q.store.WriteMark(mark); err != nil {
			t.Fatalf("WriteMark: %v", err)
		}
		deps := q.deps("one", true, "true")
		read := deps.ReadParentTips
		var atFirstTip []pushledger.Landing
		reads := 0
		deps.ReadParentTips = func(fetch bool) (fabriclayout.ParentTips, error) {
			if !fetch {
				if reads++; reads == 1 {
					atFirstTip = q.entry(pushledger.LandingKey).Landings
				}
			}
			return read(fetch)
		}

		q.requireDone(deps)

		if len(atFirstTip) != 2 || atFirstTip[0].Tip != mark.Members[0].Commit || atFirstTip[1].Tip != mark.Members[1].Commit {
			t.Errorf("landings at the first tip read = %+v; want one per marked commit %+v", atFirstTip, mark.Members)
		}
	})

	t.Run("ABackstopThatMarksTheRunLandedIsFollowedByNoMerge", func(t *testing.T) {
		t.Parallel()
		q := newQueueHub(t, "one")
		unsetUpstream(q)
		moved := q.commitOnParent("moved.txt", "moved\n")
		q.enqueue("one", "", "")
		records := gitkit.RevParse(t, q.h.PairRecordsSibling("one"), "HEAD")
		mark := landingqueue.MoveMark{Members: []landingqueue.MarkMember{{Slug: "one", Commit: moved, TaskHead: q.taskHead("one"), Records: records}}}
		if err := q.store.WriteMark(mark); err != nil {
			t.Fatalf("WriteMark: %v", err)
		}
		deps := q.deps("one", true, "true")
		head := q.taskHead("one")

		q.requireDone(deps)

		if q.taskHead("one") != head || q.primeHead() != moved {
			t.Errorf("task head = %s, prime head = %s; want %s and %s with nothing merged", q.taskHead("one"), q.primeHead(), head, moved)
		}
	})

	t.Run("AFailedEntryRemovalKeepsDone", func(t *testing.T) {
		t.Parallel()
		q := newQueueHub(t, "one")
		deps := q.deps("one", true, "true")
		land := deps.LandLocally
		deps.LandLocally = func(tip string) error {
			permkit.Chmod(t, filepath.Join(q.store.Dir, hex.EncodeToString([]byte(q.branch))), 0o500)
			return land(tip)
		}

		q.requireDone(deps)

		if _, found := q.queued("one"); !found {
			t.Error("the entry is gone; want it left by the failed removal")
		}
	})
}

// TestFinalizeQueue_LandsOneRunAlone lands a single queued run with no other entry.
func TestFinalizeQueue_LandsOneRunAlone(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	q := newQueueHub(t, "one")
	parent := q.commitOnParent("parent.txt", "parent\n")
	q.pushParent()
	gate, record := q.recordingGate()
	deps := q.deps("one", true, gate)
	probe := deps.MergeState
	var heldAtMerge []bool
	deps.MergeState = func() (fabriclayout.MidMergeState, error) {
		heldAtMerge = append(heldAtMerge, lockHeldBy(t, deps, q.branch, "one"))
		return probe()
	}

	q.requireDone(deps)

	if !slices.Equal(heldAtMerge, []bool{false, true}) {
		t.Errorf("lock held at each merge-in = %v; want the first unlocked and the second locked", heldAtMerge)
	}
	runs, log := gateRuns(t, record)
	if runs != 1 || !strings.Contains(log, `"one"`) {
		t.Errorf("gate runs = %d, log %q; want one under the run's parent lock", runs, log)
	}
	if got := gitkit.RevParse(t, q.h.PrimeWorktree(), "HEAD^1"); got != parent {
		t.Errorf("landing's first parent = %s; want %s", got, parent)
	}
	if _, found := q.queued("one"); found {
		t.Error("an entry stands after Done; want none")
	}
}

// TestFinalizeQueue_ResumedDetached resumes Finalize over a task worktree left detached at a chain commit.
// A landing that stopped during its gate leaves the worktree that way.
func TestFinalizeQueue_ResumedDetached(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	for _, tt := range []struct {
		name          string
		marked, dirty bool
	}{
		{"ADirtyWorktreeIsStuckNamingItsPaths", true, true},
		{"AWorktreeWithNoMarkerIsStuckNamingItsCommit", false, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			q := newQueueHub(t, "one")
			pair := q.location("one")
			deps := q.deps("one", true, "true")
			handle, err := fabricengine.Open(pair)
			if err != nil {
				t.Fatalf("fabricengine.Open: %v", err)
			}
			head, parent := q.taskHead("one"), q.primeHead()
			chain, err := handle.BuildLandingChain(parent, []fabricengine.ChainLink{{Slug: "one", TaskHead: head, Message: "land one"}})
			if err != nil || len(chain.Commits) != 1 {
				t.Fatalf("BuildLandingChain() = %+v, %v; want one commit", chain, err)
			}
			commit := chain.Commits[0].Commit
			if err := handle.CheckoutDetached(commit); err != nil {
				t.Fatalf("CheckoutDetached: %v", err)
			}
			want := commit
			if tt.marked {
				if err := fabricengine.WriteDetachMarker(pair, q.branches["one"]); err != nil {
					t.Fatalf("WriteDetachMarker: %v", err)
				}
			}
			if tt.dirty {
				if err := os.WriteFile(filepath.Join(pair.WorktreePath(), "stray.txt"), []byte("stray\n"), 0o644); err != nil {
					t.Fatalf("write a stray file: %v", err)
				}
				want = "stray.txt"
			}

			outcome, pointer, err := q.call(deps)

			if err != nil || outcome != shedengine.Stuck || !strings.Contains(pointer.Reason, want) {
				t.Fatalf("Call() = %q, %+v, %v; want Stuck naming %s", outcome, pointer, err, want)
			}
			if got := gitkit.RevParse(t, pair.WorktreePath(), "refs/heads/"+q.branches["one"]); got != head || q.primeHead() != parent {
				t.Errorf("task branch = %s, parent head = %s; want %s and %s unchanged", got, q.primeHead(), head, parent)
			}
		})
	}
}

// join writes the entry of slug at its current heads as its own Finalize would, owned by this process.
func (q *queueHub) join(slug string, squash bool) landingqueue.Entry {
	q.t.Helper()
	q.atFinalize(slug)
	records := q.h.PairRecordsSibling(slug)
	gitkit.Git(q.t, records, "add", "-A")
	gitkit.Git(q.t, records, "commit", "-q", "--allow-empty", "-m", "records of "+slug)
	self := proc.SelfRecord()
	entry := landingqueue.Entry{
		Slug: slug, Pair: q.h.PairCodeWorktree(slug), RunID: "waiting", PID: self.PID, StartTime: self.StartTime,
		TaskHead: q.taskHead(slug), RecordsHead: gitkit.RevParse(q.t, records, "HEAD"),
		Message:   "Land " + slug + "\n\nThe work of " + slug + ".\n\n" + fabriclayout.LandingTrailerKey + ": " + slug,
		TrueMerge: !squash, JoinedAt: time.Now().UTC(),
	}
	if err := q.store.Join(entry); err != nil {
		q.t.Fatalf("Join(%s): %v", slug, err)
	}
	return entry
}

// leader returns the landing deps of the run slug whose batch takes at most batchMax entries.
func (q *queueHub) leader(slug string, batchMax int, gate string) landingshed.Deps {
	q.t.Helper()
	deps := q.deps(slug, true, gate)
	deps.Config.BatchMax = batchMax
	return deps
}

// headGate returns a gate command that appends to the returned file the commit it ran over.
// Each line also names the branch, or "detached".
// A non-empty before runs ahead of the record.
func (q *queueHub) headGate(before string) (command, record string) {
	record = filepath.Join(q.t.TempDir(), "gate.log")
	command = fmt.Sprintf(`echo "$(git rev-parse HEAD) $(git symbolic-ref -q HEAD || echo detached)" >> '%s'`, filepath.ToSlash(record))
	if before != "" {
		command = before + "; " + command
	}
	return command, record
}

// recordedLines returns the lines of a gate record, none for an absent one.
func recordedLines(t *testing.T, record string) []string {
	t.Helper()
	data, err := os.ReadFile(record)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("read the gate record: %v", err)
	}
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

// detachedRuns counts the gate runs that ran over a detached HEAD.
func detachedRuns(lines []string) int {
	count := 0
	for _, line := range lines {
		if strings.HasSuffix(line, " detached") {
			count++
		}
	}
	return count
}

func (q *queueHub) rev(ref string) string {
	q.t.Helper()
	return gitkit.RevParse(q.t, q.h.PrimeWorktree(), ref)
}

// parentHolds reports whether the parent's head holds the file name.
func (q *queueHub) parentHolds(name string) bool {
	q.t.Helper()
	return gitkit.Git(q.t, q.h.PrimeWorktree(), "ls-tree", "--name-only", "HEAD", "--", name) != ""
}

// requireOnBranch fails unless the pair of slug is back on its branch at head, clean and with no detach marker.
func (q *queueHub) requireOnBranch(slug, head string) {
	q.t.Helper()
	pair := q.location(slug)
	state, err := fabricengine.ReadDetachState(pair)
	if err != nil {
		q.t.Fatalf("ReadDetachState(%s): %v", slug, err)
	}
	if state.Detached || state.Marker || len(state.Dirty) != 0 {
		q.t.Errorf("worktree of %s = %+v; want it on its branch, clean and unmarked", slug, state)
	}
	if got := gitkit.RevParse(q.t, pair.WorktreePath(), "refs/heads/"+q.branches[slug]); got != head {
		q.t.Errorf("task branch of %s = %s; want it unchanged at %s", slug, got, head)
	}
}

func (q *queueHub) requireOutcome(slug string, want landingqueue.Outcome) {
	q.t.Helper()
	if entry, found := q.queued(slug); !found || entry.Outcome != want {
		q.t.Errorf("entry of %s = %+v, %v; want outcome %q", slug, entry, found, want)
	}
}

// landingsByRun returns the ledger landings of the parent by run.
func (q *queueHub) landingsByRun() map[string]pushledger.Landing {
	q.t.Helper()
	landings := map[string]pushledger.Landing{}
	for _, landing := range q.entry(pushledger.LandingKey).Landings {
		landings[landing.Run] = landing
	}
	return landings
}

// onRestore runs act once, right after the first gate of a batch put the worktree back on its branch.
func onRestore(deps *landingshed.Deps, act func()) {
	restore, done := deps.Detach.Restore, false
	deps.Detach.Restore = func() error {
		err := restore()
		if !done {
			done = true
			act()
		}
		return err
	}
}

// amendMember changes the seams a batch builds for the run slug.
func amendMember(deps *landingshed.Deps, slug string, amend func(*landingshed.Deps)) {
	build := deps.MemberDeps
	deps.MemberDeps = func(pair string) (landingshed.Deps, error) {
		member, err := build(pair)
		if err == nil && member.Slug == slug {
			amend(&member)
		}
		return member, err
	}
}

// pushUpstream commits content to name on the parent branch of origin alone, from another clone.
// It returns the function that pushes that commit.
func (q *queueHub) pushUpstream(name, content string) func() {
	q.t.Helper()
	clone := q.clone(q.h.CodeBare, q.branch)
	gitkit.CommitFile(q.t, clone, name, content, "upstream: "+name)
	return func() { gitkit.Git(q.t, clone, "push", "-q") }
}

// beforeSync runs act before the second fetching read of the parent tips, the one the locked sync makes.
func beforeSync(deps *landingshed.Deps, act func()) {
	read, fetches := deps.ReadParentTips, 0
	deps.ReadParentTips = func(fetch bool) (fabriclayout.ParentTips, error) {
		if fetch {
			if fetches++; fetches == 2 {
				act()
			}
		}
		return read(fetch)
	}
}

// TestFinalizeQueue_LandsABatch covers which runs a leader's batch takes and how it gates and lands them.
// Its conflicting fast-forward row runs the conflict session on shedfake.MergeShuttle.
// Its AFailedBatchGateLeavesTheWorktreeOnItsUnmovedBranch row runs a fixer turn on spentFixerSeats.
// Those are a stated exception to PATTERN-test-stand-ins, since no recorded session stand-in exists.
func TestFinalizeQueue_LandsABatch(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	t.Run("ThreeRunsLandWithOneGateRunOverTheDetachedChainCommit", func(t *testing.T) {
		t.Parallel()
		q := newQueueHub(t, "one", "two", "three")
		q.join("two", true)
		q.join("three", true)
		gate, record := q.headGate("")
		deps := q.leader("one", 5, gate)
		head := q.taskHead("one")
		var holders []string
		write := deps.WriteLedgerLanding
		deps.WriteLedgerLanding = func(slug, tip, records string) error {
			holder, _, err := deps.Lock.Store.Read(q.branch)
			if err != nil {
				t.Errorf("read the parent lock: %v", err)
			}
			holders = append(holders, holder.Owner.Slug)
			return write(slug, tip, records)
		}

		q.requireDone(deps)

		if want := []string{q.rev("HEAD") + " detached"}; !slices.Equal(recordedLines(t, record), want) {
			t.Errorf("gate runs = %q; want %q", recordedLines(t, record), want)
		}
		landings := q.landingsByRun()
		for slug, ref := range map[string]string{"one": "HEAD~2", "two": "HEAD~1", "three": "HEAD"} {
			if landings[slug].Tip != q.rev(ref) {
				t.Errorf("landing of %s = %+v; want its own commit %s", slug, landings[slug], q.rev(ref))
			}
		}
		if !slices.Equal(holders, []string{"one", "one", "one"}) {
			t.Errorf("lock holders at the ledger writes = %v; want the leader at all three", holders)
		}
		q.requireOnBranch("one", head)
		q.requireOutcome("two", landingqueue.OutcomeLanded)
	})

	t.Run("AFailedBatchGateLeavesTheWorktreeOnItsUnmovedBranch", func(t *testing.T) {
		t.Parallel()
		q := newQueueHub(t, "one", "two")
		q.join("two", true)
		deps := q.leader("one", 5, "git symbolic-ref -q HEAD >/dev/null")
		head := q.taskHead("one")

		if outcome, pointer, err := q.call(deps); err != nil || outcome != shedengine.Stuck {
			t.Fatalf("Call() = %q, %+v, %v; want Stuck", outcome, pointer, err)
		}

		q.requireOnBranch("one", head)
		q.requireOutcome("two", "")
	})

	t.Run("AMemberDeletingADirectoryWithIgnoredOutputGatesAndRestoresClean", func(t *testing.T) {
		t.Parallel()
		q := newQueueHub(t)
		q.commitOnParent(".gitignore", "*.out\n")
		if err := os.MkdirAll(filepath.Join(q.h.PrimeWorktree(), "gen"), 0o755); err != nil {
			t.Fatalf("create gen: %v", err)
		}
		q.commitOnParent("gen/keep.txt", "keep\n")
		q.pushParent()
		for _, slug := range []string{"one", "two"} {
			q.addRun(slug, "", false)
			q.work(slug)
		}
		two := q.h.PairCodeWorktree("two")
		gitkit.Git(t, two, "rm", "-q", "-r", "gen")
		gitkit.Git(t, two, "commit", "-q", "-m", "delete gen")
		q.join("two", true)
		output := filepath.Join(q.h.PairCodeWorktree("one"), "gen", "build.out")
		if err := os.WriteFile(output, []byte("built\n"), 0o644); err != nil {
			t.Fatalf("write the build output: %v", err)
		}
		gate, record := q.headGate("test ! -e gen")
		deps := q.leader("one", 5, gate)
		head := q.taskHead("one")

		q.requireDone(deps)

		if got := detachedRuns(recordedLines(t, record)); got != 1 || q.parentHolds("gen/keep.txt") {
			t.Errorf("batch gate runs = %d, parent holds gen = %v; want one run and gen deleted", got, q.parentHolds("gen/keep.txt"))
		}
		q.requireOnBranch("one", head)
	})

	// commitToParent is a shell step that commits on the parent checkout, as another writer of it would.
	commitToParent := func(q *queueHub) string {
		prime := filepath.ToSlash(q.h.PrimeWorktree())
		return fmt.Sprintf("echo x >> '%[1]s/moved.txt' && git -C '%[1]s' add moved.txt && git -C '%[1]s' commit -q -m moved", prime)
	}

	t.Run("AParentMovedDuringTheGateIsRebuiltOnAndGatedAgain", func(t *testing.T) {
		t.Parallel()
		q := newQueueHub(t, "one", "two")
		q.join("two", true)
		flag := filepath.ToSlash(filepath.Join(t.TempDir(), "moved"))
		gate, record := q.headGate(fmt.Sprintf("if [ ! -e '%[1]s' ]; then touch '%[1]s' && %[2]s; fi", flag, commitToParent(q)))
		deps := q.leader("one", 5, gate)

		q.requireDone(deps)

		if got := detachedRuns(recordedLines(t, record)); got != 2 {
			t.Errorf("batch gate runs = %d; want 2", got)
		}
		if got := gitkit.Git(t, q.h.PrimeWorktree(), "log", "-1", "--format=%s", "HEAD~2"); got != "moved" {
			t.Errorf("subject under the landings = %q; want the commit that moved the parent", got)
		}
		q.requireOutcome("two", landingqueue.OutcomeLanded)
	})

	t.Run("AParentMovedAtEveryBatchGateLandsTheLeaderAloneAfterTheRebuilds", func(t *testing.T) {
		t.Parallel()
		q := newQueueHub(t, "one", "two")
		q.join("two", true)
		gate, record := q.headGate("if ! git symbolic-ref -q HEAD >/dev/null; then " + commitToParent(q) + "; fi")
		deps := q.leader("one", 2, gate)

		q.requireDone(deps)

		lines := recordedLines(t, record)
		if detachedRuns(lines) != 3 || len(lines) != 4 {
			t.Errorf("gate runs = %q; want the batch gated three times and then the leader's own gate", lines)
		}
		if !q.parentHolds("one.txt") || q.parentHolds("two.txt") {
			t.Errorf("parent holds one = %v, two = %v; want the leader alone", q.parentHolds("one.txt"), q.parentHolds("two.txt"))
		}
		q.requireOutcome("two", "")
	})

	t.Run("WithoutSquashEachLandingCommitKeepsItsTaskHead", func(t *testing.T) {
		t.Parallel()
		q := newQueueHub(t, "one", "two")
		q.join("two", false)
		deps := q.deps("one", false, "true")
		deps.Config.BatchMax = 5
		heads := map[string]string{"HEAD~1": q.taskHead("one"), "HEAD": q.taskHead("two")}

		q.requireDone(deps)

		for commit, head := range heads {
			parents := strings.Fields(gitkit.Git(t, q.h.PrimeWorktree(), "log", "-1", "--format=%P", commit))
			if len(parents) != 2 || parents[1] != head {
				t.Errorf("parents of %s = %v; want two, the second %s", commit, parents, head)
			}
		}
	})

	t.Run("EachRunRecordsItsOwnLandingCommit", func(t *testing.T) {
		t.Parallel()
		q := newQueueHub(t, "one", "two")
		q.join("two", true)
		deps := q.leader("one", 5, "true")
		seen := recordLandings(&deps)

		q.requireDone(deps)

		first, second := q.rev("HEAD~1"), q.rev("HEAD")
		if !slices.Equal(seen.recorded, []string{first}) || !slices.Equal(seen.landed, []string{first}) {
			t.Errorf("leader recorded = %v, landed = %v; want %s", seen.recorded, seen.landed, first)
		}

		member := q.deps("two", true, "true")
		memberSeen := recordLandings(&member)
		var from string
		size := member.RecordLandedSize
		member.RecordLandedSize = func(base, landing string) error {
			from = q.rev(base)
			return size(base, landing)
		}

		q.requireDone(member)

		if !slices.Equal(memberSeen.recorded, []string{second}) || !slices.Equal(memberSeen.landed, []string{second}) || from != first {
			t.Errorf("member recorded = %v, landed = %v, size from %s; want %s measured from %s", memberSeen.recorded, memberSeen.landed, from, second, first)
		}
	})

	t.Run("AMemberWhoseRecordsToHoldFailsHoldsItsEntrysRecordsHead", func(t *testing.T) {
		t.Parallel()
		q := newQueueHub(t, "one", "two")
		entry := q.join("two", true)
		gitkit.CommitFile(t, q.h.PairRecordsSibling("two"), "later.txt", "later\n", "records after the join")
		standing := pushledger.Landing{Run: "other", Tip: q.primeHead(), ParentBranch: q.branch, At: time.Now()}
		if err := runpusher.RecordLanding(runpusher.LandingTarget(q.h.Location), standing, time.Now()); err != nil {
			t.Fatalf("RecordLanding: %v", err)
		}
		ledger := pushledger.EntryPath(q.h.Path, pushledger.LandingKey)
		deps := q.leader("one", 5, "true")
		onRestore(&deps, func() { permkit.Chmod(t, ledger, 0) })
		var marked landingqueue.MoveMark
		write, writes := deps.WriteLedgerLanding, 0
		deps.WriteLedgerLanding = func(slug, tip, records string) error {
			if writes++; writes == 1 {
				permkit.Chmod(t, ledger, 0o600)
				marked, _, _ = q.store.ReadMark()
			}
			return write(slug, tip, records)
		}

		q.requireDone(deps)

		if len(marked.Members) != 2 || marked.Members[1].Records != entry.RecordsHead {
			t.Errorf("move mark = %+v; want two holding %s", marked, entry.RecordsHead)
		}
		if got := q.landingsByRun()["two"].RecordsCommit; got != entry.RecordsHead {
			t.Errorf("records commit of two's landing = %s; want %s", got, entry.RecordsHead)
		}
	})

	t.Run("AMemberWithAnEarlierHeldRecordsCommitIsMarkedWithIt", func(t *testing.T) {
		t.Parallel()
		q := newQueueHub(t, "one", "two")
		held := gitkit.CommitFile(t, q.h.PairRecordsSibling("two"), "held.txt", "held\n", "records an earlier landing holds")
		standing := pushledger.Landing{Run: "two", Tip: q.primeHead(), RecordsCommit: held, ParentBranch: q.branch, At: time.Now()}
		if err := runpusher.RecordLanding(runpusher.LandingTarget(q.h.Location), standing, time.Now()); err != nil {
			t.Fatalf("RecordLanding: %v", err)
		}
		q.join("two", true)
		deps := q.leader("one", 5, "true")
		var marked landingqueue.MoveMark
		write := deps.WriteLedgerLanding
		deps.WriteLedgerLanding = func(slug, tip, records string) error {
			if mark, found, err := q.store.ReadMark(); err == nil && found {
				marked = mark
			}
			return write(slug, tip, records)
		}

		q.requireDone(deps)

		if len(marked.Members) != 2 || marked.Members[1].Records != held {
			t.Errorf("move mark = %+v; want two holding %s", marked, held)
		}
	})

	for name, tt := range map[string]struct {
		// file and content are what the second run commits over its own work.
		file, content string
		wantOutcome   landingqueue.Outcome
	}{
		"AMemberConflictingWithTheChainGetsTheConflictOutcome": {"one.txt", "two's\n", landingqueue.OutcomeConflict},
		"AMemberWhoseWorkTheChainHoldsGetsNoOutcome":           {"one.txt", "one\n", ""},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			q := newQueueHub(t, "one", "three")
			q.addRun("two", "", false)
			gitkit.CommitFile(t, q.h.PairCodeWorktree("two"), tt.file, tt.content, "the work of two")
			q.join("two", true)
			q.join("three", true)

			q.requireDone(q.leader("one", 5, "true"))

			q.requireOutcome("two", tt.wantOutcome)
			q.requireOutcome("three", landingqueue.OutcomeLanded)
			if _, landed := q.landingsByRun()["two"]; landed {
				t.Error("a landing of two is recorded; want it left out of the chain")
			}
		})
	}

	t.Run("AChainShorterThanTwoLandsTheLeaderAloneWithNoBatchGate", func(t *testing.T) {
		t.Parallel()
		q := newQueueHub(t, "one", "two")
		gitkit.CommitFile(t, q.h.PairCodeWorktree("two"), "one.txt", "two's\n", "work that conflicts with one")
		q.join("two", true)
		gate, record := q.headGate("")

		q.requireDone(q.leader("one", 5, gate))

		if lines := recordedLines(t, record); len(lines) != 1 || detachedRuns(lines) != 0 {
			t.Errorf("gate runs = %q; want one over the task branch", lines)
		}
		q.requireOutcome("two", landingqueue.OutcomeConflict)
	})

	t.Run("AConflictLeaderLeadsNoBatch", func(t *testing.T) {
		t.Parallel()
		q := newQueueHub(t, "one", "two")
		q.enqueue("one", landingqueue.OutcomeConflict, "")
		q.join("two", true)

		q.requireDone(q.leader("one", 5, "true"))

		if !q.parentHolds("one.txt") || q.parentHolds("two.txt") {
			t.Errorf("parent holds one = %v, two = %v; want the leader alone", q.parentHolds("one.txt"), q.parentHolds("two.txt"))
		}
		q.requireOutcome("two", "")
	})

	t.Run("AGateLeavingAFileTheRestoreRefusesIsStuckWithTheMarkerKept", func(t *testing.T) {
		t.Parallel()
		q := newQueueHub(t, "one", "two")
		q.join("two", true)
		deps := q.leader("one", 5, "echo stray > stray.txt")

		outcome, pointer, err := q.call(deps)

		if err != nil || outcome != shedengine.Stuck || !strings.Contains(pointer.Reason, "stray.txt") || !strings.Contains(pointer.Reason, "lym loom resume") {
			t.Fatalf("Call() = %q, %+v, %v; want Stuck naming stray.txt and the resume", outcome, pointer, err)
		}
		state, err := fabricengine.ReadDetachState(q.location("one"))
		if err != nil || !state.Detached || !state.Marker {
			t.Errorf("worktree of one = %+v, %v; want it detached with its marker", state, err)
		}
	})

	for name, act := range map[string]func(*testing.T, *queueHub, landingshed.Deps){
		"AParentLockReleasedDuringTheBatchGateMovesNothing": func(t *testing.T, q *queueHub, deps landingshed.Deps) {
			owner, err := deps.Lock.Owner()
			if err != nil {
				t.Errorf("Owner(): %v", err)
			}
			if released, err := deps.Lock.Store.Release(q.branch, owner); err != nil || !released {
				t.Errorf("Release() = %v, %v; want true", released, err)
			}
		},
		"AnOwnerChangedDuringTheBatchGateMovesNothing": func(_ *testing.T, q *queueHub, _ landingshed.Deps) {
			q.setOwners(boardengine.OwnerRequest{Machine: otherMachine, Prime: true, Takeover: true})
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			q := newQueueHub(t, "one", "two")
			q.join("two", true)
			deps := q.leader("one", 5, "true")
			held := deps
			onRestore(&deps, func() { act(t, q, held) })
			before := q.primeHead()

			outcome, pointer, err := q.call(deps)

			if err != nil || outcome != shedengine.Stuck {
				t.Fatalf("Call() = %q, %+v, %v; want Stuck", outcome, pointer, err)
			}
			if q.primeHead() != before {
				t.Errorf("prime head = %s; want it unmoved at %s", q.primeHead(), before)
			}
			q.requireOutcome("two", "")
		})
	}

	t.Run("AFlakyBatchGateLandsTheBatchWithNoFlakySection", func(t *testing.T) {
		t.Parallel()
		q := newQueueHub(t, "one", "two")
		q.join("two", true)
		flag := filepath.ToSlash(filepath.Join(t.TempDir(), "failed"))
		gate, record := q.headGate("")
		failFirst := fmt.Sprintf("; if [ ! -e '%[1]s' ]; then touch '%[1]s'; printf '%%s\\n' '%[2]s' 'tier0 (exit 1), log /runs/a.log:' '  Ns.Tests.A:'; exit 1; fi", flag, gateslot.FailureReportHeader)
		deps := q.leader("one", 5, gate+failFirst)

		q.requireDone(deps)

		if got := detachedRuns(recordedLines(t, record)); got != 2 {
			t.Errorf("batch gate runs = %d; want the failure and its rerun", got)
		}
		if message := gitkit.Git(t, q.h.PrimeWorktree(), "log", "-1", "--format=%B", "HEAD~1"); strings.Contains(message, "Flaky") || !q.parentHolds("two.txt") {
			t.Errorf("leader's landing message = %q, parent holds two = %v; want no flaky section and the batch landed", message, q.parentHolds("two.txt"))
		}
	})

	t.Run("ABatchTakesAtMostItsSizeInJoinOrderAndSkipsWhoCannotJoinIt", func(t *testing.T) {
		t.Parallel()
		q := newQueueHub(t, "one", "paused", "moved", "decided", "five", "six")
		for _, slug := range []string{"paused", "moved", "decided", "five", "six"} {
			q.join(slug, true)
		}
		geometry, err := hubgeom.PauseGeometry(q.location("paused"))
		if err != nil {
			t.Fatalf("PauseGeometry: %v", err)
		}
		now := time.Now()
		if _, err := pauserecord.Update(geometry.RecordPath, now, func(record *pauserecord.Record) error {
			record.Live = append(record.Live, pauserecord.Entry{Scope: "paused", PausedAt: now, GraceEnd: now.Add(time.Hour)})
			return nil
		}); err != nil {
			t.Fatalf("write the pause record: %v", err)
		}
		gitkit.CommitFile(t, q.h.PairCodeWorktree("moved"), "late.txt", "late\n", "work after the join")
		if err := q.store.SetOutcome("decided", landingqueue.OutcomeConflict, ""); err != nil {
			t.Fatalf("SetOutcome: %v", err)
		}
		deps := q.leader("one", 2, "true")

		q.requireDone(deps)

		for _, slug := range []string{"paused", "moved", "decided", "six"} {
			if q.parentHolds(slug + ".txt") {
				t.Errorf("the parent holds the work of %s; want it left in the queue", slug)
			}
		}
		if !q.parentHolds("five.txt") {
			t.Error("the parent lacks the work of five; want it taken in join order")
		}
		q.requireOutcome("six", "")
		q.requireOutcome("paused", "")
	})

	t.Run("AFailedLeaderLandsAloneAndLeavesTheWaitingRuns", func(t *testing.T) {
		t.Parallel()
		q := newQueueHub(t, "one", "two", "three")
		q.enqueue("one", landingqueue.OutcomeFailed, q.primeHead())
		q.join("two", true)
		q.join("three", true)
		gate, record := q.headGate("")
		deps := q.leader("one", 5, gate)

		q.requireDone(deps)

		if lines := recordedLines(t, record); len(lines) != 1 || detachedRuns(lines) != 0 {
			t.Errorf("gate runs = %q; want one over the task branch", lines)
		}
		q.requireOutcome("two", "")
		q.requireOutcome("three", "")
	})

	t.Run("ALandedRunTheParentDroppedLeadsABatch", func(t *testing.T) {
		t.Parallel()
		q := newQueueHub(t, "one", "two")
		commit := q.merge("one", q.h.PrimeWorktree())
		q.enqueue("one", landingqueue.OutcomeLanded, commit)
		gitkit.Git(t, q.h.PrimeWorktree(), "reset", "-q", "--keep", "HEAD~1")
		q.join("two", true)

		q.requireDone(q.leader("one", 5, "true"))

		if !q.parentHolds("one.txt") || !q.parentHolds("two.txt") {
			t.Errorf("parent holds one = %v, two = %v; want both landed", q.parentHolds("one.txt"), q.parentHolds("two.txt"))
		}
		q.requireOutcome("two", landingqueue.OutcomeLanded)
	})

	t.Run("ARunWithNoQueueEntryTakesNoMember", func(t *testing.T) {
		t.Parallel()
		q := newQueueHub(t, "one", "two")
		q.join("two", true)
		deps := q.leader("one", 5, "true")
		// The first two reads of the mode, up to the queue join, say the landing needs GitHub.
		q.setPullRequestMode("one", beamsetting.PullRequestTrail)
		read, reads := deps.PullRequest.Mode, 0
		deps.PullRequest.Mode = func() (string, error) {
			if reads++; reads == 3 {
				q.setPullRequestMode("one", beamsetting.PullRequestOff)
			}
			return read()
		}

		q.requireDone(deps)

		if q.parentHolds("two.txt") {
			t.Error("the parent holds the work of two; want no member taken")
		}
		q.requireOutcome("two", "")
	})

	t.Run("AFastForwardConflictingWithTheLeaderBuildsNoChain", func(t *testing.T) {
		t.Parallel()
		q := newQueueHub(t, "one", "two")
		q.join("two", true)
		push := q.pushUpstream("one.txt", "upstream\n")
		deps := q.leader("one", 5, "true")
		beforeSync(&deps, push)
		builds := 0
		build := deps.BuildChain
		deps.BuildChain = func(base string, links []fabricengine.ChainLink) (fabricengine.ChainResult, error) {
			builds++
			return build(base, links)
		}
		underLock := false
		shuttle := q.resolvingShuttle(deps, "one", "one.txt", &underLock)
		during := shuttle.DuringRun
		deps.Shuttle = shuttle
		shuttle.DuringRun = func(call int, spec shuttleengine.Spec) {
			during(call, spec)
			if builds != 0 {
				t.Errorf("chains built before the conflict session = %d; want none", builds)
			}
			q.requireOutcome("two", "")
		}

		q.requireDone(deps)

		if len(shuttle.Specs) != 1 || underLock {
			t.Errorf("sessions = %d, under the lock = %v; want one with no lock held", len(shuttle.Specs), underLock)
		}
	})

	t.Run("AFastForwardHoldingTheLeadersWorkTakesNoMember", func(t *testing.T) {
		t.Parallel()
		q := newQueueHub(t, "one", "two")
		q.join("two", true)
		push := q.pushUpstream("one.txt", "one\n")
		gate, record := q.headGate("")
		deps := q.leader("one", 5, gate)
		beforeSync(&deps, push)

		q.requireDone(deps)

		if lines := recordedLines(t, record); len(lines) != 0 || q.parentHolds("two.txt") {
			t.Errorf("gate runs = %q, parent holds two = %v; want no gate and no member", lines, q.parentHolds("two.txt"))
		}
		q.requireOutcome("two", "")
	})

	t.Run("WithPushesSkippedABatchWritesNoLedgerLanding", func(t *testing.T) {
		t.Parallel()
		q := newQueueHub(t, "one", "two")
		q.join("two", true)
		deps := q.leader("one", 5, "true")
		deps.PushSkipped = true

		q.requireDone(deps)
		member := q.deps("two", true, "true")
		member.PushSkipped = true
		q.requireDone(member)

		if landings := q.entry(pushledger.LandingKey).Landings; len(landings) != 0 || !q.parentHolds("two.txt") {
			t.Errorf("landings = %+v, parent holds two = %v; want none recorded and the batch landed", landings, q.parentHolds("two.txt"))
		}
		if !q.done("one") || !q.done("two") {
			t.Errorf("done: one = %v, two = %v; want both marked", q.done("one"), q.done("two"))
		}
	})
}

// TestFinalizeQueue_LeavesOutAMember covers the runs a batch does not land, with the others landed.
func TestFinalizeQueue_LeavesOutAMember(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	// requireWithoutTwo runs the leader one and fails unless it landed itself and three and left two alone.
	requireWithoutTwo := func(t *testing.T, q *queueHub, deps landingshed.Deps) {
		t.Helper()
		q.requireDone(deps)
		if !q.parentHolds("one.txt") || !q.parentHolds("three.txt") || q.parentHolds("two.txt") {
			t.Errorf("parent holds one = %v, two = %v, three = %v; want one and three alone", q.parentHolds("one.txt"), q.parentHolds("two.txt"), q.parentHolds("three.txt"))
		}
		q.requireOutcome("two", "")
		q.requireOutcome("three", landingqueue.OutcomeLanded)
	}
	// batch builds a hub with the leader one and the waiting runs two and three.
	batch := func(t *testing.T) (*queueHub, landingshed.Deps) {
		t.Helper()
		q := newQueueHub(t, "one", "two", "three")
		q.join("two", true)
		q.join("three", true)
		return q, q.leader("one", 5, "true")
	}

	for name, arrange := range map[string]func(*testing.T, *queueHub, *landingshed.Deps){
		"AMemberEndedOnOrigin": func(t *testing.T, q *queueHub, _ *landingshed.Deps) {
			// Origin ends only a run that has a batten run record in the prime.
			status := shedengine.Status{State: shedengine.StateRunning, CurrentProducer: "Run-Shed"}
			if err := state.WriteJSON(shedrun.StatusFile(q.h.Location, "two"), shedrun.StatusLock(q.h.Location, "two"), status); err != nil {
				t.Fatalf("write the batten run record: %v", err)
			}
			done := boardengine.StatusDone
			if err := q.board().SetStatusPushed("two", &done); err != nil {
				t.Fatalf("SetStatusPushed: %v", err)
			}
		},
		"AMemberWhoseParentAnotherMachineOwns": func(t *testing.T, q *queueHub, deps *landingshed.Deps) {
			here, err := machinename.Name()
			if err != nil {
				t.Fatalf("machine name: %v", err)
			}
			amendMember(deps, "two", func(member *landingshed.Deps) {
				owner := member.ParentOwner
				// The leader reads the same parent, so the board names the other machine for the member's read alone.
				member.ParentOwner = func() (landingshed.ParentOwnership, error) {
					q.setOwners(boardengine.OwnerRequest{Machine: otherMachine, Prime: true, Takeover: true})
					defer q.setOwners(boardengine.OwnerRequest{Machine: here, Prime: true, Takeover: true})
					return owner()
				}
			})
		},
		"AMemberWithAFixerChange": func(_ *testing.T, q *queueHub, _ *landingshed.Deps) {
			q.recordFixerChange("two")
		},
		"AMemberGitHubsRulesSendThroughAPullRequest": func(_ *testing.T, q *queueHub, deps *landingshed.Deps) {
			// With the mode off the landing needs no GitHub on disk, and only the resolver's rules read sends it there.
			q.setPullRequestMode("two", beamsetting.PullRequestOff)
			api := &prkit.API{}
			api.Script(prkit.MethodRequiresPullRequest, prkit.Answer{Found: true})
			amendMember(deps, "two", func(member *landingshed.Deps) { githubOrigin(member, api) })
		},
		"AMemberWhoseJudgingReadFails": func(t *testing.T, q *queueHub, deps *landingshed.Deps) {
			q.setPullRequestMode("two", beamsetting.PullRequestOff)
			config := configengine.ConfigFile(q.location("two").AnchorPath(), beamsetting.Module)
			onRestore(deps, func() { permkit.Chmod(t, config, 0) })
		},
		"AMemberPausedAfterTheBatchWasTaken": func(t *testing.T, q *queueHub, deps *landingshed.Deps) {
			geometry, err := hubgeom.PauseGeometry(q.location("two"))
			if err != nil {
				t.Fatalf("PauseGeometry: %v", err)
			}
			onRestore(deps, func() {
				now := time.Now()
				if _, err := pauserecord.Update(geometry.RecordPath, now, func(record *pauserecord.Record) error {
					record.Live = append(record.Live, pauserecord.Entry{Scope: "two", PausedAt: now, GraceEnd: now.Add(time.Hour)})
					return nil
				}); err != nil {
					t.Errorf("write the pause record: %v", err)
				}
			})
		},
		"AMemberWhoseTaskBranchMovedAfterTheBatchWasTaken": func(t *testing.T, q *queueHub, deps *landingshed.Deps) {
			onRestore(deps, func() {
				gitkit.CommitFile(t, q.h.PairCodeWorktree("two"), "late.txt", "late\n", "work after the batch was taken")
			})
		},
		"AMemberWhoseOwnerDiedAfterTheBatchWasTaken": func(t *testing.T, q *queueHub, deps *landingshed.Deps) {
			owner := exec.Command("sleep", "300")
			if err := owner.Start(); err != nil {
				t.Fatalf("start the owner process: %v", err)
			}
			stop := sync.OnceFunc(func() {
				_ = owner.Process.Kill()
				_ = owner.Wait()
			})
			t.Cleanup(stop)
			entry, _ := q.queued("two")
			entry.PID = owner.Process.Pid
			entry.StartTime, _ = proc.StartTime(entry.PID)
			if err := q.store.Join(entry); err != nil {
				t.Fatalf("Join(two): %v", err)
			}
			onRestore(deps, stop)
		},
		"AMemberWhoseSeamsCannotBeBuilt": func(t *testing.T, q *queueHub, _ *landingshed.Deps) {
			entry, _ := q.queued("two")
			entry.Pair = t.TempDir()
			if err := q.store.Join(entry); err != nil {
				t.Fatalf("Join(two): %v", err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			q, deps := batch(t)
			arrange(t, q, &deps)
			requireWithoutTwo(t, q, deps)
		})
	}

	t.Run("AMemberOwnedElsewhere", func(t *testing.T) {
		t.Parallel()
		q := newQueueHub(t, "one", "three")
		q.addRun("two", "", true)
		q.work("two")
		q.join("two", true)
		q.join("three", true)
		q.takeOverOnOriginOnly("two")

		requireWithoutTwo(t, q, q.leader("one", 5, "true"))
	})

	t.Run("AMemberWithALandBlocker", func(t *testing.T) {
		t.Parallel()
		q := newQueueHub(t, "one", "three")
		q.addRun("two", "", true)
		q.work("two")
		// A pawl run's land blockers are the pairs still working under its beam.
		q.addRun("child", "two", false)
		seed := shedrun.Seed{Recipe: shedrun.RecipePawl, Driver: shedrun.DriverGo}
		if err := shedrun.WriteSeed(q.location("two"), shedrun.SelfRunID, seed); err != nil {
			t.Fatalf("WriteSeed: %v", err)
		}
		q.join("two", true)
		q.join("three", true)

		requireWithoutTwo(t, q, q.leader("one", 5, "true"))
	})

	t.Run("AMemberBehindOriginKeepsItsBranch", func(t *testing.T) {
		t.Parallel()
		q, deps := batch(t)
		gitkit.Git(t, q.h.PairCodeWorktree("two"), "push", "-q", "-u", "origin", "HEAD")
		head := q.taskHead("two")
		clone := q.clone(q.h.CodeBare, q.branches["two"])
		gitkit.CommitFile(t, clone, "remote.txt", "remote\n", "work pushed from elsewhere")
		gitkit.Git(t, clone, "push", "-q")

		requireWithoutTwo(t, q, deps)

		if got := q.taskHead("two"); got != head {
			t.Errorf("task head of two = %s; want it unmoved at %s", got, head)
		}
	})

	t.Run("AMemberWithAPullRequestRecordLandsAloneThroughIt", func(t *testing.T) {
		t.Parallel()
		q, deps := batch(t)
		member := q.deps("two", true, "true")
		record := pullrequest.Record{Number: 7, URL: "https://github.com/acme/widgets/pull/7"}
		if err := pullrequest.WriteRecord(member.PullRequest.RecordPath, member.PullRequest.RecordLockPath, record); err != nil {
			t.Fatalf("WriteRecord: %v", err)
		}
		if err := member.PullRequest.CommitRecord(); err != nil {
			t.Fatalf("CommitRecord: %v", err)
		}

		requireWithoutTwo(t, q, deps)

		api := &prkit.API{}
		api.Script(prkit.MethodStatus, prkit.Answer{Status: pullrequest.Status{State: pullrequest.StateOpen}})
		githubOrigin(&member, api)
		q.setPullRequestMode("two", beamsetting.PullRequestTrail)
		// A GitHub landing waits for the batch's push of the parent.
		q.pass(runpusher.LandingTarget(q.h.Location))
		q.requireDone(member)
		if len(api.Calls(prkit.MethodMerge)) != 1 || !q.parentHolds("two.txt") {
			t.Errorf("GitHub merges = %v, parent holds two = %v; want one merge and two landed", api.Calls(prkit.MethodMerge), q.parentHolds("two.txt"))
		}
	})

	t.Run("ALeaderThroughAPullRequestTakesNoMember", func(t *testing.T) {
		t.Parallel()
		q, deps := batch(t)
		api := &prkit.API{}
		api.Script(prkit.MethodCreate, prkit.Answer{Ref: pullrequest.Ref{Number: 7, URL: "https://github.com/acme/widgets/pull/7"}})
		githubOrigin(&deps, api)
		q.setPullRequestMode("one", beamsetting.PullRequestTrail)

		q.requireDone(deps)

		if !q.parentHolds("one.txt") || q.parentHolds("two.txt") || q.parentHolds("three.txt") {
			t.Errorf("parent holds one = %v, two = %v, three = %v; want the leader alone", q.parentHolds("one.txt"), q.parentHolds("two.txt"), q.parentHolds("three.txt"))
		}
		q.requireOutcome("two", "")
	})

	t.Run("AParentDirtiedDuringTheGateIsStuckAndUnmoved", func(t *testing.T) {
		t.Parallel()
		q := newQueueHub(t, "one", "two")
		q.commitOnParent("tracked.txt", "tracked\n")
		q.pushParent()
		q.join("two", true)
		gate := fmt.Sprintf("echo dirt >> '%s/tracked.txt'", filepath.ToSlash(q.h.PrimeWorktree()))
		deps := q.leader("one", 5, gate)
		before := q.primeHead()

		outcome, pointer, err := q.call(deps)

		if err != nil || outcome != shedengine.Stuck || !strings.Contains(pointer.Reason, "tracked.txt") {
			t.Fatalf("Call() = %q, %+v, %v; want Stuck naming tracked.txt", outcome, pointer, err)
		}
		if _, found, err := q.store.ReadMark(); err != nil || found || q.primeHead() != before {
			t.Errorf("move mark stands = %v, %v, prime head = %s; want no mark and %s", found, err, q.primeHead(), before)
		}
		q.requireOutcome("two", "")
	})
}

// finalizingSeats is the spent fixer seat runner with another run's Finalize called at its first turn.
type finalizingSeats struct {
	spentFixerSeats
	first func()
}

func (s *finalizingSeats) Run(table seatengine.Table) (seatengine.Result, error) {
	if s.runs == 0 {
		s.first()
	}
	return s.spentFixerSeats.Run(table)
}

// TestFinalizeQueue_BisectsAFailedBatch covers which run a failed batch gate blames and what then lands.
// The culprit leader's fixer rows run on finalizingSeats, spentFixerSeats and fixingSeats.
// Those are a stated exception to PATTERN-test-stand-ins, since no recorded session stand-in exists.
func TestFinalizeQueue_BisectsAFailedBatch(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	// failingGate returns a gate that fails on a tree holding fail.txt.
	// It records the commit of each run, and whether the detach marker of the run one stood, to the two returned files.
	failingGate := func(t *testing.T, q *queueHub) (gate, record, marks string) {
		t.Helper()
		marks = filepath.Join(t.TempDir(), "marks.log")
		marker := filepath.ToSlash(fabricengine.DetachMarkerPath(q.location("one")))
		command, record := q.headGate(fmt.Sprintf("if [ -e '%s' ]; then echo marked; else echo unmarked; fi >> '%s'", marker, filepath.ToSlash(marks)))
		summary := gateslot.TierSummaryLine(gateslot.RunRecord{ExitCode: 1}, "/runs/tier0.log")
		return command + fmt.Sprintf("; git rev-parse HEAD; echo '%s'; test ! -e fail.txt", summary), record, marks
	}
	// culprit builds a hub whose leader one fails the gate, with the runs two and three waiting.
	culprit := func(t *testing.T) (*queueHub, landingshed.Deps, string) {
		t.Helper()
		q := newQueueHub(t, "one", "two", "three")
		gitkit.CommitFile(t, q.h.PairCodeWorktree("one"), "fail.txt", "fail\n", "the failing work")
		q.join("two", true)
		q.join("three", true)
		gate, record, _ := failingGate(t, q)
		return q, q.leader("one", 5, gate), record
	}

	t.Run("ABatchWhoseThirdRunFailsLandsTheFirstTwo", func(t *testing.T) {
		t.Parallel()
		q := newQueueHub(t, "one", "two", "three", "four")
		gitkit.CommitFile(t, q.h.PairCodeWorktree("three"), "fail.txt", "fail\n", "the failing work")
		for _, slug := range []string{"two", "three", "four"} {
			q.join(slug, true)
		}
		gate, record, marks := failingGate(t, q)
		deps := q.leader("one", 5, gate)

		q.requireDone(deps)

		lines := recordedLines(t, record)
		if len(lines) != 3 || detachedRuns(lines) != 3 {
			t.Fatalf("gate runs = %q; want the whole chain and two prefixes", lines)
		}
		if got := recordedLines(t, marks); !slices.Equal(got, []string{"marked", "marked", "marked"}) {
			t.Errorf("detach marker at each gate run = %v; want it standing", got)
		}
		landings := q.landingsByRun()
		if len(landings) != 2 || landings["one"].Tip != q.rev("HEAD~1") || landings["two"].Tip != q.rev("HEAD") {
			t.Errorf("landings = %+v; want one of one and one of two", landings)
		}
		third, _, _ := strings.Cut(lines[2], " ")
		if entry, _ := q.queued("three"); entry.Outcome != landingqueue.OutcomeFailed || entry.Commit != third {
			t.Errorf("entry of three = %+v; want failed on %s", entry, third)
		}
		q.requireOutcome("four", "")
		if failure, found, err := verifytree.ReadLandingFailure(verifytree.NewPaths(deps.WorktreeRoot, deps.VerifyDir)); err != nil || found {
			t.Errorf("landing failure record = %+v, %v, %v; want none", failure, found, err)
		}
	})

	t.Run("ACulpritLeaderLandsNothingAndGoesToItsFixer", func(t *testing.T) {
		t.Parallel()
		q, deps, record := culprit(t)
		// The sync brings the leader a parent commit it has not merged, which is the chain's base.
		beforeSync(&deps, q.pushUpstream("upstream.txt", "upstream\n"))
		var atFixer []string
		deps.FixerSeats = &spentFixerSeats{onRun: func() { atFixer = recordedLines(t, record) }}

		outcome, pointer, err := q.call(deps)

		if err != nil || outcome != shedengine.Stuck || !strings.HasPrefix(pointer.Reason, landingshed.LandingStopPrefix) {
			t.Fatalf("Call() = %q, %+v, %v; want a landing stop", outcome, pointer, err)
		}
		if q.parentHolds("fail.txt") || q.parentHolds("two.txt") {
			t.Error("the parent holds work of the batch; want nothing landed")
		}
		q.requireOutcome("one", landingqueue.OutcomeFailed)
		q.requireOutcome("two", "")
		q.requireOutcome("three", "")
		if len(atFixer) == 0 || detachedRuns(atFixer) != len(atFixer) {
			t.Fatalf("gate runs before the fixer's turn = %q; want batch runs alone", atFixer)
		}
		base := q.primeHead()
		merge, found, err := landingshed.ReadMergeRecord(deps.MergeRecordPath)
		if err != nil || !found || merge.MergedTip != base || merge.MergeCommit != q.taskHead("one") {
			t.Errorf("merge record = %+v, %v, %v; want the chain base %s merged", merge, found, err, base)
		}
		failure, found, err := verifytree.ReadLandingFailure(verifytree.NewPaths(deps.WorktreeRoot, deps.VerifyDir))
		if err != nil || !found {
			t.Fatalf("ReadLandingFailure() = %v, %v; want a record", found, err)
		}
		log, err := os.ReadFile(failure.LogPath)
		first, _, _ := strings.Cut(atFixer[len(atFixer)-1], " ")
		if err != nil || !strings.Contains(string(log), first) {
			t.Errorf("failure log %s = %q, %v; want the run over the first chain commit %s", failure.LogPath, log, err, first)
		}
	})

	t.Run("ABatchLedWhileTheCulpritsFixerRunsLeavesItsEntry", func(t *testing.T) {
		t.Parallel()
		q, deps, _ := culprit(t)
		var taken []string
		seats := &finalizingSeats{first: func() {
			second := q.leader("two", 5, "true")
			build := second.BuildChain
			second.BuildChain = func(base string, links []fabricengine.ChainLink) (fabricengine.ChainResult, error) {
				for _, link := range links {
					taken = append(taken, link.Slug)
				}
				return build(base, links)
			}
			q.requireDone(second)
		}}
		deps.FixerSeats = seats

		if outcome, pointer, err := q.call(deps); err != nil || outcome != shedengine.Stuck {
			t.Fatalf("Call() = %q, %+v, %v; want Stuck", outcome, pointer, err)
		}

		if !slices.Equal(taken, []string{"two", "three"}) {
			t.Errorf("runs of the second batch = %v; want two and three", taken)
		}
		q.requireOutcome("one", landingqueue.OutcomeFailed)
	})

	t.Run("ACulpritLeaderWhoseFixPassesLandsAlone", func(t *testing.T) {
		t.Parallel()
		q, deps, _ := culprit(t)
		deps.Config.FinalizeFixMaxLines = 100
		// The fixture has no plan, which counts every file as a test file; a recipe with no plan row matches by name.
		deps.IsTestFile = planparser.IsAnyLanguageTestFile
		pair := q.h.PairCodeWorktree("one")
		deps.FixerSeats = fixingSeats{
			report: filepath.Join(deps.ScratchDir, loomshed.NameFinalize+"-fixer-report.md"),
			fix: func() {
				gitkit.Git(t, pair, "rm", "-q", "fail.txt")
				gitkit.Git(t, pair, "commit", "-q", "-m", "fix the failure")
			},
		}

		q.requireDone(deps)

		if !q.parentHolds("one.txt") || q.parentHolds("fail.txt") || q.parentHolds("two.txt") {
			t.Errorf("parent holds one = %v, fail = %v, two = %v; want the fixed leader alone", q.parentHolds("one.txt"), q.parentHolds("fail.txt"), q.parentHolds("two.txt"))
		}
		q.requireOutcome("two", "")
	})

	t.Run("ACulpritLeaderWithNoAttemptLeftStopsWithNoFixer", func(t *testing.T) {
		t.Parallel()
		q, deps, _ := culprit(t)
		deps.Config.FinalizeFixAttempts = 0
		seats := &spentFixerSeats{}
		deps.FixerSeats = seats

		outcome, pointer, err := q.call(deps)

		if err != nil || outcome != shedengine.Stuck || !strings.Contains(pointer.Reason, "spent every attempt") || seats.runs != 0 {
			t.Errorf("Call() = %q, %+v, %v with %d fixer runs; want a spent stop and none", outcome, pointer, err, seats.runs)
		}
	})

	for name, tt := range map[string]struct {
		// failedFirst has the whole chain's run fail on its tests before a run meets the memory pressure stop.
		failedFirst bool
		wantRuns    int
	}{
		"AMemoryPressureStopHaltsTheLeaderWithNoBisection": {false, 1},
		"AMemoryPressureStopMidBisectionHaltsTheLeader":    {true, 2},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			q := newQueueHub(t, "one", "two", "three")
			q.join("two", true)
			q.join("three", true)
			line := filepath.Join(t.TempDir(), "pressure.log")
			if err := os.WriteFile(line, []byte(gateslot.MemoryPressureLine("tier1 (exit 1)")+"\n"), 0o644); err != nil {
				t.Fatalf("write the memory pressure line: %v", err)
			}
			gate, record := q.headGate("")
			if tt.failedFirst {
				flag := filepath.ToSlash(filepath.Join(t.TempDir(), "failed"))
				summary := gateslot.TierSummaryLine(gateslot.RunRecord{ExitCode: 1}, "/runs/tier0.log")
				gate += fmt.Sprintf("; if [ ! -e '%[1]s' ]; then touch '%[1]s'; echo '%[2]s'; exit 1; fi", flag, summary)
			}
			deps := q.leader("one", 5, fmt.Sprintf("%s; cat '%s'; exit 1", gate, filepath.ToSlash(line)))

			outcome, pointer, err := q.call(deps)

			if err != nil || outcome != shedengine.Stuck || !strings.HasPrefix(pointer.Reason, landingshed.LandingStopPrefix) {
				t.Fatalf("Call() = %q, %+v, %v; want a landing stop", outcome, pointer, err)
			}
			if lines := recordedLines(t, record); len(lines) != tt.wantRuns {
				t.Errorf("gate runs = %q; want %d", lines, tt.wantRuns)
			}
			for _, slug := range []string{"one", "two", "three"} {
				q.requireOutcome(slug, "")
			}
		})
	}
}

// TestFinalizeQueue_ActsOnItsOutcome covers a run whose entry or parent already says how its landing stands.
func TestFinalizeQueue_ActsOnItsOutcome(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	// requireHeld runs Finalize and fails unless it records want with no merge, no gate and no session.
	requireHeld := func(t *testing.T, q *queueHub, want string) *landedCommits {
		t.Helper()
		gate, record := q.recordingGate()
		deps := q.deps("one", true, gate)
		shuttle := &shedfake.MergeShuttle{}
		deps.Shuttle = shuttle
		seen := recordLandings(&deps)
		parent, task := q.primeHead(), q.taskHead("one")

		q.requireDone(deps)

		runs, _ := gateRuns(t, record)
		if runs != 0 || len(shuttle.Specs) != 0 || q.primeHead() != parent || q.taskHead("one") != task {
			t.Errorf("gate runs = %d, sessions = %d, prime = %s, task = %s; want none, %s and %s", runs, len(shuttle.Specs), q.primeHead(), q.taskHead("one"), parent, task)
		}
		if !slices.Equal(seen.recorded, []string{want}) || !slices.Equal(seen.landed, []string{want}) {
			t.Errorf("recorded = %v, landed = %v; want %s for both", seen.recorded, seen.landed, want)
		}
		return seen
	}

	t.Run("ALandedRunRecordsItsCommit", func(t *testing.T) {
		t.Parallel()
		q := newQueueHub(t, "one")
		commit := q.merge("one", q.h.PrimeWorktree())
		q.enqueue("one", landingqueue.OutcomeLanded, commit)

		requireHeld(t, q, commit)
	})

	t.Run("ALandedRunWhoseFileTheParentRewroteIsStillHeld", func(t *testing.T) {
		t.Parallel()
		q := newQueueHub(t, "one")
		commit := q.merge("one", q.h.PrimeWorktree())
		q.commitOnParent("one.txt", "rewritten\n")
		q.enqueue("one", landingqueue.OutcomeLanded, commit)

		requireHeld(t, q, commit)
	})

	t.Run("AParentHoldingTheWorkIsHeldWithNoOutcome", func(t *testing.T) {
		t.Parallel()
		q := newQueueHub(t, "one")
		commit := q.merge("one", q.h.PrimeWorktree())

		requireHeld(t, q, commit)
	})

	t.Run("ASquashWithNoTrailerRecordsTheParentHeadWithNoSize", func(t *testing.T) {
		t.Parallel()
		q := newQueueHub(t, "one")
		gitkit.Git(t, q.h.PrimeWorktree(), "merge", "--squash", "-q", q.branches["one"])
		gitkit.Git(t, q.h.PrimeWorktree(), "commit", "-q", "-m", "squashed by hand")

		seen := requireHeld(t, q, q.primeHead())

		if seen.sizes != 0 || !slices.Equal(seen.empty, []bool{false}) {
			t.Errorf("sizes recorded = %d, empty = %v; want no size and a landing read as not empty", seen.sizes, seen.empty)
		}
	})

	// requireLandsAgain runs Finalize and fails unless it gates once and the parent then holds file.
	requireLandsAgain := func(t *testing.T, q *queueHub, file string) {
		t.Helper()
		gate, record := q.recordingGate()

		q.requireDone(q.deps("one", true, gate))

		if runs, _ := gateRuns(t, record); runs != 1 {
			t.Errorf("gate runs = %d; want 1", runs)
		}
		gitkit.Git(t, q.h.PrimeWorktree(), "cat-file", "-e", "HEAD:"+file)
	}

	t.Run("ALandedCommitTheParentDroppedIsLandedAgain", func(t *testing.T) {
		t.Parallel()
		q := newQueueHub(t, "one")
		commit := q.merge("one", q.h.PrimeWorktree())
		q.enqueue("one", landingqueue.OutcomeLanded, commit)
		gitkit.Git(t, q.h.PrimeWorktree(), "reset", "-q", "--keep", "HEAD~1")

		requireLandsAgain(t, q, "one.txt")
	})

	t.Run("ACommitMadeAfterTheLandingIsLanded", func(t *testing.T) {
		t.Parallel()
		q := newQueueHub(t, "one")
		commit := q.merge("one", q.h.PrimeWorktree())
		q.pushParent()
		q.enqueue("one", landingqueue.OutcomeLanded, commit)
		gitkit.CommitFile(t, q.h.PairCodeWorktree("one"), "later.txt", "later\n", "more work")

		requireLandsAgain(t, q, "later.txt")
	})

	t.Run("AMoveWhoseLeaderDiedGetsItsLandingsBeforeAnyEntryIsPruned", func(t *testing.T) {
		t.Parallel()
		q := newQueueHub(t, "one", "two", "three")
		mark := landingqueue.MoveMark{}
		for _, slug := range []string{"two", "three"} {
			commit := q.merge(slug, q.h.PrimeWorktree())
			records := gitkit.RevParse(t, q.h.PairRecordsSibling(slug), "HEAD")
			mark.Members = append(mark.Members, landingqueue.MarkMember{Slug: slug, Commit: commit, TaskHead: q.taskHead(slug), Records: records})
			dead := landingqueue.Entry{Slug: slug, TaskHead: q.taskHead(slug), PID: 1, StartTime: "dead", JoinedAt: time.Now().UTC()}
			if err := q.store.Join(dead); err != nil {
				t.Fatalf("Join(%s): %v", slug, err)
			}
		}
		if err := q.store.WriteMark(mark); err != nil {
			t.Fatalf("WriteMark: %v", err)
		}

		q.requireDone(q.deps("one", true, "true"))

		landings := q.entry(pushledger.LandingKey).Landings
		for _, member := range mark.Members {
			if !slices.ContainsFunc(landings, func(l pushledger.Landing) bool {
				return l.Run == member.Slug && l.Tip == member.Commit && l.RecordsCommit == member.Records
			}) {
				t.Errorf("landings = %+v; want one of %s at %s holding %s", landings, member.Slug, member.Commit, member.Records)
			}
			if entry, found := q.queued(member.Slug); !found || entry.Outcome != landingqueue.OutcomeLanded {
				t.Errorf("entry of %s = %+v, %v; want it kept as landed", member.Slug, entry, found)
			}
		}
	})
}
