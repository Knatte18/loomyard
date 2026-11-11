// verifybudget_test.go covers when a verify gate's spent time budget notices the run's parent.

package loomshed

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// budgetRig is a budget clock over a told clock, a recording Notify and a timer the test fires by hand.
type budgetRig struct {
	now       time.Time
	notices   []string
	delay     time.Duration
	fire      func()
	cancelled bool
	clock     *budgetClock
}

func newBudgetRig(limit time.Duration) *budgetRig {
	r := &budgetRig{now: time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)}
	r.clock = &budgetClock{site: "Darn-Write gate", budget: VerifyBudget{
		Limit:  limit,
		Notify: func(line string) error { r.notices = append(r.notices, line); return nil },
		now:    func() time.Time { return r.now },
		after: func(d time.Duration, f func()) func() {
			r.delay, r.fire = d, f
			return func() { r.cancelled = true }
		},
	}}
	return r
}

// TestVerifyBudget pins when a spent budget notices the run's parent.
// The notice goes once, at an arrival past the limit or when a run's armed timer fires.
// It names the attempt and every failed run so far, the latest first, and is cut to its byte cap on a rune boundary.
// A cancelled timer and a budget that is off send none.
func TestVerifyBudget(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("an arrival past the limit sends one notice naming the attempt and every failed run, the latest first", func(t *testing.T) {
		t.Parallel()
		r := newBudgetRig(30 * time.Minute)
		dir := t.TempDir()
		logPath := filepath.Join(dir, "verify-1.log")
		if err := os.WriteFile(logPath, []byte("FAIL\n"+gateslot.FailedRunsPrefix+"tier0 (exit 1); the exit code is tier0's\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		absentLog := filepath.Join(dir, "verify-2.log")

		r.clock.arrive(1)
		r.clock.failed(1, verifytree.Result{Status: verifytree.StatusFailed, ExitCode: 1, Log: logPath})
		r.clock.arrive(2)
		r.clock.failed(2, verifytree.Result{Status: verifytree.StatusFailed, ExitCode: 3, Log: absentLog})
		r.now = r.now.Add(31 * time.Minute)
		r.clock.arrive(3)
		r.clock.arrive(4)

		if len(r.notices) != 1 {
			t.Fatalf("notices = %q; want exactly one", r.notices)
		}
		notice := r.notices[0]
		// A log that names no failed runs falls back to the exit code.
		for _, want := range []string{"Darn-Write gate:", "30m0s budget", "attempt 3;", "attempt 1, tier0 (exit 1)", logPath, "attempt 2, exit code 3", absentLog} {
			if !strings.Contains(notice, want) {
				t.Errorf("notice = %q; want it to hold %q", notice, want)
			}
		}
		if second, first := strings.Index(notice, "attempt 2, exit code 3"), strings.Index(notice, "attempt 1, tier0 (exit 1)"); second > first {
			t.Errorf("notice = %q; want the latest failure, attempt 2, before attempt 1", notice)
		}
	})

	t.Run("a notice past the byte cap is cut on a rune boundary", func(t *testing.T) {
		t.Parallel()
		// The two pads shift the multibyte log path by a byte, so one of them puts the cut inside a rune.
		for _, pad := range []string{"", "x"} {
			r := newBudgetRig(time.Minute)
			r.clock.arrive(1)
			r.clock.failed(1, verifytree.Result{Status: verifytree.StatusFailed, ExitCode: 2, Log: pad + strings.Repeat("é", verifyNoticeBytes)})
			r.now = r.now.Add(2 * time.Minute)
			r.clock.arrive(2)

			if len(r.notices) != 1 {
				t.Fatalf("pad %q: notices = %q; want exactly one", pad, r.notices)
			}
			notice := r.notices[0]
			if !utf8.ValidString(notice) || !strings.HasSuffix(notice, "...") || len(notice) > verifyNoticeBytes+len("...") {
				t.Errorf("pad %q: notice of %d bytes = %q; want valid UTF-8 cut to %d bytes and ending in an ellipsis", pad, len(notice), notice, verifyNoticeBytes)
			}
			if !strings.Contains(notice, "attempt 1, exit code 2") {
				t.Errorf("pad %q: notice = %q; want the latest failure kept ahead of the cut", pad, notice)
			}
		}
	})

	t.Run("a run armed before the limit sends the notice when its timer fires", func(t *testing.T) {
		t.Parallel()
		r := newBudgetRig(30 * time.Minute)

		r.clock.arrive(1)
		r.now = r.now.Add(10 * time.Minute)
		disarm := r.clock.arm()
		if r.delay != 20*time.Minute || r.fire == nil {
			t.Fatalf("armed delay = %v; want the 20m0s the budget has left", r.delay)
		}
		r.now = r.now.Add(20 * time.Minute)
		r.fire()
		disarm()

		if len(r.notices) != 1 || !strings.Contains(r.notices[0], "no verify run has failed yet") {
			t.Errorf("notices = %q; want one saying no verify run has failed yet", r.notices)
		}
		if !r.cancelled {
			t.Error("the timer was not cancelled once the run returned")
		}
	})

	t.Run("arrivals within the limit and a cancelled timer send nothing", func(t *testing.T) {
		t.Parallel()
		r := newBudgetRig(30 * time.Minute)

		r.clock.arrive(1)
		r.now = r.now.Add(29 * time.Minute)
		r.clock.arrive(2)
		r.clock.arm()()

		if len(r.notices) != 0 || !r.cancelled {
			t.Errorf("notices = %q, cancelled = %v; want none and the timer cancelled", r.notices, r.cancelled)
		}
	})

	t.Run("a zero limit or a nil Notify turns the budget off", func(t *testing.T) {
		t.Parallel()
		for _, r := range []*budgetRig{newBudgetRig(0), newBudgetRig(time.Minute)} {
			if r.clock.budget.Limit != 0 {
				r.clock.budget.Notify = nil
			}
			r.clock.arrive(1)
			r.now = r.now.Add(2 * time.Hour)
			r.clock.arrive(2)
			r.clock.arm()()
			if r.fire != nil || len(r.notices) != 0 {
				t.Errorf("limit %v: notices = %q, timer armed = %v; want neither", r.clock.budget.Limit, r.notices, r.fire != nil)
			}
		}
	})
}
