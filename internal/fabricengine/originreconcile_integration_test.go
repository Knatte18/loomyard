//go:build integration

// originreconcile_integration_test.go proves the origin reconcile's judgment on hubforge hubs.
// The other machine is a second clone of the hub's code or records bare repository.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
)

// The text a stopped dirty, gone or busy branch's line is followed by in a printed summary.
const cleanupRerunLine = "then re-run lyx cleanup --apply from the prime"

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

// originHub is a hub whose prime records branch is published, so every judged branch has a tracking ref, and the paths its tests reach for.
type originHub struct {
	*hubforge.Hub
	primeBranch      string
	primeRecordsPath string
	primeRecords     string
}

func newOriginHub(t *testing.T) *originHub {
	t.Helper()
	hub := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	recordsBranch := gitkit.CurrentBranch(t, hub.PrimeRecords())
	gitkit.Git(t, hub.PrimeRecords(), "push", "--set-upstream", "origin", recordsBranch)
	return &originHub{
		Hub:              hub,
		primeBranch:      gitkit.CurrentBranch(t, hub.PrimeWorktree()),
		primeRecordsPath: hub.PrimeRecords(),
		primeRecords:     recordsBranch,
	}
}

// otherMachine clones bare at branch, as a second machine working on the same remote.
func otherMachine(t *testing.T, bare, branch string) string {
	t.Helper()
	other := filepath.Join(t.TempDir(), "other")
	gitkit.Git(t, filepath.Dir(other), "clone", "--branch", branch, bare, other)
	return other
}

// originPair adds the pair slug, with its upstream set, and returns its code worktree and branch.
func originPair(t *testing.T, hub *originHub, slug string) (worktree, branch string) {
	t.Helper()
	hubforge.AddPair(t, hub.Hub, slug)
	worktree = hub.PairCodeWorktree(slug)
	branch = gitkit.CurrentBranch(t, worktree)
	gitkit.Git(t, worktree, "branch", "--set-upstream-to=origin/"+branch)
	return worktree, branch
}

func reconcile(t *testing.T, hub *originHub, scope fabricengine.ReconcileScope, opts fabricengine.ReconcileOptions) fabricengine.OriginReport {
	t.Helper()
	report, err := fabricengine.ReconcileOrigin(hub.Location, scope, fabricengine.OriginSeams{}, opts)
	if err != nil {
		t.Fatalf("ReconcileOrigin(%+v) error = %v; want nil", scope, err)
	}
	return report
}

// branchOf returns the outcome for the branch of repo side, failing the test when the report holds none.
func branchOf(t *testing.T, report fabricengine.OriginReport, repo, branch string) fabricengine.BranchOutcome {
	t.Helper()
	for _, outcome := range report.Branches {
		if outcome.Repo == repo && outcome.Branch == branch {
			return outcome
		}
	}
	t.Fatalf("report holds no %s branch %q; branches = %+v", repo, branch, report.Branches)
	return fabricengine.BranchOutcome{}
}

func requireState(t *testing.T, report fabricengine.OriginReport, repo, branch string, want fabricengine.BranchState) fabricengine.BranchOutcome {
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
func writeStatus(t *testing.T, hub *originHub, slug, producer string) {
	t.Helper()
	path := shedrun.StatusFile(hub.Location, slug)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(`{"current_producer":"`+producer+`"}`), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// TestReconcileOrigin_Judge covers the origin reconcile's judgment, its network calls and its entry classification.
// It runs without t.Parallel because it sets the process-global origin call hook.
func TestReconcileOrigin_Judge(t *testing.T) {
	boardkit.InstallReplay(t)

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
		t.Cleanup(fabricengine.SetOriginCallHookForTest(calls.record))
		report := reconcile(t, hub, fabricengine.ReconcileScope{JudgeOnly: true}, fabricengine.ReconcileOptions{})

		code := requireState(t, report, "code", hub.primeBranch, fabricengine.BranchFastForward)
		records := requireState(t, report, "records", hub.primeRecords, fabricengine.BranchFastForward)
		bolt := requireState(t, report, "bolt", "main", fabricengine.BranchFastForward)
		for _, row := range []struct {
			name    string
			outcome fabricengine.BranchOutcome
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

	t.Run("An origin that moved nothing is read by two ls-remote calls and no fetch", func(t *testing.T) {
		hub := newOriginHub(t)
		originPair(t, hub, "task")

		var calls callLog
		t.Cleanup(fabricengine.SetOriginCallHookForTest(calls.record))
		report := reconcile(t, hub, fabricengine.ReconcileScope{}, fabricengine.ReconcileOptions{})

		for _, outcome := range report.Branches {
			if outcome.State != fabricengine.BranchLevel {
				t.Errorf("%s branch %q state = %q; want level", outcome.Repo, outcome.Branch, outcome.State)
			}
		}
		if got := len(calls.withVerb("ls-remote")); got != 2 {
			t.Errorf("ls-remote calls = %d; want 2", got)
		}
		if fetches := calls.withVerb("fetch"); len(fetches) != 0 {
			t.Errorf("fetch calls = %v; want none when no head moved", fetches)
		}
		if report.Summary() != "" {
			t.Errorf("Summary() = %q; want empty for a level hub", report.Summary())
		}
	})

	t.Run("A code branch both sides moved reads diverged and names the merge that settles it", func(t *testing.T) {
		hub := newOriginHub(t)
		pairCode, pairBranch := originPair(t, hub, "task")
		for _, row := range []struct {
			name, worktree, branch, bare string
			prime                        bool
		}{
			{"a task branch", pairCode, pairBranch, hub.CodeBare, false},
			{"the prime's branch", hub.PrimeWorktree(), hub.primeBranch, hub.CodeBare, true},
		} {
			other := otherMachine(t, row.bare, row.branch)
			gitkit.CommitFile(t, other, "origin.txt", "origin", "origin side")
			gitkit.Git(t, other, "push", "origin", row.branch)
			gitkit.CommitFile(t, row.worktree, "local.txt", "local", "local side")

			report := reconcile(t, hub, fabricengine.ReconcileScope{}, fabricengine.ReconcileOptions{})

			outcome := requireState(t, report, "code", row.branch, fabricengine.BranchDiverged)
			want := fabricengine.CodeDivergedWayForwardForTest(row.worktree, row.branch, row.prime)
			if outcome.WayForward != want {
				t.Errorf("%s WayForward = %q; want %q", row.name, outcome.WayForward, want)
			}
			if !strings.Contains(report.Summary(), want) {
				t.Errorf("%s Summary() = %q; want it to carry %q", row.name, report.Summary(), want)
			}
			if row.prime && (!strings.Contains(want, "`git push` in "+row.worktree) || strings.Contains(want, "lyx fabric push")) {
				t.Errorf("the prime's line = %q; want `git push` in the prime and no `lyx fabric push`", want)
			}
		}
	})

	for _, withLocal := range []bool{false, true} {
		for _, fetchedBy := range []string{"nothing", "Fabric.Pull", "a moving reconcile"} {
			name := "A task branch origin rewound reads rewound, fetched by " + fetchedBy
			if withLocal {
				name += ", with an unpushed local commit"
			}
			t.Run(name, func(t *testing.T) {
				rewound := newRewoundBranch(t, withLocal)
				switch fetchedBy {
				case "Fabric.Pull":
					loc, err := lyxcwd.ResolveWorktree(rewound.worktree)
					if err != nil {
						t.Fatalf("ResolveWorktree(%s): %v", rewound.worktree, err)
					}
					f, err := fabricengine.Open(loc)
					if err != nil {
						t.Fatalf("Open(%s): %v", rewound.worktree, err)
					}
					if _, err := f.Pull(fabricengine.SyncOptions{}); !errors.Is(err, fabricengine.ErrWarpRewound) {
						t.Fatalf("Pull() error = %v; want ErrWarpRewound after fetching the tracking ref", err)
					}
				case "a moving reconcile":
					reconcile(t, rewound.hub, fabricengine.ReconcileScope{}, fabricengine.ReconcileOptions{})
				}

				for _, scope := range []fabricengine.ReconcileScope{{JudgeOnly: true}, {}} {
					report := reconcile(t, rewound.hub, scope, fabricengine.ReconcileOptions{})
					outcome := requireState(t, report, "code", rewound.branch, fabricengine.BranchRewound)
					if !slices.Equal(outcome.Commits, []string{rewound.second, rewound.first}) {
						t.Errorf("scope %+v dropped commits = %v; want %v", scope, outcome.Commits, []string{rewound.second, rewound.first})
					}
					want := fabricengine.RewoundWayForwardForTest(rewound.worktree, rewound.branch, rewound.base, []string{rewound.second, rewound.first})
					if outcome.WayForward != want || !strings.Contains(report.Summary(), want) {
						t.Errorf("scope %+v WayForward = %q, Summary() = %q; want both to carry %q", scope, outcome.WayForward, report.Summary(), want)
					}
				}
			})
		}
	}

	for _, settle := range []string{"git reset --keep to origin's head", "git push of the branch"} {
		t.Run("A rewound task branch settled by "+settle+" reads level", func(t *testing.T) {
			rewound := newRewoundBranch(t, true)
			reconcile(t, rewound.hub, fabricengine.ReconcileScope{}, fabricengine.ReconcileOptions{})
			if settle == "git push of the branch" {
				gitkit.Git(t, rewound.worktree, "push", "origin", rewound.branch)
			} else {
				gitkit.Git(t, rewound.worktree, "reset", "--keep", rewound.base)
			}

			report := reconcile(t, rewound.hub, fabricengine.ReconcileScope{}, fabricengine.ReconcileOptions{})

			requireState(t, report, "code", rewound.branch, fabricengine.BranchLevel)
		})
	}

	t.Run("A branch whose origin head lies below an earlier tracking value and shares no commit with the local tip reads diverged", func(t *testing.T) {
		hub := newOriginHub(t)
		pairCode, branch := originPair(t, hub, "task")
		parent := gitkit.RevParse(t, pairCode, "HEAD")
		kept := gitkit.CommitFile(t, pairCode, "one.txt", "one", "one")
		earlier := gitkit.CommitFile(t, pairCode, "two.txt", "two", "two")
		gitkit.Git(t, pairCode, "push", "origin", branch)
		gitkit.Git(t, pairCode, "reset", "--hard", parent)
		gitkit.CommitFile(t, pairCode, "local.txt", "local", "local only")
		other := otherMachine(t, hub.CodeBare, branch)
		gitkit.Git(t, other, "push", "--force-with-lease=refs/heads/"+branch+":"+earlier, "origin", kept+":refs/heads/"+branch)

		report := reconcile(t, hub, fabricengine.ReconcileScope{}, fabricengine.ReconcileOptions{})

		requireState(t, report, "code", branch, fabricengine.BranchDiverged)
	})

	t.Run("A code branch origin replaced with a history no earlier value holds reads rewritten", func(t *testing.T) {
		hub := newOriginHub(t)
		pairCode, pairBranch := originPair(t, hub, "task")
		for _, row := range []struct{ name, worktree, branch string }{
			{"task branch", pairCode, pairBranch},
			{"prime branch", hub.PrimeWorktree(), hub.primeBranch},
		} {
			// A pull gives the tracking ref a reflog value, as every branch of a long-lived checkout has one; a fresh clone's ref has none.
			other := otherMachine(t, hub.CodeBare, row.branch)
			gitkit.CommitFile(t, other, "advanced.txt", "advanced", "origin advanced")
			gitkit.Git(t, other, "push", "origin", row.branch)
			gitkit.Git(t, row.worktree, "pull", "--ff-only")
			tip := gitkit.RevParse(t, row.worktree, "HEAD")
			gitkit.Git(t, other, "checkout", "--orphan", "replaced")
			gitkit.Git(t, other, "commit", "--allow-empty", "-m", "origin replaced the history")
			gitkit.Git(t, other, "push", "--force-with-lease=refs/heads/"+row.branch+":"+tip, "origin", "replaced:refs/heads/"+row.branch)

			report := reconcile(t, hub, fabricengine.ReconcileScope{}, fabricengine.ReconcileOptions{})

			outcome := requireState(t, report, "code", row.branch, fabricengine.BranchRewritten)
			if want := "`lyx fabric pull` in " + row.worktree; outcome.WayForward != want {
				t.Errorf("%s WayForward = %q; want exactly %q", row.name, outcome.WayForward, want)
			}
		}
	})

	t.Run("A deleted upstream reads gone and its summary follows the way forward with the cleanup re-entry", func(t *testing.T) {
		hub := newOriginHub(t)
		pairCode, branch := originPair(t, hub, "task")
		gitkit.Git(t, hub.CodeBare, "branch", "-D", branch)

		report := reconcile(t, hub, fabricengine.ReconcileScope{}, fabricengine.ReconcileOptions{})

		outcome := requireState(t, report, "code", branch, fabricengine.BranchGone)
		want := fabricengine.FailedPushWayForward(fabricengine.FailedPush{Repo: "code", RepoPath: pairCode, Branch: branch, Class: fabricengine.PushFailureGone})
		if outcome.WayForward != want {
			t.Errorf("WayForward = %q; want exactly %q, with no re-entry verb", outcome.WayForward, want)
		}
		if got := report.Summary(); !strings.Contains(got, want+"; "+cleanupRerunLine) {
			t.Errorf("Summary() = %q; want %q followed by %q", got, want, cleanupRerunLine)
		}
	})

	t.Run("A judge-only reconcile fetches a moved branch's objects and leaves every ref unchanged, and reads a diverged Bolt as needs_replay", func(t *testing.T) {
		hub := newOriginHub(t)
		recordsOther := otherMachine(t, hub.RecordsBare, hub.primeRecords)
		recordsHead := gitkit.CommitFile(t, recordsOther, "moved.txt", "moved", "origin moved records")
		gitkit.Git(t, recordsOther, "push", "origin", hub.primeRecords)
		boltOther := otherMachine(t, hub.RecordsBare, "main")
		gitkit.CommitFile(t, boltOther, "origin.txt", "origin", "origin side")
		gitkit.Git(t, boltOther, "push", "origin", "main")
		gitkit.CommitFile(t, hub.BoardDir(), "local.txt", "local", "local side")
		refsBefore := refsOf(t, hub.PrimeRecords())

		report := reconcile(t, hub, fabricengine.ReconcileScope{JudgeOnly: true}, fabricengine.ReconcileOptions{})

		requireState(t, report, "records", hub.primeRecords, fabricengine.BranchFastForward)
		requireState(t, report, "bolt", "main", fabricengine.BranchNeedsReplay)
		if got := refsOf(t, hub.PrimeRecords()); got != refsBefore {
			t.Errorf("refs after a judge-only reconcile:\n%s\nwant them unchanged:\n%s", got, refsBefore)
		}
		gitkit.Git(t, hub.PrimeRecords(), "cat-file", "-e", recordsHead+"^{commit}")
		if got := report.Summary(); strings.Contains(got, "needs_replay") {
			t.Errorf("Summary() = %q; want a branch awaiting replay not listed as a stop", got)
		}
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
			t.Cleanup(fabricengine.SetOriginCallHookForTest(calls.record))
			opts := fabricengine.ReconcileOptions{
				RetryBound:    3 * time.Second,
				RetryInterval: time.Second,
				Sleep: func(time.Duration) {
					sleeps++
					if row.restoreOnSleep {
						restore()
					}
				},
			}

			report := reconcile(t, hub, fabricengine.ReconcileScope{}, opts)

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

	t.Run("An origin that is no repository is an error with no retry", func(t *testing.T) {
		hub := newOriginHub(t)
		gitkit.Git(t, hub.PrimeRecords(), "remote", "set-url", "origin", filepath.Join(t.TempDir(), "no-repository"))
		var calls callLog
		t.Cleanup(fabricengine.SetOriginCallHookForTest(calls.record))
		sleeps := 0

		_, err := fabricengine.ReconcileOrigin(hub.Location, fabricengine.ReconcileScope{}, fabricengine.OriginSeams{}, fabricengine.ReconcileOptions{
			RetryBound:    3 * time.Second,
			RetryInterval: time.Second,
			Sleep:         func(time.Duration) { sleeps++ },
		})

		if err == nil || !strings.Contains(err.Error(), "records repository") {
			t.Fatalf("ReconcileOrigin() error = %v; want an error naming the records repository", err)
		}
		if sleeps != 0 || len(calls.withVerb("ls-remote")) != 2 {
			t.Errorf("sleeps = %d, ls-remote calls = %d; want no retry of a refusal", sleeps, len(calls.withVerb("ls-remote")))
		}
	})

	t.Run("A fetch that fails after both ls-remote calls answered is an error and moves nothing", func(t *testing.T) {
		hub := newOriginHub(t)
		other := otherMachine(t, hub.RecordsBare, hub.primeRecords)
		gitkit.CommitFile(t, other, "moved.txt", "moved", "origin moved records")
		gitkit.Git(t, other, "push", "origin", hub.primeRecords)
		refsBefore := refsOf(t, hub.PrimeRecords())
		localBefore := gitkit.RevParse(t, hub.PrimeRecords(), "HEAD")
		t.Cleanup(fabricengine.SetOriginCallHookForTest(func(args []string) {
			if args[0] == "fetch" && slices.ContainsFunc(args, func(arg string) bool { return strings.Contains(arg, hub.primeRecords) }) {
				gitkit.Git(t, hub.RecordsBare, "update-ref", "-d", "refs/heads/"+hub.primeRecords)
			}
		}))

		_, err := fabricengine.ReconcileOrigin(hub.Location, fabricengine.ReconcileScope{}, fabricengine.OriginSeams{}, fabricengine.ReconcileOptions{})

		if err == nil || !strings.Contains(err.Error(), hub.primeRecords) {
			t.Fatalf("ReconcileOrigin() error = %v; want a fetch error naming %s", err, hub.primeRecords)
		}
		if got := refsOf(t, hub.PrimeRecords()); got != refsBefore {
			t.Errorf("refs after the failed fetch:\n%s\nwant them unchanged:\n%s", got, refsBefore)
		}
		if got := gitkit.RevParse(t, hub.PrimeRecords(), "HEAD"); got != localBefore {
			t.Errorf("records HEAD = %q; want %q, nothing moved", got, localBefore)
		}
	})

	t.Run("A run scope judges Bolt, the run's parent branch on both sides and the run's own pair", func(t *testing.T) {
		hub := newOriginHub(t)
		_, firstBranch := originPair(t, hub, "first")
		originPair(t, hub, "second")
		firstRecords := gitkit.CurrentBranch(t, hub.PairRecordsSibling("first"))

		report := reconcile(t, hub, fabricengine.ReconcileScope{Run: "first"}, fabricengine.ReconcileOptions{})

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

	t.Run("An entry is classified only for a pair with a batten run record", func(t *testing.T) {
		hub := newOriginHub(t)
		_, stoppedBranch := originPair(t, hub, "stopped")
		originPair(t, hub, "active")
		originPair(t, hub, "unrecorded")
		gitkit.Git(t, hub.CodeBare, "branch", "-D", stoppedBranch)
		writeStatus(t, hub, "stopped", "Land")
		writeStatus(t, hub, "active", "Land")

		report := reconcile(t, hub, fabricengine.ReconcileScope{}, fabricengine.ReconcileOptions{})

		got := map[string]fabricengine.EntryState{}
		for _, entry := range report.Entries {
			got[entry.Slug] = entry.State
		}
		want := map[string]fabricengine.EntryState{"stopped": fabricengine.EntryStopped, "active": fabricengine.EntryActive}
		if len(got) != len(want) || got["stopped"] != want["stopped"] || got["active"] != want["active"] {
			t.Errorf("entries = %v; want %v, with none for the pair without a run record", got, want)
		}
	})

	t.Run("classifyEntry puts ended over owned_elsewhere, owned_elsewhere over every branch cause and a D3 stop over active", func(t *testing.T) {
		for _, tt := range []struct {
			name  string
			facts fabricengine.EntryFactsForTest
			want  fabricengine.EntryState
		}{
			{"ended over a stopping branch", fabricengine.EntryFactsForTest{Ended: true, Branches: []fabricengine.BranchState{fabricengine.BranchDirty}}, fabricengine.EntryEnded},
			{"ended over a D3 stop", fabricengine.EntryFactsForTest{Ended: true, D3Stop: true}, fabricengine.EntryEnded},
			{"ended over owned_elsewhere", fabricengine.EntryFactsForTest{Ended: true, OwnedElsewhere: true}, fabricengine.EntryEnded},
			{"owned_elsewhere over stopping branches", fabricengine.EntryFactsForTest{OwnedElsewhere: true, Branches: []fabricengine.BranchState{fabricengine.BranchDiverged, fabricengine.BranchDirty}}, fabricengine.EntryOwnedElsewhere},
			{"owned_elsewhere over a D3 stop", fabricengine.EntryFactsForTest{OwnedElsewhere: true, D3Stop: true}, fabricengine.EntryOwnedElsewhere},
			{"a D3 stop over level branches", fabricengine.EntryFactsForTest{D3Stop: true, Branches: []fabricengine.BranchState{fabricengine.BranchLevel}}, fabricengine.EntryStopped},
			{"a diverged branch", fabricengine.EntryFactsForTest{Branches: []fabricengine.BranchState{fabricengine.BranchLevel, fabricengine.BranchDiverged}}, fabricengine.EntryStopped},
			{"a rewound branch", fabricengine.EntryFactsForTest{Branches: []fabricengine.BranchState{fabricengine.BranchRewound}}, fabricengine.EntryStopped},
			{"a rewritten branch", fabricengine.EntryFactsForTest{Branches: []fabricengine.BranchState{fabricengine.BranchRewritten}}, fabricengine.EntryStopped},
			{"a gone branch", fabricengine.EntryFactsForTest{Branches: []fabricengine.BranchState{fabricengine.BranchGone}}, fabricengine.EntryStopped},
			{"a busy branch", fabricengine.EntryFactsForTest{Branches: []fabricengine.BranchState{fabricengine.BranchBusy}}, fabricengine.EntryStopped},
			{"a live pair's branch", fabricengine.EntryFactsForTest{Branches: []fabricengine.BranchState{fabricengine.BranchLive}}, fabricengine.EntryStopped},
			{"branches that never stop", fabricengine.EntryFactsForTest{Branches: []fabricengine.BranchState{fabricengine.BranchLevel, fabricengine.BranchFastForward, fabricengine.BranchAhead, fabricengine.BranchNeedsReplay}}, fabricengine.EntryActive},
			{"no branch", fabricengine.EntryFactsForTest{}, fabricengine.EntryActive},
		} {
			if got := fabricengine.ClassifyEntryForTest(tt.facts); got != tt.want {
				t.Errorf("%s: classifyEntry(%+v) = %q; want %q", tt.name, tt.facts, got, tt.want)
			}
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

// newRewoundBranch builds a hub with a pair whose branch carries two pushed commits that another machine's leased update then dropped from origin.
// With withLocal the pair also holds one commit that was never pushed.
func newRewoundBranch(t *testing.T, withLocal bool) rewoundBranch {
	t.Helper()
	hub := newOriginHub(t)
	worktree, branch := originPair(t, hub, "task")
	base := gitkit.RevParse(t, worktree, "HEAD")
	first := gitkit.CommitFile(t, worktree, "one.txt", "one", "one")
	second := gitkit.CommitFile(t, worktree, "two.txt", "two", "two")
	gitkit.Git(t, worktree, "push", "origin", branch)
	other := otherMachine(t, hub.CodeBare, branch)
	gitkit.Git(t, other, "push", "--force-with-lease=refs/heads/"+branch+":"+second, "origin", base+":refs/heads/"+branch)
	if withLocal {
		gitkit.CommitFile(t, worktree, "local.txt", "local", "local only")
	}
	return rewoundBranch{hub: hub, worktree: worktree, branch: branch, base: base, first: first, second: second}
}
