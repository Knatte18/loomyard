// gatehold_test.go pins the gate-hold notice over fake slot holders and process reads.
// It covers the bound on awake time, the one notice per hold, the line and the retries after a failed read or post.

package dobbyengine

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// gateHoldFake is what the gate-hold seams of one test report, and what they were given.
type gateHoldFake struct {
	holders     []GateHolder
	holdersErr  error
	holderCalls int
	trees       map[int][]proc.ProcessSample
	treeErr     error
	marked      []int
	markedErr   error
	worktreeErr error
	cfg         Config
	cfgErr      error
	postErr     error
	posted      []string
}

// newGateHoldHarness returns a pass harness over one running run, task, whose task worktree is /hub/task,
// and a fake holding one tier3 slot under it since the harness clock's start.
func newGateHoldHarness(t *testing.T) (*passHarness, *gateHoldFake) {
	t.Helper()
	h := newPassHarness(t, runningRun("task"))
	f := &gateHoldFake{
		holders: []GateHolder{{
			Slot: 1, Worktree: "/hub/task/sub", Site: "gate test tier3", Tier: "tier3", Marker: "LYM_GATE_TEST_PID=100",
			PID: 100, Started: h.clock.now(), TierTimeout: 35 * time.Minute,
		}},
		trees: map[int][]proc.ProcessSample{
			100: {
				{PID: 100, Path: "/bin/lym", CPU: time.Second},
				{PID: 101, Parent: 100, Path: "/bin/go", CPU: 2 * time.Second},
				{PID: 102, Parent: 101, Path: "/tmp/x.test", CPU: 3 * time.Second},
			},
			200: {{PID: 200, Path: "/usr/bin/tmux", CPU: 4 * time.Second}},
		},
		marked: []int{101, 200},
		cfg:    Config{GateHoldFactor: 1.5, GateHoldMin: 30},
	}
	h.d.deps.LoadConfig = func() (Config, error) { return f.cfg, f.cfgErr }
	h.d.deps.GateHolds = GateHoldDeps{
		Holders: func() ([]GateHolder, error) {
			f.holderCalls++
			return slices.Clone(f.holders), f.holdersErr
		},
		TaskWorktree: func(slug string) (string, error) { return "/hub/" + slug, f.worktreeErr },
		Tree:         func(pid int) ([]proc.ProcessSample, error) { return f.trees[pid], f.treeErr },
		MarkedPIDs:   func(string) ([]int, error) { return f.marked, f.markedErr },
		Post: func(_ context.Context, _, line string) error {
			if f.postErr != nil {
				return f.postErr
			}
			f.posted = append(f.posted, line)
			return nil
		},
	}
	return h, f
}

// tickAt runs one probe tick at the wall time since past start.
func (h *passHarness) tickAt(start time.Time, since time.Duration) {
	h.t.Helper()
	// tick itself advances the clock by one probe interval.
	h.advance(start.Add(since).Sub(h.clock.now()) - 10*time.Second)
	h.tick(true)
}

func TestGateHolds(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const line = "[dobby notice] task: a gate slot is held past its bound: slots 1; tier tier3; site gate test tier3; " +
		"started 2026-10-25T12:00:00Z; bound 52m30s; holder pid 100 /bin/lym cpu 1s; " +
		"child pid 102 /tmp/x.test cpu 3s; child pid 101 /bin/go cpu 2s; " +
		"outside the tree, carrying LYM_GATE_TEST_PID=100: pid 200 /usr/bin/tmux cpu 4s"
	past := 53 * time.Minute
	requirePosts := func(t *testing.T, f *gateHoldFake, want int) {
		t.Helper()
		if len(f.posted) != want {
			t.Fatalf("posted = %q; want %d notices", f.posted, want)
		}
	}
	requireHolds := func(t *testing.T, f *gateHoldFake, want ...string) {
		t.Helper()
		requirePosts(t, f, 1)
		for _, part := range want {
			if !strings.Contains(f.posted[0], part) {
				t.Errorf("notice = %q; want it to hold %q", f.posted[0], part)
			}
		}
	}
	unreadable := func(fail func(f *gateHoldFake)) func(*testing.T, *passHarness, *gateHoldFake, time.Time) {
		return func(t *testing.T, h *passHarness, f *gateHoldFake, start time.Time) {
			fail(f)
			h.tickAt(start, past)
			h.tickAt(start, past+time.Minute)
			requireHolds(t, f, "holder pid 100; process tree unreadable")
		}
	}

	tests := []struct {
		name string
		run  func(t *testing.T, h *passHarness, f *gateHoldFake, start time.Time)
	}{
		{"PostsTheLineOnceTheAwakeBoundPasses", func(t *testing.T, h *passHarness, f *gateHoldFake, start time.Time) {
			h.tickAt(start, 52*time.Minute)
			requirePosts(t, f, 0)
			h.tickAt(start, past)
			if want := []string{line}; !slices.Equal(f.posted, want) {
				t.Errorf("posted = %q; want %q", f.posted, want)
			}
		}},
		{"AMachineSleepDelaysTheNotice", func(t *testing.T, h *passHarness, f *gateHoldFake, start time.Time) {
			h.tick(true)
			h.clock.suspend(2 * time.Minute)
			h.tickAt(start, past)
			requirePosts(t, f, 0)
			h.tickAt(start, past+2*time.Minute)
			requirePosts(t, f, 1)
		}},
		{"OneHoldIsNoticedOnceAcrossTicksAndARestart", func(t *testing.T, h *passHarness, f *gateHoldFake, start time.Time) {
			h.tickAt(start, past)
			h.tick(true)
			h.restartDobby()
			h.tick(true)
			requirePosts(t, f, 1)
		}},
		{"TwoSlotsOfOneProcessAreOneHoldFromItsEarliestStart", func(t *testing.T, h *passHarness, f *gateHoldFake, start time.Time) {
			second := f.holders[0]
			second.Slot, second.TierTimeout = 2, 40*time.Minute
			f.holders[0].Started = start.Add(10 * time.Minute)
			f.holders = append(f.holders, second)
			h.tickAt(start, 59*time.Minute)
			requirePosts(t, f, 0)
			h.tickAt(start, 61*time.Minute)
			h.tick(true)
			requireHolds(t, f, "slots 1,2;", "started 2026-10-25T12:00:00Z;", "bound 1h0m0s;")
		}},
		{"AnotherProcessOrALaterStartIsANewHold", func(t *testing.T, h *passHarness, f *gateHoldFake, start time.Time) {
			h.tickAt(start, past)
			f.holders[0].PID = 300
			h.tick(true)
			requirePosts(t, f, 2)
			f.holders[0].Started = start.Add(time.Minute)
			h.tickAt(start, past+time.Minute)
			requirePosts(t, f, 3)
			want := []string{"300|" + strconv.FormatInt(start.Add(time.Minute).UnixNano(), 10)}
			if got := h.stateRecord().Watches["task"].GateHolds; !slices.Equal(got, want) {
				t.Errorf("GateHolds = %q; want only the current hold %q", got, want)
			}
		}},
		{"AHolderInTheTaskWorktreeItselfIsTheRuns", func(t *testing.T, h *passHarness, f *gateHoldFake, start time.Time) {
			f.holders[0].Worktree = "/hub/task"
			h.tickAt(start, past)
			requirePosts(t, f, 1)
		}},
		{"AFactorPastTheDurationRangeNoticesNothing", func(t *testing.T, h *passHarness, f *gateHoldFake, start time.Time) {
			f.cfg.GateHoldFactor = 1e10
			h.tickAt(start, past)
			requirePosts(t, f, 0)
		}},
		{"ASiblingWorktreeIsNotTheRuns", func(t *testing.T, h *passHarness, f *gateHoldFake, start time.Time) {
			f.holders[0].Worktree = "/hub/task-b"
			h.tickAt(start, past)
			requirePosts(t, f, 0)
		}},
		{"AHolderWithNoTierTakesTheMinutesBound", func(t *testing.T, h *passHarness, f *gateHoldFake, start time.Time) {
			f.holders[0].Tier, f.holders[0].TierTimeout = "", 0
			h.tickAt(start, 29*time.Minute)
			requirePosts(t, f, 0)
			h.tickAt(start, 30*time.Minute)
			requireHolds(t, f, "tier none;", "bound 30m0s;")
		}},
		{"FactorZeroSendsNothing", func(t *testing.T, h *passHarness, f *gateHoldFake, start time.Time) {
			f.cfg.GateHoldFactor = 0
			h.tickAt(start, past)
			requirePosts(t, f, 0)
		}},
		{"UnreadableHoldersAreReadAgain", func(t *testing.T, h *passHarness, f *gateHoldFake, start time.Time) {
			f.holdersErr = errors.New("no pool")
			h.tickAt(start, past)
			requirePosts(t, f, 0)
			f.holdersErr = nil
			h.tick(true)
			requirePosts(t, f, 1)
		}},
		{"AFailingConfigIsLoadedAgain", func(t *testing.T, h *passHarness, f *gateHoldFake, _ time.Time) {
			// A tick itself fails on a config that does not load, so the step is called on its own.
			h.advance(past)
			h.d.probeDue = true
			h.d.listedRuns = []RunView{runningRun("task")}
			f.cfgErr = errors.New("bad config")
			h.d.gateHolds(context.Background(), h.clock.now())
			requirePosts(t, f, 0)
			f.cfgErr = nil
			h.d.gateHolds(context.Background(), h.clock.now())
			requirePosts(t, f, 1)
		}},
		{"ATickThatIsNoProbeReadsNothing", func(t *testing.T, h *passHarness, f *gateHoldFake, _ time.Time) {
			h.advance(past)
			h.tick(false)
			requirePosts(t, f, 0)
			if f.holderCalls != 0 {
				t.Errorf("Holders calls = %d; want 0", f.holderCalls)
			}
		}},
		{"AFailingTreeStillPosts", unreadable(func(f *gateHoldFake) { f.treeErr = errors.New("no proc") })},
		{"AFailingMarkerScanStillPosts", unreadable(func(f *gateHoldFake) { f.markedErr = errors.New("no proc") })},
		{"AnUnresolvedTaskWorktreeSkipsTheRun", func(t *testing.T, h *passHarness, f *gateHoldFake, start time.Time) {
			f.worktreeErr = errors.New("no pair")
			h.tickAt(start, past)
			requirePosts(t, f, 0)
		}},
		{"AFailedPostIsTriedAgainAndThenRecorded", func(t *testing.T, h *passHarness, f *gateHoldFake, start time.Time) {
			f.postErr = errors.New("no orch")
			h.tickAt(start, past)
			if got := h.stateRecord().Watches["task"].GateHolds; len(got) != 0 {
				t.Errorf("GateHolds after a failed post = %q; want none", got)
			}
			f.postErr = nil
			h.tick(true)
			requirePosts(t, f, 1)
			want := []string{"100|" + strconv.FormatInt(start.UnixNano(), 10)}
			if got := h.stateRecord().Watches["task"].GateHolds; !slices.Equal(got, want) {
				t.Errorf("GateHolds = %q; want %q", got, want)
			}
		}},
		{"ALongProcessListIsCutAndCounted", func(t *testing.T, h *passHarness, f *gateHoldFake, start time.Time) {
			for i := range 23 {
				f.trees[100] = append(f.trees[100], proc.ProcessSample{PID: 400 + i, Parent: 100, Path: "/bin/sh"})
			}
			f.marked = nil
			h.tickAt(start, past)
			requireHolds(t, f, "child pid 102 /tmp/x.test cpu 3s; child pid 101 /bin/go cpu 2s;")
			if got := strings.Count(f.posted[0], "; child pid "); got != 20 {
				t.Errorf("descendants named = %d; want 20", got)
			}
			if !strings.HasSuffix(f.posted[0], "; and 5 more") {
				t.Errorf("notice = %q; want it to end with the count of the rest", f.posted[0])
			}
		}},
		{"AWatchLevelOfOffStillGetsTheNotice", func(t *testing.T, h *passHarness, f *gateHoldFake, start time.Time) {
			h.d.deps.RunWatch = RunWatchDeps{
				Level: func(string) (string, error) { return RunWatchOff, nil },
				Post:  func(context.Context, string, string) (bool, error) { return true, nil },
			}
			h.tickAt(start, past)
			requirePosts(t, f, 1)
			// The run watch rewrites the memory that holds the noticed holds on every tick.
			h.tick(true)
			h.restartDobby()
			h.tick(true)
			requirePosts(t, f, 1)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h, f := newGateHoldHarness(t)
			tt.run(t, h, f, h.clock.now())
		})
	}
}
