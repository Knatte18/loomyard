//go:build tier0

// runnotice_test.go pins the run notices over a fake child reading: each condition and its guards, one notice per episode across ticks and a restart, the api-error revive streak, and delivery with its retries.

package dobbyengine

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/pauserecord"
)

// noticeFake is the child reading of one run notice test, and what its seams report.
type noticeFake struct {
	child ChildReading
	// childMissing and childErr are what ReadChild reports besides the child: no status file, and a read error.
	childMissing bool
	childErr     error
	alive        bool
	runs         []AgentRun
	waitLive     bool
	agentsErr    error
	pause        pauserecord.Coverage
	marker       string
	revives      int
}

// newNoticeHarness returns a pass harness over one running run, task, whose child is running at Plan-Write with a live driver and no agent run, and the fake its notice seams read.
func newNoticeHarness(t *testing.T) (*passHarness, *noticeFake) {
	t.Helper()
	h := newPassHarness(t, runningRun("task"))
	f := &noticeFake{
		child:  ChildReading{State: childStateRunning, CurrentProducer: "Plan-Write", Modified: h.clock.now(), PairDir: "/wt/task"},
		alive:  true,
		marker: filepath.Join(t.TempDir(), "notice-episode"),
	}
	h.d.deps.RunNotices = RunNoticeDeps{
		ReadChild:   func(string) (ChildReading, bool, error) { return f.child, !f.childMissing, f.childErr },
		DriverAlive: func(context.Context, string) (bool, error) { return f.alive, nil },
		Agents: func(context.Context, string) ([]AgentRun, bool, error) {
			return f.runs, f.waitLive, f.agentsErr
		},
		Pause:      func(string) (pauserecord.Coverage, error) { return f.pause, nil },
		MarkerPath: func(string) string { return f.marker },
		Quiet:      45 * time.Minute,
	}
	return h, f
}

// wireRevive makes the harness's dobby revive API-error stalls through a seam that counts its calls.
func (f *noticeFake) wireRevive(h *passHarness) {
	h.d.deps.RunNotices.ReviveAPIError = func(context.Context, string) error {
		f.revives++
		return nil
	}
}

// advance moves the harness's wall clock forward by d with the machine awake.
func (h *passHarness) advance(d time.Duration) {
	h.clock.sleep(context.Background(), d)
}

// restartDobby replaces the harness's dobby with a fresh one over the same state record, seams and probe interval, as a re-exec does.
func (h *passHarness) restartDobby() {
	h.d = &daemon{geom: h.d.geom, deps: h.d.deps, probe: h.d.probe, flights: map[string]flight{}, childSeen: map[string]time.Time{}, wasCovered: map[string]bool{}, liftPending: map[string]bool{}}
}

// runNotices returns the run notices the harness's dobby posted.
func (h *passHarness) runNotices() []string {
	var out []string
	for _, line := range h.fake.notices {
		if strings.HasPrefix(line, runNoticePrefix) {
			out = append(out, line)
		}
	}
	return out
}

func TestRunNotice_Conditions(t *testing.T) {
	t.Parallel()
	start := newLoopClock().now()
	stalled := func(text string) AgentRun {
		return AgentRun{Producer: "t:task:impl", LastActivity: start, APIError: true, APIErrorText: text}
	}

	tests := []struct {
		name string
		// act sets the reading and moves the clock before the two probe ticks the test runs.
		act func(h *passHarness, f *noticeFake)
		// want is the one notice expected, or every substring it must hold; none expects no notice.
		want  []string
		exact bool
	}{
		{
			name: "ADeadDriverReadsTheChildsStateAndReasonRelativeToThePair",
			act: func(_ *passHarness, f *noticeFake) {
				f.alive = false
				f.child.Error, f.child.HistoryLen = "waits on /wt/task/_lym/x.md", 4
			},
			want:  []string{"[dobby notice] task: running at Plan-Write; driver strand is dead; waiting: waits on _lym/x.md; since 2026-10-25T12:00:00Z; history 4"},
			exact: true,
		},
		{
			name: "ADeadDriverAtTheRespawnCapNamesTheCapAndLoomStart",
			act: func(_ *passHarness, f *noticeFake) {
				f.alive = false
				f.child.RespawnCap = 3
			},
			want: []string{`running at Plan-Write; driver strand is dead; respawned 3 times without progress; run "lym loom start" in the task worktree`},
		},
		{
			name: "ACoveredChildSendsNothing",
			act: func(_ *passHarness, f *noticeFake) {
				f.alive = false
				f.pause = pauserecord.Coverage{Covered: true}
			},
		},
		{
			name: "AChildThatIsNotRunningSendsNothing",
			act: func(_ *passHarness, f *noticeFake) {
				f.alive = false
				f.child.State = "blocked"
			},
		},
		{
			name: "ARunWhoseBattenStateIsNotRunningSendsNothing",
			act: func(h *passHarness, f *noticeFake) {
				f.alive = false
				h.fake.runs[0].State = "blocked"
			},
		},
		{
			name: "ARunWithACancelMarkerSendsNothing",
			act: func(h *passHarness, f *noticeFake) {
				f.alive = false
				h.fake.runs[0].CancelMarker = true
			},
		},
		{
			name: "AChildWithNoStatusFileSendsNothing",
			act: func(_ *passHarness, f *noticeFake) {
				f.alive = false
				f.childMissing = true
			},
		},
		{
			name: "AChildThatCannotBeReadSendsNothingAndFailsNoTick",
			act: func(_ *passHarness, f *noticeFake) {
				f.alive = false
				f.childErr = errors.New("status file unreadable")
			},
		},
		{
			name: "ANilReadChildSendsNothing",
			act: func(h *passHarness, f *noticeFake) {
				f.alive = false
				h.d.deps.RunNotices.ReadChild = nil
			},
		},
		{
			name: "IdleAgentsGoQuietAtTheWindowMeasuredFromTheNewestRun",
			act: func(h *passHarness, f *noticeFake) {
				f.runs = []AgentRun{{Producer: "t:task:impl", LastActivity: start}, {Producer: "t:task:review", LastActivity: start.Add(-time.Hour)}}
				h.advance(45 * time.Minute)
			},
			want: []string{"running at Plan-Write; agents idle for 45m10s"},
		},
		{
			name: "IdleAgentsInsideTheWindowSendNothing",
			act: func(h *passHarness, f *noticeFake) {
				f.runs = []AgentRun{{Producer: "t:task:impl", LastActivity: start}}
				h.advance(44 * time.Minute)
			},
		},
		{
			name: "AZeroQuietWindowSendsNoQuietNoticeForAgentRuns",
			act: func(h *passHarness, f *noticeFake) {
				f.runs = []AgentRun{{Producer: "t:task:impl", LastActivity: start}}
				h.d.deps.RunNotices.Quiet = 0
				h.advance(2 * time.Hour)
			},
		},
		{
			name: "AZeroQuietWindowSendsNoQuietNoticeWhenNoRunIsRead",
			act: func(h *passHarness, _ *noticeFake) {
				h.d.deps.RunNotices.Quiet = 0
				h.advance(2 * time.Hour)
			},
		},
		{
			name: "ALiveWaitHoldsTheQuietNotice",
			act: func(h *passHarness, f *noticeFake) {
				f.runs, f.waitLive = []AgentRun{{Producer: "t:task:impl", LastActivity: start}}, true
				h.advance(2 * time.Hour)
			},
		},
		{
			name: "NoRunReadFallsBackToTheNewestHistoryEntry",
			act: func(h *passHarness, f *noticeFake) {
				f.child.NewestHistory = start.Add(30 * time.Minute)
				h.advance(75 * time.Minute)
			},
			want: []string{"running at Plan-Write; no agent activity readable for 45m10s"},
		},
		{
			name: "NoRunReadInsideTheWindowOfTheNewestHistoryEntrySendsNothing",
			act: func(h *passHarness, f *noticeFake) {
				f.child.NewestHistory = start.Add(30 * time.Minute)
				h.advance(60 * time.Minute)
			},
		},
		{
			name: "AnAPIErrorGoesOnceItHasStoodItsIdleTime",
			act: func(h *passHarness, f *noticeFake) {
				f.runs = []AgentRun{stalled("overloaded\nplease retry"), {Producer: "t:task:review", LastActivity: start.Add(-time.Hour)}}
				h.advance(apiErrorIdle)
			},
			want: []string{"running at Plan-Write; agent t:task:impl hit an API error: overloaded please retry"},
		},
		{
			name: "AnAPIErrorBeforeItsIdleTimeSendsNothing",
			act: func(h *passHarness, f *noticeFake) {
				f.runs = []AgentRun{stalled("overloaded")}
				h.advance(apiErrorIdle - 30*time.Second)
			},
		},
		{
			name: "AnAPIErrorOutranksQuiet",
			act: func(h *passHarness, f *noticeFake) {
				f.runs = []AgentRun{stalled("overloaded")}
				h.advance(2 * time.Hour)
			},
			want: []string{"hit an API error: overloaded"},
		},
		{
			name: "AUsageLimitOutranksAnAPIError",
			act: func(h *passHarness, f *noticeFake) {
				limited := stalled("You've hit your weekly limit")
				limited.Producer, limited.UsageLimited, limited.UsageLimitResetsAt = "t:task:review", true, start.Add(3*time.Hour)
				f.runs = []AgentRun{stalled("overloaded"), limited}
				h.advance(apiErrorIdle)
			},
			want: []string{"running at Plan-Write; agent t:task:review hit a usage limit, resets 2026-10-25T15:00:00Z: You've hit your weekly limit"},
		},
		{
			name: "AUsageLimitWithNoReadableResetSaysSo",
			act: func(h *passHarness, f *noticeFake) {
				limited := stalled("limit reached")
				limited.UsageLimited = true
				f.runs = []AgentRun{limited}
				h.advance(apiErrorIdle)
			},
			want: []string{"hit a usage limit, reset time unknown: limit reached"},
		},
		{
			name: "AnUnreadableActivitySendsNothing",
			act: func(h *passHarness, f *noticeFake) {
				f.agentsErr = errors.New("shuttle config unreadable")
				h.advance(2 * time.Hour)
			},
		},
		{
			name: "APausedIntervalIsNotIdleTime",
			act: func(h *passHarness, f *noticeFake) {
				f.runs = []AgentRun{{Producer: "t:task:impl", LastActivity: start}}
				f.pause = pauserecord.Coverage{Intervals: []pauserecord.Interval{{Start: start, End: start.Add(time.Hour)}}}
				h.advance(time.Hour + 44*time.Minute)
			},
		},
		{
			name: "ASleepIsNotIdleTime",
			act: func(h *passHarness, f *noticeFake) {
				f.runs = []AgentRun{{Producer: "t:task:impl", LastActivity: start}}
				h.tick(false)
				h.clock.suspend(3 * time.Hour)
				h.tick(false)
			},
		},
		{
			name: "TheAwakeIdleTimeAfterASleepReachesTheWindow",
			act: func(h *passHarness, f *noticeFake) {
				f.runs = []AgentRun{{Producer: "t:task:impl", LastActivity: start}}
				h.tick(false)
				h.clock.suspend(3 * time.Hour)
				h.tick(false)
				h.advance(45 * time.Minute)
			},
			want: []string{"agents idle for 45m"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h, f := newNoticeHarness(t)
			tt.act(h, f)
			h.tick(true)
			h.tick(true)

			got := h.runNotices()
			if len(tt.want) == 0 {
				if len(got) != 0 {
					t.Errorf("run notices = %q; want none", got)
				}
				return
			}
			if len(got) != 1 {
				t.Fatalf("run notices = %q; want exactly one across two probe ticks", got)
			}
			if tt.exact && got[0] != tt.want[0] {
				t.Errorf("run notice = %q; want %q", got[0], tt.want[0])
			}
			for _, want := range tt.want {
				if !strings.Contains(got[0], want) {
					t.Errorf("run notice = %q; want it to hold %q", got[0], want)
				}
			}
			if strings.Contains(got[0], "3h") {
				t.Errorf("run notice = %q; want no slept time in the idle time", got[0])
			}
		})
	}
}

func TestRunNotice_OncePerEpisode(t *testing.T) {
	t.Parallel()
	h, f := newNoticeHarness(t)
	f.alive = false
	since := f.child.Modified

	h.tick(false)
	if got := h.runNotices(); len(got) != 0 {
		t.Fatalf("run notices after a tick that is no probe tick = %q; want none", got)
	}
	h.tick(true)
	h.tick(true)
	if got := h.runNotices(); len(got) != 1 {
		t.Fatalf("run notices = %q; want one driver-dead notice across two probe ticks", got)
	}
	wantKey := conditionDriverDead + "|running|" + strconv.FormatInt(since.UnixNano(), 10)
	if keys := readNoticeKeys(f.marker); !slices.Equal(keys, []string{wantKey}) {
		t.Fatalf("marker keys = %q; want %q", keys, wantKey)
	}

	// A restarted dobby reads the memory and the marker, and sends nothing more.
	// That holds when the child's status file was rewritten while it stayed running, because the episode's start comes from the memory.
	h.advance(time.Minute)
	f.child.Modified = h.clock.now()
	h.restartDobby()
	h.tick(true)
	if got := h.runNotices(); len(got) != 1 {
		t.Fatalf("run notices after a restart = %q; want no second notice", got)
	}

	// The child leaving running drops the key, and a later running episode is noticed again.
	f.child.State, f.child.Modified = "blocked", h.clock.now()
	h.tick(true)
	if keys := readNoticeKeys(f.marker); len(keys) != 0 {
		t.Fatalf("marker keys after the child halted = %q; want none", keys)
	}
	f.child.State, f.child.Modified = childStateRunning, h.clock.now()
	h.tick(true)
	if got := h.runNotices(); len(got) != 2 {
		t.Fatalf("run notices = %q; want a second notice for the new running episode", got)
	}
}

func TestRunNotice_APIErrorRevive(t *testing.T) {
	t.Parallel()
	// newEpisode stalls an agent at the clock's time and lets the error stand its idle time.
	newEpisode := func(h *passHarness, f *noticeFake) {
		f.runs = []AgentRun{{Producer: "t:task:impl", LastActivity: h.clock.now(), APIError: true, APIErrorText: "overloaded"}}
		h.advance(apiErrorIdle)
	}

	t.Run("TheFirstSightRevivesAndTheEpisodeStillStandingAtTheNextProbeTickNotices", func(t *testing.T) {
		t.Parallel()
		h, f := newNoticeHarness(t)
		f.wireRevive(h)
		newEpisode(h, f)
		h.tick(true)
		if f.revives != 1 || len(h.runNotices()) != 0 {
			t.Fatalf("revives = %d, run notices = %q; want one revive and no notice at the first sight", f.revives, h.runNotices())
		}
		h.tick(true)
		h.tick(true)
		if got := h.runNotices(); f.revives != 1 || len(got) != 1 || !strings.Contains(got[0], "API error") {
			t.Fatalf("revives = %d, run notices = %q; want the one revive and one api-error notice", f.revives, got)
		}
	})

	t.Run("AnEpisodeThatEndsAfterTheReviveNoticesNothing", func(t *testing.T) {
		t.Parallel()
		h, f := newNoticeHarness(t)
		f.wireRevive(h)
		newEpisode(h, f)
		h.tick(true)
		f.runs = []AgentRun{{Producer: "t:task:impl", LastActivity: h.clock.now()}}
		h.tick(true)
		h.tick(true)
		if f.revives != 1 || len(h.runNotices()) != 0 {
			t.Fatalf("revives = %d, run notices = %q; want one revive and no notice once the session was active again", f.revives, h.runNotices())
		}
	})

	t.Run("AReviveAndTheEpisodeMemorySurviveARestartThroughTheStateRecord", func(t *testing.T) {
		t.Parallel()
		h, f := newNoticeHarness(t)
		f.wireRevive(h)
		newEpisode(h, f)
		h.tick(true)

		current, _, err := ReadState(h.d.geom.StatePath)
		if err != nil {
			t.Fatalf("ReadState() = %v; want nil", err)
		}
		recorded := current.Runs["task"].Notice
		if recorded.ChildState != childStateRunning || !recorded.Since.Equal(f.child.Modified) || recorded.APIReviveKey == "" || recorded.APIRevives != 1 || !recorded.APIReviveStatusTime.Equal(f.child.Modified) {
			t.Fatalf("recorded notice memory = %+v; want the running episode since %v, the revived key, one revive and the status time %v", recorded, f.child.Modified, f.child.Modified)
		}

		h.restartDobby()
		h.tick(true)
		if got := h.runNotices(); f.revives != 1 || len(got) != 1 || !strings.Contains(got[0], "API error") {
			t.Fatalf("revives = %d, run notices = %q; want the restarted dobby to read the revive back, revive nothing more and notice the episode still standing", f.revives, got)
		}
	})

	tests := []struct {
		name string
		// writeAfter is the revived episode after which the child's status file is written, zero for none.
		writeAfter int
		// restartAfter is the revived episode after which the dobby restarts, zero for none.
		restartAfter int
		wantNotices  int
	}{
		{name: "ThreeRevivedEpisodesWithNoStatusWriteNoticeOnTheThird", wantNotices: 1},
		{name: "AStatusWriteBetweenThemResetsTheCount", writeAfter: 2, wantNotices: 0},
		{name: "ARestartBetweenThemKeepsTheCount", restartAfter: 2, wantNotices: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h, f := newNoticeHarness(t)
			f.wireRevive(h)
			for episode := 1; episode <= apiErrorReviveStreak; episode++ {
				newEpisode(h, f)
				h.tick(true)
				if tt.writeAfter == episode {
					f.child.Modified = h.clock.now()
				}
				if tt.restartAfter == episode {
					h.restartDobby()
				}
			}
			if got := h.runNotices(); f.revives != apiErrorReviveStreak || len(got) != tt.wantNotices {
				t.Errorf("revives = %d, run notices = %q; want %d revives and %d notices", f.revives, got, apiErrorReviveStreak, tt.wantNotices)
			}
		})
	}
}

func TestRunNotice_DeliveryIsRetriedOnProbeTicksThenDropped(t *testing.T) {
	t.Parallel()
	h, f := newNoticeHarness(t)
	f.alive = false
	attempts := 0
	h.d.deps.Notice = func(_ context.Context, line string) error {
		if strings.HasPrefix(line, runNoticePrefix) {
			attempts++
		}
		return errors.New("no orch strand is recorded to receive the notice")
	}

	for range runNoticeMaxAttempts + 2 {
		h.tick(true)
		h.tick(false)
	}
	if attempts != runNoticeMaxAttempts {
		t.Errorf("attempts = %d; want %d, one per probe tick, then the notice dropped", attempts, runNoticeMaxAttempts)
	}
	if keys := readNoticeKeys(f.marker); len(keys) != 0 {
		t.Errorf("marker keys = %q; want none for a notice that was never posted", keys)
	}

	// The dropped notice's attempts are kept while its run is listed, and forgotten once it leaves the listing.
	if len(h.d.deliveries) != 1 {
		t.Fatalf("delivery records = %d; want the dropped notice's one while its run is listed", len(h.d.deliveries))
	}
	h.fake.runs = nil
	h.tick(true)
	if len(h.d.deliveries) != 0 {
		t.Errorf("delivery records = %d after the run left the listing; want none, so a later run with the same slug starts at zero attempts", len(h.d.deliveries))
	}
}
