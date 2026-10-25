//go:build tier0

// anchor_test.go covers the sequence of `lyx fabric anchor` over a fake anchorDeps: the print path, every refusal, the order of the move's steps and the restart or the re-run each failure calls for.

package fabriccli

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/runproc"
)

// anchorFake is an in-memory hub for changeAnchor: it answers every seam from its fields, logs the actions in order and models the engine's move record.
type anchorFake struct {
	calls []string

	acting, prime string
	// anchors holds the anchor of each worktree this machine holds, "." for one it lists with none.
	anchors map[string]string
	beams   map[string]bool
	goos    string

	owners       map[string]string
	upstream     string
	hasUpstream  bool
	recordFound  bool
	reed         map[string]bool
	driverLocked bool
	battenLive   bool
	runOpen      bool
	upstreamOpen bool
	primeRunOpen bool
	dobbyPIDs    map[string]int
	dobbyLive    map[string]bool
	hubReed      []string
	hubBatten    []string
	gates        []string
	hibernating  bool

	// passPolls is how many readings report a pass in flight before one reports none.
	passPolls int
	passErr   error
	lockErr   error
	endErr    error
	// endedBeforeErr are the dobbies the end reports as ended beside endErr.
	endedBeforeErr []string
	// reopenFails is the 1-based count of the ReopenLog call that fails.
	reopenFails int
	reopenErr   error

	noop        bool
	beginErr    error
	applyErr    error
	applyLanded bool
	// applyKeepsRecord leaves the record standing after a failed Apply.
	applyKeepsRecord bool
	finishErr        error
	// finishRemovesRecord removes the record before Finish fails.
	finishRemovesRecord bool
	startErr            error
	ensureErr           error

	reopens int
	polls   int
}

func newAnchorFake() *anchorFake {
	return &anchorFake{
		acting:    "beam1",
		prime:     "prime",
		anchors:   map[string]string{"prime": ".", "beam1": ".", "beam2": ".", "pair1": "."},
		beams:     map[string]bool{"beam1": true, "beam2": true},
		goos:      "linux",
		owners:    map[string]string{},
		reed:      map[string]bool{},
		dobbyPIDs: map[string]int{},
		dobbyLive: map[string]bool{},
	}
}

func (f *anchorFake) log(format string, args ...any) {
	f.calls = append(f.calls, fmt.Sprintf(format, args...))
}

// fakeChange is the engine's AnchorChange as the fake models it.
type fakeChange struct {
	f       *anchorFake
	from    string
	to      string
	fresh   bool
	applied bool
}

func (c *fakeChange) NoOp() bool   { return c.f.noop }
func (c *fakeChange) From() string { return c.from }
func (c *fakeChange) To() string   { return c.to }

func (c *fakeChange) Apply(rec *fabricengine.Mutations) (bool, error) {
	c.f.log("apply")
	c.applied = true
	if c.f.applyErr != nil {
		if !c.f.applyKeepsRecord {
			c.f.recordFound = false
		}
		return c.f.applyLanded, c.f.applyErr
	}
	return true, nil
}

func (c *fakeChange) Finish(rec *fabricengine.Mutations) error {
	c.f.log("finish")
	if c.f.finishErr != nil {
		if c.f.finishRemovesRecord {
			c.f.recordFound = false
		}
		return c.f.finishErr
	}
	c.f.recordFound = false
	return nil
}

func (c *fakeChange) Close() {
	c.f.log("close")
	if c.fresh && !c.applied {
		c.f.recordFound = false
	}
}

func (f *anchorFake) location(slug string) *lyxcwd.Location {
	return &lyxcwd.Location{RepoName: "repo", HubPath: "/hub", WorktreeName: slug, AnchorRel: f.anchors[slug]}
}

func (f *anchorFake) deps() anchorDeps {
	return anchorDeps{
		GOOS:   f.goos,
		Acting: f.acting,
		Prime:  f.prime,
		Worktree: func(slug string) (*lyxcwd.Location, bool, error) {
			if _, held := f.anchors[slug]; !held {
				return nil, false, nil
			}
			return f.location(slug), true, nil
		},
		IsBeam: func(slug string) (bool, error) { return f.beams[slug], nil },
		LocalBeams: func() ([]string, error) {
			var beams []string
			for slug := range f.beams {
				beams = append(beams, slug)
			}
			slices.Sort(beams)
			return beams, nil
		},
		ForeignOwner: func(slug string) (string, error) { return f.owners[slug], nil },
		UpstreamAnchor: func(*lyxcwd.Location) (string, bool, error) {
			return f.upstream, f.hasUpstream, nil
		},
		MoveRecord: func(*lyxcwd.Location) (fabricengine.MoveRecord, bool, error) {
			return fabricengine.MoveRecord{A: ".", B: "sub", Kind: fabricengine.MoveKindMove}, f.recordFound, nil
		},
		ReedHeld:              func(l *lyxcwd.Location) (bool, error) { return f.reed[l.WorktreeName], nil },
		DriverLockHeld:        func(*lyxcwd.Location) (bool, error) { return f.driverLocked, nil },
		BattenLive:            func(*lyxcwd.Location) (bool, error) { return f.battenLive, nil },
		RunUnfinished:         func(*lyxcwd.Location) (bool, error) { return f.runOpen, nil },
		UpstreamRunUnfinished: func(*lyxcwd.Location) (bool, error) { return f.upstreamOpen, nil },
		PrimeRunUnfinished:    func(string) (bool, error) { return f.primeRunOpen, nil },
		DobbyPID: func(worktree string) (int, bool, error) {
			pid, live := f.dobbyPIDs[worktree]
			return pid, live, nil
		},
		DobbyLive:     func(worktree string) (bool, error) { return f.dobbyLive[worktree], nil },
		HubReedHeld:   func() ([]string, error) { return f.hubReed, nil },
		HubBattenLive: func() ([]string, error) { return f.hubBatten, nil },
		GateHolders:   func() ([]string, error) { return f.gates, nil },
		HibernateLive: func() (bool, error) { return f.hibernating, nil },
		PassInFlight: func(worktree string) (string, error) {
			if f.passErr != nil {
				return "", f.passErr
			}
			if f.polls < f.passPolls && worktree == f.acting {
				return "a pass of the dobby of " + worktree, nil
			}
			return "", nil
		},
		Sleep: func(ctx context.Context, d time.Duration) error {
			f.polls++
			f.log("wait")
			return ctx.Err()
		},
		Note: func(line string) { f.log("note %s", line) },
		AcquireParentLock: func(_ context.Context, l *lyxcwd.Location) (func(), error) {
			if f.lockErr != nil {
				return nil, f.lockErr
			}
			f.log("parent-lock")
			return func() { f.log("release-parent-lock") }, nil
		},
		EndDobbies: func(worktrees []string, spare ...string) ([]string, error) {
			f.log("end-dobbies %s spare %s", strings.Join(worktrees, ","), strings.Join(spare, ","))
			if f.endErr != nil {
				return f.endedBeforeErr, f.endErr
			}
			return worktrees, nil
		},
		StartDobby: func(slug string) error {
			f.log("start-dobby %s", slug)
			return f.startErr
		},
		EnsureDobbies: func() error {
			f.log("ensure-dobbies")
			return f.ensureErr
		},
		Begin: func(l *lyxcwd.Location, target string) (anchorChange, error) {
			f.log("begin %s", target)
			if f.beginErr != nil {
				return nil, f.beginErr
			}
			change := &fakeChange{f: f, from: l.AnchorRel, to: target, fresh: !f.recordFound}
			if f.noop {
				change.to = l.AnchorRel
			} else {
				f.recordFound = true
			}
			return change, nil
		},
		ReopenLog: func(dir string) error {
			f.reopens++
			f.log("reopen-log %s", dir)
			if f.reopens == f.reopenFails {
				return f.reopenErr
			}
			return nil
		},
	}
}

func (f *anchorFake) lockDir(slug string) string {
	l := f.location(slug)
	return fabricengine.RecordsLockDirPath(fabricengine.RecordsWorktreePath(l, slug))
}

func (f *anchorFake) logsDir(slug, anchor string) string {
	return logger.LogsDirAt(filepath.Join(fabricengine.WorktreePath(f.location(slug), slug), anchor))
}

func (f *anchorFake) did(call string) bool {
	return slices.ContainsFunc(f.calls, func(got string) bool { return strings.HasPrefix(got, call) })
}

func runAnchorFake(t *testing.T, f *anchorFake, req anchorRequest) (anchorResult, error) {
	t.Helper()

	return changeAnchor(context.Background(), f.deps(), req)
}

func TestChangeAnchor(t *testing.T) {
	t.Parallel()

	t.Run("Without --set it prints the anchor of the named or the acting worktree and changes nothing", func(t *testing.T) {
		t.Parallel()
		f := newAnchorFake()
		f.anchors["pair1"] = "backend"

		acting, err := runAnchorFake(t, f, anchorRequest{})
		if err != nil || acting.Slug != "beam1" || acting.Anchor != "." || acting.Changed {
			t.Errorf("changeAnchor() = (%+v, %v); want the acting worktree's anchor . and no change", acting, err)
		}
		named, err := runAnchorFake(t, f, anchorRequest{Slug: "pair1"})
		if err != nil || named.Slug != "pair1" || named.Anchor != "backend" {
			t.Errorf("changeAnchor(pair1) = (%+v, %v); want the anchor backend", named, err)
		}
		if len(f.calls) != 0 {
			t.Errorf("calls = %v; want none, since a print touches nothing", f.calls)
		}
	})

	t.Run("A slug this machine does not hold is refused naming lyx fabric pairs", func(t *testing.T) {
		t.Parallel()
		f := newAnchorFake()

		_, err := runAnchorFake(t, f, anchorRequest{Slug: "nobody", Set: "sub"})

		if err == nil || !strings.Contains(err.Error(), "lyx fabric pairs") {
			t.Errorf("changeAnchor() error = %v; want a refusal naming `lyx fabric pairs`", err)
		}
		if len(f.calls) != 0 {
			t.Errorf("calls = %v; want none", f.calls)
		}
	})

	t.Run("Each refusal names its way forward and calls no engine step, ends no dobby and takes no lock", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name   string
			slug   string
			mutate func(f *anchorFake)
			want   []string
			absent []string
		}{
			{
				name: "AnotherMachineOwnsIt",
				slug: "pair1",
				mutate: func(f *anchorFake) {
					f.owners["pair1"] = "elsewhere"
				},
				want:   []string{"elsewhere", "lyx adopt pair1", "--keep-local", "lyx fabric anchor pair1 --set sub` on elsewhere"},
				absent: []string{"the marker at the fetched upstream tip"},
			},
			{
				name: "AnotherMachineOwnsItAndItsUpstreamTipRecordsAnotherAnchor",
				slug: "pair1",
				mutate: func(f *anchorFake) {
					f.owners["pair1"] = "elsewhere"
					f.upstream, f.hasUpstream = "backend", true
				},
				want: []string{"lyx fabric anchor pair1 --set backend`, the marker at the fetched upstream tip"},
			},
			{
				name: "AnotherMachineOwnsItAndItsUpstreamTipRecordsTheCheckedOutAnchor",
				slug: "pair1",
				mutate: func(f *anchorFake) {
					f.owners["pair1"] = "elsewhere"
					f.upstream, f.hasUpstream = ".", true
				},
				want:   []string{"elsewhere"},
				absent: []string{"the marker at the fetched upstream tip"},
			},
			{
				name: "AnotherMachineOwnsThePrime",
				slug: "prime",
				mutate: func(f *anchorFake) {
					f.owners["prime"] = "elsewhere"
				},
				want: []string{"lyx adopt --prime", "elsewhere"},
			},
			{
				name:   "ReedHoldsAStrand",
				slug:   "pair1",
				mutate: func(f *anchorFake) { f.reed["pair1"] = true },
				want:   []string{"lyx reed down", "pair1"},
			},
			{
				name:   "TheDriverLockIsHeld",
				slug:   "pair1",
				mutate: func(f *anchorFake) { f.driverLocked = true },
				want:   []string{"driver lock", "lyx batten cancel pair1"},
			},
			{
				name:   "ABattenRunIsLive",
				slug:   "pair1",
				mutate: func(f *anchorFake) { f.battenLive = true },
				want:   []string{"batten run", "lyx batten cancel pair1"},
			},
			{
				name:   "TheRunStatusIsUnfinished",
				slug:   "pair1",
				mutate: func(f *anchorFake) { f.runOpen = true },
				want:   []string{"unfinished run", "lyx batten cancel pair1", "an entry stopped only by this change"},
			},
			{
				name:   "ThePrimesBattenStatusOfThePairIsUnfinished",
				slug:   "pair1",
				mutate: func(f *anchorFake) { f.primeRunOpen = true },
				want:   []string{"unfinished in the prime", "lyx batten cancel pair1"},
			},
			{
				name: "ARunThatReadsUnfinishedAtTheFetchedUpstreamTipOfAWorktreeAnotherMachineOwns",
				slug: "pair1",
				mutate: func(f *anchorFake) {
					f.owners["pair1"] = "elsewhere"
					f.upstream, f.hasUpstream = "sub", true
					f.upstreamOpen = true
					f.runOpen = false
				},
				want:   []string{"fetched upstream tip", "lyx batten cancel pair1` on elsewhere", "lyx fabric pull", "the marker at that tip"},
				absent: []string{"lyx batten cancel pair1`, which an entry stopped only by this change"},
			},
			{
				name:   "ALiveDobbyOnWindows",
				slug:   "beam1",
				mutate: func(f *anchorFake) { f.goos = "windows"; f.dobbyPIDs["beam1"] = 4321 },
				want:   []string{"4321", "taskkill /PID 4321 /T /F", "lyx fabric anchor beam1 --set sub"},
			},
			{
				name:   "ReedHoldsAStrandSomewhereInTheHubForThePrime",
				slug:   "prime",
				mutate: func(f *anchorFake) { f.hubReed = []string{"orch", "beam2"} },
				want:   []string{"orch, beam2", "lyx reed down", "the orch's included"},
			},
			{
				name:   "ABattenRunIsLiveSomewhereInTheHubForThePrime",
				slug:   "prime",
				mutate: func(f *anchorFake) { f.hubBatten = []string{"task-a"} },
				want:   []string{"task-a", "lyx batten status", "lyx batten pause"},
			},
			{
				name:   "AGateSlotIsHeldForThePrime",
				slug:   "prime",
				mutate: func(f *anchorFake) { f.gates = []string{"slot 1 (beam2, gate test, process 7)"} },
				want:   []string{"slot 1 (beam2, gate test, process 7)", "wait for each holder"},
			},
			{
				name:   "TheHibernateDaemonIsLiveForThePrime",
				slug:   "prime",
				mutate: func(f *anchorFake) { f.hibernating = true },
				want:   []string{"lyx hibernate cancel"},
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				f := newAnchorFake()
				tt.mutate(f)

				_, err := runAnchorFake(t, f, anchorRequest{Slug: tt.slug, Set: "sub"})

				if err == nil {
					t.Fatalf("changeAnchor() succeeded; want a refusal")
				}
				for _, want := range tt.want {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("changeAnchor() error = %q; want it to contain %q", err, want)
					}
				}
				for _, absent := range tt.absent {
					if strings.Contains(err.Error(), absent) {
						t.Errorf("changeAnchor() error = %q; want it to omit %q", err, absent)
					}
				}
				for _, work := range []string{"begin", "parent-lock", "end-dobbies", "apply", "reopen-log"} {
					if f.did(work) {
						t.Errorf("calls = %v; want no %s after a refusal", f.calls, work)
					}
				}
			})
		}
	})

	t.Run("A worktree another machine owns is not refused for the anchor its fetched upstream tip records or for a standing move record", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name   string
			mutate func(f *anchorFake)
		}{
			{
				name: "TheUpstreamTipRecordsTheRequestedAnchor",
				mutate: func(f *anchorFake) {
					f.owners["pair1"] = "elsewhere"
					f.upstream, f.hasUpstream = "sub", true
				},
			},
			{
				name: "AMoveRecordStandsAndTheUpstreamRecordsAnother",
				mutate: func(f *anchorFake) {
					f.owners["pair1"] = "elsewhere"
					f.upstream, f.hasUpstream = "backend", true
					f.recordFound = true
				},
			},
			{
				name: "TheCheckedOutRunReadsUnfinishedAndTheUpstreamTipsFinished",
				mutate: func(f *anchorFake) {
					f.owners["pair1"] = "elsewhere"
					f.upstream, f.hasUpstream = "sub", true
					f.runOpen, f.upstreamOpen = true, false
				},
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				f := newAnchorFake()
				tt.mutate(f)

				result, err := runAnchorFake(t, f, anchorRequest{Slug: "pair1", Set: "sub"})

				if err != nil || !result.Changed {
					t.Errorf("changeAnchor() = (%+v, %v); want the move to go through", result, err)
				}
			})
		}
	})

	t.Run("A move of a beam runs its steps in order and starts the dobby again", func(t *testing.T) {
		t.Parallel()
		f := newAnchorFake()
		f.passPolls = 2

		result, err := runAnchorFake(t, f, anchorRequest{Set: "sub"})

		if err != nil {
			t.Fatalf("changeAnchor() error = %v; want nil", err)
		}
		want := []string{
			"begin sub",
			"note waiting for a pass of the dobby of beam1", "wait",
			"note waiting for a pass of the dobby of beam1", "wait",
			"parent-lock",
			"end-dobbies beam1 spare " + runproc.RolePush + "," + runproc.RoleSync,
			"reopen-log " + f.lockDir("beam1"),
			"apply",
			"reopen-log " + f.logsDir("beam1", "sub"),
			"finish",
			"start-dobby beam1",
			"release-parent-lock",
			"close",
		}
		if !slices.Equal(f.calls, want) {
			t.Errorf("calls =\n%s\nwant\n%s", strings.Join(f.calls, "\n"), strings.Join(want, "\n"))
		}
		if !result.Changed || result.From != "." || result.Anchor != "sub" || result.DobbyStartError != "" || result.PushError != "" {
			t.Errorf("result = %+v; want a change from . to sub with nothing to report", result)
		}
	})

	t.Run("A move of the prime ends every dobby of the machine and ensures them live afterwards", func(t *testing.T) {
		t.Parallel()
		f := newAnchorFake()

		_, err := runAnchorFake(t, f, anchorRequest{Slug: "prime", Set: "sub"})

		if err != nil {
			t.Fatalf("changeAnchor() error = %v; want nil", err)
		}
		if !f.did("end-dobbies prime,beam1,beam2 spare push,sync") || !f.did("ensure-dobbies") {
			t.Errorf("calls = %v; want every dobby ended with push and sync spared and ensured live after", f.calls)
		}
	})

	t.Run("A pair that is no beam has no dobby to end or start", func(t *testing.T) {
		t.Parallel()
		f := newAnchorFake()

		if _, err := runAnchorFake(t, f, anchorRequest{Slug: "pair1", Set: "sub"}); err != nil {
			t.Fatalf("changeAnchor() error = %v; want nil", err)
		}

		for _, call := range []string{"end-dobbies", "start-dobby", "ensure-dobbies"} {
			if f.did(call) {
				t.Errorf("calls = %v; want no %s", f.calls, call)
			}
		}
	})

	t.Run("A worktree already at the anchor is reported as a no-op, ending no dobby", func(t *testing.T) {
		t.Parallel()
		f := newAnchorFake()
		f.noop = true

		result, err := runAnchorFake(t, f, anchorRequest{Set: "."})

		if err != nil || !result.NoOp || result.Changed {
			t.Errorf("changeAnchor() = (%+v, %v); want a no-op", result, err)
		}
		if !slices.Equal(f.calls, []string{"begin .", "close"}) {
			t.Errorf("calls = %v; want only begin and close", f.calls)
		}
	})

	t.Run("A failure before the engine lands restarts the dobbies it ended, or names the re-run over a record that stands", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name   string
			mutate func(f *anchorFake)
			// cause is a text the returned error carries.
			cause string
			// started lists the dobby starts that follow a failure on a fresh run, empty for none.
			started []string
		}{
			{
				name:   "ThePassWaitFails",
				mutate: func(f *anchorFake) { f.passErr = errors.New("probe failed") },
				cause:  "probe failed",
			},
			{
				name:   "TheParentLockFailsAfterBegin",
				mutate: func(f *anchorFake) { f.lockErr = errors.New("lock unavailable") },
				cause:  "lock unavailable",
			},
			{
				name: "TheDobbyEndFailsAfterOneDobbyEnded",
				mutate: func(f *anchorFake) {
					f.endErr, f.endedBeforeErr = errors.New("survivor"), []string{"beam1"}
				},
				cause:   "survivor",
				started: []string{"start-dobby beam1"},
			},
			{
				name:   "TheLogSinkFailsToCloseOnItsMove",
				mutate: func(f *anchorFake) { f.reopenFails, f.reopenErr = 1, errors.New("sink stuck") },
				cause:  "sink stuck",
				// The dobby end preceded it, so the dobby is started again.
				started: []string{"start-dobby beam1"},
			},
			{
				name: "ApplyFailsBeforeTheCommitAndTheRecordIsGone",
				mutate: func(f *anchorFake) {
					f.applyErr = errors.New("commit failed")
				},
				cause:   "commit failed",
				started: []string{"start-dobby beam1"},
			},
		}
		for _, tt := range tests {
			t.Run(tt.name+"OnAFreshRun", func(t *testing.T) {
				t.Parallel()
				f := newAnchorFake()
				tt.mutate(f)

				_, err := runAnchorFake(t, f, anchorRequest{Set: "sub"})

				if err == nil || !strings.Contains(err.Error(), tt.cause) {
					t.Fatalf("changeAnchor() error = %v; want it to carry %q", err, tt.cause)
				}
				if f.recordFound {
					t.Errorf("the fake's record stands after a failure on a fresh run; want Close to have removed it or Apply to have rolled back")
				}
				for _, start := range tt.started {
					if !f.did(start) {
						t.Errorf("calls = %v; want %q", f.calls, start)
					}
				}
				if len(tt.started) == 0 && f.did("start-dobby") {
					t.Errorf("calls = %v; want no dobby started, none having ended", f.calls)
				}
				if !f.did("close") {
					t.Errorf("calls = %v; want Close called", f.calls)
				}
			})
			t.Run(tt.name+"OverAStandingRecord", func(t *testing.T) {
				t.Parallel()
				f := newAnchorFake()
				f.recordFound = true
				tt.mutate(f)
				f.applyKeepsRecord = true

				_, err := runAnchorFake(t, f, anchorRequest{Set: "sub"})

				if err == nil || !strings.Contains(err.Error(), "lyx fabric anchor beam1 --set sub") {
					t.Fatalf("changeAnchor() error = %v; want it to name `lyx fabric anchor beam1 --set sub`", err)
				}
				if f.did("start-dobby") {
					t.Errorf("calls = %v; want no dobby started while the record stands", f.calls)
				}
				if f.did("parent-lock") && !f.did("release-parent-lock") {
					t.Errorf("calls = %v; want the parent lock released", f.calls)
				}
			})
		}
	})

	t.Run("An Apply failure that left the record starts no dobby, whether or not it landed", func(t *testing.T) {
		t.Parallel()
		for _, landed := range []bool{false, true} {
			t.Run(fmt.Sprintf("Landed%v", landed), func(t *testing.T) {
				t.Parallel()
				f := newAnchorFake()
				f.applyErr, f.applyLanded, f.applyKeepsRecord = errors.New("wiring failed"), landed, true

				_, err := runAnchorFake(t, f, anchorRequest{Set: "sub"})

				if err == nil || !strings.Contains(err.Error(), "lyx fabric anchor beam1 --set sub") {
					t.Errorf("changeAnchor() error = %v; want it to name the re-run of the --set", err)
				}
				if f.did("start-dobby") || !f.did("release-parent-lock") {
					t.Errorf("calls = %v; want no dobby started and the parent lock released", f.calls)
				}
				wantSink := f.logsDir("beam1", ".")
				if landed {
					wantSink = f.logsDir("beam1", "sub")
				}
				if !f.did("reopen-log " + wantSink) {
					t.Errorf("calls = %v; want the sink reopened at %s, where Apply left the worktree", f.calls, wantSink)
				}
			})
		}
	})

	t.Run("A re-run over a standing record finishes it and then starts the dobby", func(t *testing.T) {
		t.Parallel()
		f := newAnchorFake()
		f.recordFound = true

		result, err := runAnchorFake(t, f, anchorRequest{Set: "sub"})

		if err != nil || !result.Changed {
			t.Fatalf("changeAnchor() = (%+v, %v); want the re-run to finish", result, err)
		}
		finish, start := slices.Index(f.calls, "finish"), slices.Index(f.calls, "start-dobby beam1")
		if finish < 0 || start < finish {
			t.Errorf("calls = %v; want Finish and then the dobby start", f.calls)
		}
	})

	t.Run("An error reopening the sink after Apply starts no dobby and leaves the record for the re-run", func(t *testing.T) {
		t.Parallel()
		f := newAnchorFake()
		f.reopenFails, f.reopenErr = 2, errors.New("sink moved away")

		_, err := runAnchorFake(t, f, anchorRequest{Set: "sub"})

		if err == nil || !strings.Contains(err.Error(), "lyx fabric anchor beam1 --set sub") {
			t.Errorf("changeAnchor() error = %v; want it to name the re-run of the --set", err)
		}
		if !f.recordFound || f.did("start-dobby") || f.did("finish") {
			t.Errorf("calls = %v, record %v; want the record left, Finish not run and no dobby started", f.calls, f.recordFound)
		}
	})

	t.Run("A Finish error before its record removal names the re-run, and one after it only reports", func(t *testing.T) {
		t.Parallel()
		before := newAnchorFake()
		before.finishErr = errors.New("remove failed")

		_, err := runAnchorFake(t, before, anchorRequest{Set: "sub"})

		if err == nil || !strings.Contains(err.Error(), "lyx fabric anchor beam1 --set sub") || before.did("start-dobby") {
			t.Errorf("changeAnchor() = %v, calls %v; want an error naming the re-run and no dobby started", err, before.calls)
		}

		after := newAnchorFake()
		after.finishErr, after.finishRemovesRecord = errors.New("push not spawned"), true

		result, err := runAnchorFake(t, after, anchorRequest{Set: "sub"})

		if err != nil || !result.Changed || result.PushError != "push not spawned" || !after.did("start-dobby beam1") {
			t.Errorf("changeAnchor() = (%+v, %v), calls %v; want the push error reported, the dobby started and no error", result, err, after.calls)
		}
	})

	t.Run("A beam this machine does not own, and one whose lock reads held, gets no dobby start after Finish", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name   string
			mutate func(f *anchorFake)
		}{
			{"NotOwnedHere", func(f *anchorFake) { f.owners["beam1"] = "elsewhere"; f.upstream, f.hasUpstream = "sub", true }},
			{"LockHeld", func(f *anchorFake) { f.dobbyLive["beam1"] = true }},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				f := newAnchorFake()
				tt.mutate(f)

				result, err := runAnchorFake(t, f, anchorRequest{Set: "sub"})

				if err != nil || !result.Changed {
					t.Fatalf("changeAnchor() = (%+v, %v); want the move to go through", result, err)
				}
				if f.did("start-dobby") {
					t.Errorf("calls = %v; want no dobby start", f.calls)
				}
			})
		}
	})

	t.Run("A dobby start that fails after Finish is reported with lyx beam start and fails nothing", func(t *testing.T) {
		t.Parallel()
		f := newAnchorFake()
		f.startErr = errors.New("spawn refused")

		result, err := runAnchorFake(t, f, anchorRequest{Set: "sub"})

		if err != nil || !result.Changed || result.DobbyStartError != "spawn refused" {
			t.Errorf("changeAnchor() = (%+v, %v); want the start failure reported and no error", result, err)
		}
		if fields := result.fields(); fields["way_forward"] != "lyx beam start" || fields["dobby_start_error"] != "spawn refused" {
			t.Errorf("fields = %v; want the way forward `lyx beam start` beside the start failure", fields)
		}
	})
}
