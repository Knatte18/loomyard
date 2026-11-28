// pushsummary_test.go pins the push summary over a real ledger, an injected clock and a recording notice.

package dobbyengine

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/pushledger"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// summaryHarness drives one daemon's push summary over the ledger of a temp hub.
type summaryHarness struct {
	t       *testing.T
	hub     string
	start   time.Time
	d       *daemon
	notices []string
	postErr error
	waits   []RunView
	refused map[pushledger.Key]string
}

func newSummaryHarness(t *testing.T) *summaryHarness {
	t.Helper()
	h := &summaryHarness{
		t: t, hub: t.TempDir(), start: time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC),
		refused: map[pushledger.Key]string{},
	}
	h.d = &daemon{
		geom: Geometry{WorktreeName: "prime", StatePath: filepath.Join(t.TempDir(), "state.json")},
		deps: Deps{
			Notice: func(_ context.Context, line string) error {
				if h.postErr != nil {
					return h.postErr
				}
				h.notices = append(h.notices, line)
				return nil
			},
			PushSummary: PushSummaryDeps{
				Ledger: func() (map[pushledger.Key]pushledger.Entry, error) { return pushledger.List(h.hub) },
				Refused: func(key pushledger.Key, _ pushledger.Entry) (string, bool, error) {
					wayForward, refused := h.refused[key]
					return wayForward, refused, nil
				},
				Waits:     func() ([]RunView, error) { return h.waits, nil },
				Threshold: func() (time.Duration, error) { return 10 * time.Minute, nil },
			},
		},
	}
	return h
}

// write replaces the ledger entry of key; keep false removes it.
func (h *summaryHarness) write(key pushledger.Key, entry pushledger.Entry, keep bool) {
	h.t.Helper()
	err := pushledger.Update(h.hub, key, h.start, func(pushledger.Entry, bool) (pushledger.Entry, bool, error) {
		return entry, keep, nil
	})
	if err != nil {
		h.t.Fatalf("pushledger.Update(%s) = %v; want nil", key, err)
	}
}

// tickAt runs the summary at minutes after the start and checks how many notices were posted so far.
func (h *summaryHarness) tickAt(minutes, wantNotices int) {
	h.t.Helper()
	h.d.pushSummary(h.t.Context(), h.start.Add(time.Duration(minutes)*time.Minute))
	if len(h.notices) != wantNotices {
		h.t.Fatalf("notices after the tick at %dm = %q; want %d", minutes, h.notices, wantNotices)
	}
}

// wantNotice checks that notice number n holds every one of parts.
func (h *summaryHarness) wantNotice(n int, parts ...string) {
	h.t.Helper()
	for _, part := range parts {
		if !strings.Contains(h.notices[n], part) {
			h.t.Errorf("notice %d = %q; want it to contain %q", n, h.notices[n], part)
		}
	}
}

const allClear = "prime: every branch is pushed to origin and nothing waits on the network"

func TestPushSummary(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("OneSummaryPerOwedPeriodThenOneAllClear", func(t *testing.T) {
		t.Parallel()
		h := newSummaryHarness(t)
		worktree := t.TempDir()
		h.write(pushledger.BoardKey, pushledger.Entry{FirstOwed: h.start}, true)
		h.write("foo-records", pushledger.Entry{Worktree: worktree, FirstOwed: h.start.Add(time.Minute)}, true)
		h.waits = []RunView{{Slug: "bar", WaitProducer: "Land", WaitSince: h.start.Add(2 * time.Minute)}}

		h.tickAt(9, 0)
		h.tickAt(10, 1)
		h.wantNotice(0, "the board, owed 10m", "the branches of "+filepath.Base(worktree)+", owed 9m", "bar at Land for 8m")
		h.tickAt(11, 1)

		h.write(pushledger.BoardKey, pushledger.Entry{}, false)
		h.write("foo-records", pushledger.Entry{}, false)
		h.waits = nil
		h.tickAt(12, 2)
		if h.notices[1] != allClear {
			t.Errorf("second notice = %q; want %q", h.notices[1], allClear)
		}
		h.tickAt(13, 2)
	})

	// The run is one the dobby lists nowhere else, as a beam member's is.
	t.Run("AWaitingRunAloneIsSummarised", func(t *testing.T) {
		t.Parallel()
		h := newSummaryHarness(t)
		h.waits = []RunView{{Slug: "member", WaitProducer: "Finalize", WaitSince: h.start}}

		h.tickAt(0, 0)
		h.tickAt(10, 1)
		h.wantNotice(0, "member at Finalize for 10m")
	})

	t.Run("ACatchUpWaitingOnOriginIsSummarisedPastTheThreshold", func(t *testing.T) {
		t.Parallel()
		h := newSummaryHarness(t)
		h.d.deps.CatchUp = fakeCatchUpDeps(func() error { return ErrOriginUnreachable })
		h.d.oweCatchUp()
		h.d.catchUp(h.t.Context(), h.start)
		h.tickAt(9, 0)

		h.d.catchUpNext = time.Time{}
		h.d.catchUp(h.t.Context(), h.start.Add(10*time.Minute))
		h.tickAt(10, 1)
		h.wantNotice(0, "the dobby's catch-up from origin for 10m")

		h.d.deps.CatchUp = fakeCatchUpDeps(func() error { return nil })
		h.d.catchUpNext = time.Time{}
		h.d.catchUp(h.t.Context(), h.start.Add(11*time.Minute))
		h.tickAt(11, 2)
		if h.notices[1] != allClear {
			t.Errorf("second notice = %q; want %q", h.notices[1], allClear)
		}
	})

	t.Run("ABeamsCatchUpWaitingOnOriginIsSummarised", func(t *testing.T) {
		t.Parallel()
		h := newSummaryHarness(t)
		h.d.deps.PushSummary.BeamCatchUpWaits = func() (map[string]CatchUpWait, error) {
			return map[string]CatchUpWait{
				"beam":    {Reason: CatchUpWaitOrigin, At: h.start.Add(5 * time.Minute), Since: h.start},
				"refused": {Reason: CatchUpWaitRefused, At: h.start, Since: h.start},
			}, nil
		}

		h.tickAt(9, 0)
		h.tickAt(10, 1)
		want := "prime: waiting on the network: beam's catch-up from origin for 10m. lym retries by itself, so only a refused push needs a step"
		if h.notices[0] != want {
			t.Errorf("notice = %q; want %q", h.notices[0], want)
		}
	})

	t.Run("AWaitRecordedWithoutItsStartCountsFromWhenItWasRead", func(t *testing.T) {
		t.Parallel()
		h := newSummaryHarness(t)
		err := UpdateState(h.d.geom.StatePath, func(s *State) error {
			s.CatchUp = &CatchUpWait{Reason: CatchUpWaitOrigin, At: h.start}
			return nil
		})
		if err != nil {
			t.Fatalf("UpdateState() = %v; want nil", err)
		}

		h.tickAt(9, 0)
		h.tickAt(10, 1)
		h.wantNotice(0, "the dobby's catch-up from origin for 10m")
	})

	t.Run("ACatchUpRefusedOrFailedStaysOutOfTheSummary", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name string
			err  error
		}{
			{"Refused", &boardengine.RegistrationError{Reason: "the name is taken", WayForward: "rename this machine"}},
			{"Failed", errors.New("the board diverged")},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				h := newSummaryHarness(t)
				h.d.deps.CatchUp = fakeCatchUpDeps(func() error { return tt.err })
				h.d.oweCatchUp()
				h.d.catchUp(h.t.Context(), h.start)

				// The one notice is the catch-up's own.
				h.tickAt(10, 1)
				h.tickAt(60, 1)
			})
		}
	})

	t.Run("HeldEntriesAreListedAndBlockNoAllClear", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name  string
			entry pushledger.Entry
		}{
			{"HeldRecordsOnly", pushledger.Entry{Worktree: "/hub/kept", HeldRecords: "abc"}},
			{"DroppedOnly", pushledger.Entry{Worktree: "/hub/kept", Dropped: map[string]string{"run": "abc"}}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				h := newSummaryHarness(t)
				h.write("kept-records", tt.entry, true)
				h.tickAt(30, 0)

				h.write(pushledger.BoardKey, pushledger.Entry{FirstOwed: h.start.Add(30 * time.Minute)}, true)
				h.tickAt(40, 1)
				h.wantNotice(0, "held, until its pair goes or its commit leaves the parent: the branches of kept")

				h.write(pushledger.BoardKey, pushledger.Entry{}, false)
				h.tickAt(41, 2)
				if h.notices[1] != allClear {
					t.Errorf("second notice = %q; want %q", h.notices[1], allClear)
				}
			})
		}
	})

	t.Run("ALandingWhoseWorktreeIsGoneIsNamedGone", func(t *testing.T) {
		t.Parallel()
		h := newSummaryHarness(t)
		gone := filepath.Join(t.TempDir(), "beam")
		h.write("beam-records", pushledger.Entry{
			Worktree: gone,
			Landings: []pushledger.Landing{{Run: "member", Tip: "abc", ParentBranch: "beam", At: h.start}},
		}, true)

		h.tickAt(10, 1)
		h.wantNotice(0, "the branches of beam, owed 10m", "its worktree "+gone+" is gone")
	})

	t.Run("ARefusedTargetBlocksTheAllClearUntilItsEntryIsDropped", func(t *testing.T) {
		t.Parallel()
		h := newSummaryHarness(t)
		h.write(pushledger.LandingKey, pushledger.Entry{FirstOwed: h.start}, true)
		h.refused[pushledger.LandingKey] = "run `git push` in /hub/prime"

		h.tickAt(10, 1)
		h.wantNotice(0, "the prime's code branch, owed 10m, refused (way forward: run `git push` in /hub/prime)")
		h.tickAt(60, 1)

		h.write(pushledger.LandingKey, pushledger.Entry{}, false)
		h.tickAt(61, 2)
		if h.notices[1] != allClear {
			t.Errorf("second notice = %q; want %q", h.notices[1], allClear)
		}
	})

	t.Run("AFailedPostIsPostedOnTheNextTickOnce", func(t *testing.T) {
		t.Parallel()
		h := newSummaryHarness(t)
		h.write(pushledger.BoardKey, pushledger.Entry{FirstOwed: h.start}, true)
		h.postErr = errors.New("no orch")

		h.tickAt(10, 0)
		h.postErr = nil
		h.tickAt(11, 1)
		h.tickAt(12, 1)
	})
}
