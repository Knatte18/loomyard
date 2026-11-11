//go:build !windows

// verifygate_budget_integration_test.go drives NewVerifyGate with a told time budget over a real scratch repo and shell.
// It pins that the gate feeds its budget clock at each arrival, around each verify command and at each failure.

package loomshed

import (
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestVerifyGate_NoticesTheParentOnceItsBudgetIsSpent runs a failing verify command through three arrivals.
// The first arrival arms the notice timer for the whole budget and cancels it once the command returns.
// The arrival past the limit sends one notice naming the attempt and the failed run, and a later arrival sends none.
func TestVerifyGate_NoticesTheParentOnceItsBudgetIsSpent(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	worktree, verifyDir := t.TempDir(), t.TempDir()
	gitkit.Git(t, worktree, "init", "-b", "main")
	gitkit.Git(t, worktree, "config", "user.email", "test@test.com")
	gitkit.Git(t, worktree, "config", "user.name", "Test")
	gitkit.CommitFile(t, worktree, "a.txt", "a\n", "init")

	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	var notices []string
	var delays []time.Duration
	cancelled := 0
	budget := VerifyBudget{
		Limit:  30 * time.Minute,
		Notify: func(line string) error { notices = append(notices, line); return nil },
		now:    func() time.Time { return now },
		after: func(d time.Duration, _ func()) func() {
			delays = append(delays, d)
			return func() { cancelled++ }
		},
	}
	gate := NewVerifyGate(worktree, verifyDir, "Darn-Write gate", func() (string, error) { return "exit 1", nil }, nil, nil, nil, budget)
	requireFailure := func(t *testing.T) {
		t.Helper()
		if got, err := gate(); err != nil || got.Passed {
			t.Fatalf("gate() = %+v, %v; want a failure", got, err)
		}
	}

	requireFailure(t)
	if len(delays) != 1 || delays[0] != 30*time.Minute || cancelled != 1 || len(notices) != 0 {
		t.Fatalf("after the first arrival: timers armed for %v, %d cancelled, notices %q; want one timer for 30m0s, cancelled, no notice", delays, cancelled, notices)
	}

	now = now.Add(31 * time.Minute)
	requireFailure(t)
	if len(notices) != 1 {
		t.Fatalf("after the arrival past the limit: notices = %q; want exactly one", notices)
	}
	for _, want := range []string{"Darn-Write gate:", "30m0s budget", "attempt 2;", "attempt 1, exit code 1"} {
		if !strings.Contains(notices[0], want) {
			t.Errorf("notice = %q; want it to hold %q", notices[0], want)
		}
	}

	requireFailure(t)
	if len(notices) != 1 {
		t.Errorf("after a later arrival: notices = %q; want still the one", notices)
	}
}
