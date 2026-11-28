// originreconcile_integration_test.go proves the origin reconcile's judgment on hubforge hubs.
// The other machine is a second clone of the hub's code or records bare repository.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/testkit/fabrickit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// callLog records the arguments of every remote call a reconcile makes.
type callLog struct {
	mu    sync.Mutex
	calls [][]string
}

func (c *callLog) record(args []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, slices.Clone(args))
}

// withVerb returns the recorded calls whose git subcommand is verb.
func (c *callLog) withVerb(verb string) [][]string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var found [][]string
	for _, args := range c.calls {
		if args[0] == verb {
			found = append(found, args)
		}
	}
	return found
}

// originHub is a hub whose prime records branch is published, so every judged branch has a tracking ref.
// It also holds the paths its tests reach for.
type originHub struct {
	*hubforge.Hub
	primeBranch      string
	primeRecordsPath string
	primeRecords     string
}

// originTemplate is the "." hub with the prime's records branch published.
var originTemplate = hubforge.Registration{
	Key:   "fabricengine-origin",
	Base:  hubforge.Shape{Anchor: "."},
	Build: publishPrimeRecords,
}

// originTaskTemplate is originTemplate's hub with the pair "task" added and its code branch's upstream set.
var originTaskTemplate = hubforge.Registration{
	Key:   "fabricengine-origin-task",
	Base:  hubforge.Shape{Anchor: "."},
	Build: buildOriginTask,
}

// publishPrimeRecords pushes the prime's records branch with its upstream set.
func publishPrimeRecords(tb testing.TB, h *hubforge.Hub) {
	tb.Helper()
	publishBranch(tb, h.PrimeRecords())
}

// buildOriginTask publishes the prime's records branch and adds the pair "task" with its upstream set.
func buildOriginTask(tb testing.TB, h *hubforge.Hub) {
	tb.Helper()
	publishPrimeRecords(tb, h)
	originPair(tb, originHubOf(tb, h), "task")
}

// originHubOf returns h with the prime's branch names and records path read from it.
func originHubOf(tb testing.TB, h *hubforge.Hub) *originHub {
	tb.Helper()
	return &originHub{
		Hub:              h,
		primeBranch:      gitkit.CurrentBranch(tb, h.PrimeWorktree()),
		primeRecordsPath: h.PrimeRecords(),
		primeRecords:     gitkit.CurrentBranch(tb, h.PrimeRecords()),
	}
}

// newOriginHub returns a copy of originTemplate.
func newOriginHub(t *testing.T) *originHub {
	t.Helper()
	return originHubOf(t, hubforge.CopyRegistered(t, &originTemplate))
}

// newOriginTaskHub returns a copy of originTaskTemplate with the code worktree and branch of its pair "task".
func newOriginTaskHub(t *testing.T) (hub *originHub, worktree, branch string) {
	t.Helper()
	hub = originHubOf(t, hubforge.CopyRegistered(t, &originTaskTemplate))
	worktree = hub.PairCodeWorktree("task")
	return hub, worktree, gitkit.CurrentBranch(t, worktree)
}

// otherMachine clones bare at branch, as a second machine working on the same remote.
func otherMachine(t *testing.T, bare, branch string) string {
	t.Helper()
	other := filepath.Join(t.TempDir(), "other")
	gitkit.Git(t, filepath.Dir(other), "clone", "--branch", branch, bare, other)
	return other
}

// originPair adds the pair slug, with its upstream set, and returns its code worktree and branch.
func originPair(tb testing.TB, hub *originHub, slug string) (worktree, branch string) {
	tb.Helper()
	hubforge.AddPair(tb, hub.Hub, slug)
	worktree = hub.PairCodeWorktree(slug)
	branch = gitkit.CurrentBranch(tb, worktree)
	gitkit.Git(tb, worktree, "branch", "--set-upstream-to=origin/"+branch)
	return worktree, branch
}

func reconcile(t *testing.T, hub *originHub, scope fabriclayout.ReconcileScope, opts fabricengine.ReconcileOptions) fabriclayout.OriginReport {
	t.Helper()
	report, err := fabricengine.ReconcileOrigin(hub.Location, scope, fabricengine.OriginSeams{}, opts)
	if err != nil {
		t.Fatalf("ReconcileOrigin(%+v) error = %v; want nil", scope, err)
	}
	return report
}

// remoteRefsLockPaths returns where the remote-refs locks of the hub's code and records repositories live.
func remoteRefsLockPaths(t *testing.T, hub *originHub) []string {
	t.Helper()
	var paths []string
	for _, dir := range []string{hub.PrimeWorktree(), hub.PrimeRecords()} {
		commonDir, err := gitrepo.New(dir).CommonDir()
		if err != nil {
			t.Fatalf("CommonDir(%s) error = %v", dir, err)
		}
		paths = append(paths, filepath.Join(commonDir, gitrepo.RemoteRefsLockFileName))
	}
	return paths
}

// freeRemoteRefsLocks returns the lock paths it could take, releasing each at once.
func freeRemoteRefsLocks(t *testing.T, paths []string) []string {
	t.Helper()
	var free []string
	for _, path := range paths {
		held, acquired, err := lock.TryAcquireWriteLock(path)
		if err != nil {
			t.Errorf("TryAcquireWriteLock(%s) error = %v", path, err)
		}
		if acquired {
			_ = held.Release()
			free = append(free, path)
		}
	}
	return free
}

// branchOf returns the outcome for the branch of repo side, failing the test when the report holds none.
func branchOf(t *testing.T, report fabriclayout.OriginReport, repo, branch string) fabriclayout.BranchOutcome {
	t.Helper()
	outcome, found := fabrickit.BranchOf(report, repo, branch)
	if !found {
		t.Fatalf("report holds no %s branch %q; branches = %+v", repo, branch, report.Branches)
	}
	return outcome
}

func requireState(t *testing.T, report fabriclayout.OriginReport, repo, branch string, want fabriclayout.BranchState) fabriclayout.BranchOutcome {
	t.Helper()
	outcome := branchOf(t, report, repo, branch)
	if outcome.State != want {
		t.Errorf("%s branch %q state = %q; want %q", repo, branch, outcome.State, want)
	}
	return outcome
}

// refsOf lists every ref of dir with its target, tracking refs included.
func refsOf(t *testing.T, dir string) string {
	t.Helper()
	return gitkit.Git(t, dir, "for-each-ref", "--format=%(refname) %(objectname)")
}

// writeStatus gives slug a batten run record at the prime, naming producer as its current row.
func writeStatus(tb testing.TB, hub *originHub, slug, producer string) {
	tb.Helper()
	path := fabricengine.RunStatusFileForTest(hub.Location, slug)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		tb.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(`{"current_producer":"`+producer+`"}`), 0o644); err != nil {
		tb.Fatalf("write %s: %v", path, err)
	}
}

// TestReconcileOrigin_Judge covers the origin reconcile's judgment and its network calls.
func TestReconcileOrigin_Judge(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	t.Run("A judge-only reconcile of an origin that moved a records branch, Bolt and the prime's code branch reads fast_forward for each, fetching only moved heads", func(t *testing.T) {
		hub := newOriginHub(t)
		warpOther := otherMachine(t, hub.CodeBare, hub.primeBranch)
		warpHead := gitkit.CommitFile(t, warpOther, "moved.txt", "moved", "origin moved code")
		gitkit.Git(t, warpOther, "push", "origin", hub.primeBranch)
		recordsOther := otherMachine(t, hub.RecordsBare, hub.primeRecords)
		recordsHead := gitkit.CommitFile(t, recordsOther, "moved.txt", "moved", "origin moved records")
		gitkit.Git(t, recordsOther, "push", "origin", hub.primeRecords)
		boltOther := otherMachine(t, hub.RecordsBare, "main")
		boltHead := gitkit.CommitFile(t, boltOther, "moved.txt", "moved", "origin moved bolt")
		gitkit.Git(t, boltOther, "push", "origin", "main")

		var calls callLog
		report := reconcile(t, hub, fabriclayout.ReconcileScope{JudgeOnly: true}, fabricengine.ReconcileOptions{CallHook: calls.record})

		code := requireState(t, report, "code", hub.primeBranch, fabriclayout.BranchFastForward)
		records := requireState(t, report, "records", hub.primeRecords, fabriclayout.BranchFastForward)
		bolt := requireState(t, report, "bolt", "main", fabriclayout.BranchFastForward)
		for _, row := range []struct {
			name    string
			outcome fabriclayout.BranchOutcome
			want    string
		}{{"code", code, warpHead}, {"records", records, recordsHead}, {"bolt", bolt, boltHead}} {
			if row.outcome.OriginHead != row.want {
				t.Errorf("%s OriginHead = %q; want %q", row.name, row.outcome.OriginHead, row.want)
			}
			if row.outcome.WayForward != "" || row.outcome.Moved {
				t.Errorf("%s outcome = %+v; want no way forward and nothing moved by a judging reconcile", row.name, row.outcome)
			}
		}
		if got := gitkit.RevParse(t, hub.PrimeWorktree(), hub.primeBranch); got == warpHead {
			t.Errorf("local %s moved to origin's head; a judging reconcile moves no branch", hub.primeBranch)
		}

		if got := len(calls.withVerb("ls-remote")); got != 2 {
			t.Errorf("ls-remote calls = %d; want exactly one per repository", got)
		}
		fetches := calls.withVerb("fetch")
		if len(fetches) != 2 {
			t.Fatalf("fetch calls = %v; want one per repository", fetches)
		}
		fetchedByRepo := [][]string{fetches[0][4:], fetches[1][4:]}
		slices.SortFunc(fetchedByRepo, func(a, b []string) int { return len(a) - len(b) })
		wantCode := []string{warpHead}
		wantRecords := []string{recordsHead, boltHead}
		slices.Sort(wantRecords)
		slices.Sort(fetchedByRepo[1])
		if !slices.Equal(fetchedByRepo[0], wantCode) || !slices.Equal(fetchedByRepo[1], wantRecords) {
			t.Errorf("fetched heads = %v; want %v and %v, the moved heads only", fetchedByRepo, wantCode, wantRecords)
		}
	})

	t.Run("A reconcile holds both repositories' remote-refs locks across its ls-remote calls, fetches and branch verdicts, and frees them once it returns", func(t *testing.T) {
		hub := newOriginHub(t)
		other := otherMachine(t, hub.CodeBare, hub.primeBranch)
		gitkit.CommitFile(t, other, "moved.txt", "moved", "origin moved code")
		gitkit.Git(t, other, "push", "origin", hub.primeBranch)
		lockPaths := remoteRefsLockPaths(t, hub)

		var calls callLog
		var freeDuringCalls callLog
		verdicts := 0
		var freeDuringVerdicts [][]string
		opts := fabricengine.ReconcileOptions{
			CallHook: func(args []string) {
				calls.record(args)
				if free := freeRemoteRefsLocks(t, lockPaths); len(free) > 0 {
					freeDuringCalls.record(append([]string{args[0]}, free...))
				}
			},
			VerdictHook: func() {
				verdicts++
				if free := freeRemoteRefsLocks(t, lockPaths); len(free) > 0 {
					freeDuringVerdicts = append(freeDuringVerdicts, free)
				}
			},
		}
		report := reconcile(t, hub, fabriclayout.ReconcileScope{}, opts)

		if len(calls.withVerb("ls-remote")) != 2 || len(calls.withVerb("fetch")) != 1 {
			t.Fatalf("remote calls = %v; want two ls-remote calls and one fetch", calls.calls)
		}
		if len(freeDuringCalls.calls) != 0 {
			t.Errorf("remote calls that ran with a remote-refs lock free = %v; want every lock held throughout", freeDuringCalls.calls)
		}
		if verdicts == 0 || verdicts < len(report.Branches) {
			t.Errorf("branch verdicts reached = %d for %d reported branches; want one per branch", verdicts, len(report.Branches))
		}
		if len(freeDuringVerdicts) != 0 {
			t.Errorf("branch verdicts that ran with a remote-refs lock free = %v; want every lock held through the last verdict", freeDuringVerdicts)
		}
		if free := freeRemoteRefsLocks(t, lockPaths); len(free) != len(lockPaths) {
			t.Errorf("remote-refs locks free after the reconcile = %v; want all of %v", free, lockPaths)
		}
	})

	t.Run("A reconcile frees both remote-refs locks on every return", func(t *testing.T) {
		unreachableOptions := fabricengine.ReconcileOptions{
			RetryBound:    3 * time.Second,
			RetryInterval: time.Second,
			Sleep:         func(time.Duration) {},
		}
		for _, row := range []struct {
			name            string
			breakURL        string
			scope           fabriclayout.ReconcileScope
			options         fabricengine.ReconcileOptions
			wantError       bool
			wantUnreachable bool
		}{
			{"a judge-only reconcile", "", fabriclayout.ReconcileScope{JudgeOnly: true}, fabricengine.ReconcileOptions{}, false, false},
			{"an unreachable origin", "http://127.0.0.1:1/x", fabriclayout.ReconcileScope{}, unreachableOptions, false, true},
			{"an origin missing on disk", filepath.Join(t.TempDir(), "removed.git"), fabriclayout.ReconcileScope{}, unreachableOptions, false, true},
			{"an origin that is no repository", t.TempDir(), fabriclayout.ReconcileScope{}, unreachableOptions, true, false},
		} {
			hub := newOriginHub(t)
			if row.breakURL != "" {
				gitkit.Git(t, hub.PrimeRecords(), "remote", "set-url", "origin", row.breakURL)
			}
			lockPaths := remoteRefsLockPaths(t, hub)

			report, err := fabricengine.ReconcileOrigin(hub.Location, row.scope, fabricengine.OriginSeams{}, row.options)

			if (err != nil) != row.wantError {
				t.Errorf("%s: ReconcileOrigin() error = %v; want error %v", row.name, err, row.wantError)
			}
			if report.Unreachable != row.wantUnreachable {
				t.Errorf("%s: ReconcileOrigin() Unreachable = %v; want %v", row.name, report.Unreachable, row.wantUnreachable)
			}
			if free := freeRemoteRefsLocks(t, lockPaths); len(free) != len(lockPaths) {
				t.Errorf("%s: remote-refs locks free after the reconcile = %v; want all of %v", row.name, free, lockPaths)
			}
		}
	})

	t.Run("A task branch origin rewound still reads rewound after a moving reconcile fetched it", func(t *testing.T) {
		rewound := newRewoundBranch(t)
		reconcile(t, rewound.hub, fabriclayout.ReconcileScope{}, fabricengine.ReconcileOptions{})

		for _, scope := range []fabriclayout.ReconcileScope{{JudgeOnly: true}, {}} {
			report := reconcile(t, rewound.hub, scope, fabricengine.ReconcileOptions{})
			outcome := requireState(t, report, "code", rewound.branch, fabriclayout.BranchRewound)
			if !slices.Equal(outcome.Commits, []string{rewound.second, rewound.first}) {
				t.Errorf("scope %+v dropped commits = %v; want %v", scope, outcome.Commits, []string{rewound.second, rewound.first})
			}
			want := fabricengine.RewoundWayForwardForTest(rewound.worktree, rewound.branch, rewound.base, []string{rewound.second, rewound.first})
			if outcome.WayForward != want || !strings.Contains(report.Summary(), want) {
				t.Errorf("scope %+v WayForward = %q, Summary() = %q; want both to carry %q", scope, outcome.WayForward, report.Summary(), want)
			}
		}
	})

	t.Run("A branch whose origin head lies below an earlier tracking value and shares no commit with the local tip reads diverged", func(t *testing.T) {
		hub, pairCode, branch := newOriginTaskHub(t)
		parent := gitkit.RevParse(t, pairCode, "HEAD")
		kept := gitkit.CommitFile(t, pairCode, "one.txt", "one", "one")
		earlier := gitkit.CommitFile(t, pairCode, "two.txt", "two", "two")
		gitkit.Git(t, pairCode, "push", "origin", branch)
		gitkit.Git(t, pairCode, "reset", "--hard", parent)
		gitkit.CommitFile(t, pairCode, "local.txt", "local", "local only")
		other := otherMachine(t, hub.CodeBare, branch)
		gitkit.Git(t, other, "push", "--force-with-lease=refs/heads/"+branch+":"+earlier, "origin", kept+":refs/heads/"+branch)

		report := reconcile(t, hub, fabriclayout.ReconcileScope{}, fabricengine.ReconcileOptions{})

		requireState(t, report, "code", branch, fabriclayout.BranchDiverged)
	})

	t.Run("A records origin that refuses the connection reports unreachable, and a retry restores it", func(t *testing.T) {
		hub := newOriginHub(t)
		realURL := gitkit.Git(t, hub.PrimeRecords(), "remote", "get-url", "origin")
		refuse := func() { gitkit.Git(t, hub.PrimeRecords(), "remote", "set-url", "origin", "http://127.0.0.1:1/x") }
		restore := func() { gitkit.Git(t, hub.PrimeRecords(), "remote", "set-url", "origin", realURL) }

		for _, row := range []struct {
			name            string
			restoreOnSleep  bool
			wantUnreachable bool
			wantSleeps      int
			wantLsRemotes   int
		}{
			{"restored after the first attempt", true, false, 1, 4},
			{"never restored", false, true, 3, 8},
		} {
			refuse()
			var calls callLog
			sleeps := 0
			opts := fabricengine.ReconcileOptions{
				RetryBound:    3 * time.Second,
				RetryInterval: time.Second,
				CallHook:      calls.record,
				Sleep: func(time.Duration) {
					sleeps++
					if row.restoreOnSleep {
						restore()
					}
				},
			}

			report := reconcile(t, hub, fabriclayout.ReconcileScope{}, opts)

			if report.Unreachable != row.wantUnreachable {
				t.Errorf("%s: Unreachable = %v; want %v", row.name, report.Unreachable, row.wantUnreachable)
			}
			if sleeps != row.wantSleeps {
				t.Errorf("%s: sleeps = %d; want %d", row.name, sleeps, row.wantSleeps)
			}
			if got := len(calls.withVerb("ls-remote")); got != row.wantLsRemotes {
				t.Errorf("%s: ls-remote calls = %d; want %d, the pair retried together", row.name, got, row.wantLsRemotes)
			}
			if row.wantUnreachable && (len(report.Branches) != 0 || !strings.Contains(report.Summary(), "unreachable")) {
				t.Errorf("%s: report = %+v; want nothing judged and the summary naming unreachable", row.name, report)
			}
			if !row.wantUnreachable && len(report.Branches) == 0 {
				t.Errorf("%s: no branch was judged after the retry", row.name)
			}
			restore()
		}
	})

	t.Run("A fetch that fails after both ls-remote calls answered is an error and moves nothing", func(t *testing.T) {
		hub := newOriginHub(t)
		other := otherMachine(t, hub.RecordsBare, hub.primeRecords)
		gitkit.CommitFile(t, other, "moved.txt", "moved", "origin moved records")
		gitkit.Git(t, other, "push", "origin", hub.primeRecords)
		refsBefore := refsOf(t, hub.PrimeRecords())
		localBefore := gitkit.RevParse(t, hub.PrimeRecords(), "HEAD")
		lockPaths := remoteRefsLockPaths(t, hub)
		opts := fabricengine.ReconcileOptions{CallHook: func(args []string) {
			if args[0] == "fetch" && slices.ContainsFunc(args, func(arg string) bool { return strings.Contains(arg, hub.primeRecords) }) {
				gitkit.Git(t, hub.RecordsBare, "update-ref", "-d", "refs/heads/"+hub.primeRecords)
			}
		}}

		_, err := fabricengine.ReconcileOrigin(hub.Location, fabriclayout.ReconcileScope{}, fabricengine.OriginSeams{}, opts)

		if err == nil || !strings.Contains(err.Error(), hub.primeRecords) {
			t.Fatalf("ReconcileOrigin() error = %v; want a fetch error naming %s", err, hub.primeRecords)
		}
		if got := refsOf(t, hub.PrimeRecords()); got != refsBefore {
			t.Errorf("refs after the failed fetch:\n%s\nwant them unchanged:\n%s", got, refsBefore)
		}
		if got := gitkit.RevParse(t, hub.PrimeRecords(), "HEAD"); got != localBefore {
			t.Errorf("records HEAD = %q; want %q, nothing moved", got, localBefore)
		}
		if free := freeRemoteRefsLocks(t, lockPaths); len(free) != len(lockPaths) {
			t.Errorf("remote-refs locks free after the failed fetch = %v; want all of %v", free, lockPaths)
		}
	})

	t.Run("A run scope judges Bolt, the run's parent branch on both sides and the run's own pair", func(t *testing.T) {
		hub := newOriginHub(t)
		_, firstBranch := originPair(t, hub, "first")
		originPair(t, hub, "second")
		firstRecords := gitkit.CurrentBranch(t, hub.PairRecordsSibling("first"))

		report := reconcile(t, hub, fabriclayout.ReconcileScope{Run: "first"}, fabricengine.ReconcileOptions{})

		var got []string
		for _, outcome := range report.Branches {
			got = append(got, outcome.Repo+":"+outcome.Branch)
		}
		want := []string{"bolt:main", "code:" + hub.primeBranch, "records:" + hub.primeRecords, "code:" + firstBranch, "records:" + firstRecords}
		slices.Sort(got)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Errorf("judged branches = %v; want %v", got, want)
		}
	})
}

// rewoundBranch is a task branch whose origin was reset back to base past first and second, by another machine.
type rewoundBranch struct {
	hub           *originHub
	worktree      string
	branch        string
	base          string
	first, second string
}

// newRewoundBranch builds a hub with a pair whose branch carries two pushed commits.
// Another machine's leased update then drops them from origin.
// The pair also holds one commit that was never pushed.
func newRewoundBranch(t *testing.T) rewoundBranch {
	t.Helper()
	hub, worktree, branch := newOriginTaskHub(t)
	base := gitkit.RevParse(t, worktree, "HEAD")
	first := gitkit.CommitFile(t, worktree, "one.txt", "one", "one")
	second := gitkit.CommitFile(t, worktree, "two.txt", "two", "two")
	gitkit.Git(t, worktree, "push", "origin", branch)
	other := otherMachine(t, hub.CodeBare, branch)
	gitkit.Git(t, other, "push", "--force-with-lease=refs/heads/"+branch+":"+second, "origin", base+":refs/heads/"+branch)
	gitkit.CommitFile(t, worktree, "local.txt", "local", "local only")
	return rewoundBranch{hub: hub, worktree: worktree, branch: branch, base: base, first: first, second: second}
}
