// processscope_state_linux_test.go covers the reading of a scope unit's state from the user manager's D-Bus answers,
// and the wait for a scope that gives up once the state shows the scope ended.

package proc_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestUnitObjectPathLabel(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		unit string
		want string
	}{
		{"lym-pane-42.scope", "lym_2dpane_2d42_2escope"},
		{"7zip.scope", "_37zip_2escope"},
	}
	for _, tt := range tests {
		t.Run(tt.unit, func(t *testing.T) {
			t.Parallel()
			if got := proc.UnitObjectPathLabel(tt.unit); got != tt.want {
				t.Errorf("UnitObjectPathLabel(%q) = %q; want %q", tt.unit, got, tt.want)
			}
		})
	}
}

func TestScopeStateEnded(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name      string
		answer    string
		wantState string
		wantEnded bool
	}{
		{"ActiveIsNotEnded", "s \"active\"\n", "active", false},
		{"ActivatingIsNotEnded", "s \"activating\"\n", "activating", false},
		{"FailedIsEnded", "s \"failed\"\n", "failed", true},
		{"InactiveIsEnded", "s \"inactive\"\n", "inactive", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			state, ended := proc.ScopeStateEnded(tt.answer)
			if state != tt.wantState || ended != tt.wantEnded {
				t.Errorf("ScopeStateEnded(%q) = %q, %v; want %q, %v", tt.answer, state, ended, tt.wantState, tt.wantEnded)
			}
		})
	}
}

func TestScopeEnded_ABusctlThatCannotRunReadsAsNotEnded(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	state, ended := proc.ScopeEnded(context.Background(), "/nonexistent/busctl", "lym-pane-1.scope")
	if state != "" || ended {
		t.Errorf("ScopeEnded() with no busctl = %q, %v; want an empty state and false", state, ended)
	}
}

func TestProcessOOMKills_ReadsTheCgroupOfARunningProcess(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	cgroup, kills, err := proc.ProcessOOMKills(os.Getpid())
	if err != nil {
		t.Skipf("this host lists no memory.events for the test process's cgroup: %v", err)
	}
	if !strings.HasPrefix(cgroup, "/") || kills < 0 {
		t.Errorf("ProcessOOMKills(own pid) = %q, %d; want a cgroup path and a count of 0 or more", cgroup, kills)
	}
}

// The tests below empty PATH with t.Setenv, so they do not run in parallel.

func TestCapProcessMemory_WithoutBusctlOnPathIsACapError(t *testing.T) {
	tiergate.Require(t, 0)
	t.Setenv("PATH", t.TempDir())
	_, err := proc.CapProcessMemory(os.Getpid(), "lym-pane-1.scope", 1<<30)
	if !errors.Is(err, proc.ErrMemoryCap) || !strings.Contains(err.Error(), "is not on PATH") {
		t.Errorf("CapProcessMemory() with no busctl = %v; want an error wrapping ErrMemoryCap that names the missing busctl", err)
	}
}

func TestOOMKillsInCgroup_WithoutJournalctlOnPathIsAnError(t *testing.T) {
	tiergate.Require(t, 0)
	t.Setenv("PATH", t.TempDir())
	if kills, err := proc.OOMKillsInCgroup("/app.slice/lym-pane-1.scope"); err == nil || kills != nil {
		t.Errorf("OOMKillsInCgroup() with no journalctl = %v, %v; want no kills and an error", kills, err)
	}
}

func TestWaitForScope_GivesUpOnAScopeReadEndedTwiceInARow(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	listing, err := os.ReadFile(proc.SelfCgroupFile)
	if err != nil || !strings.Contains("\n"+string(listing), "\n0::") {
		t.Skip("this host lists no cgroup v2 path for a process")
	}

	// The test process never joins the scope, so the wait runs on until a state read ends it or the fake cancels the wait.
	// Once the scripted states run out, the fake cancels the wait and answers active.
	tests := []struct {
		name      string
		states    []string
		wantErr   string
		wantReads int
	}{
		{"TwoInactiveReadsInARowEndTheWait", []string{"inactive", "inactive"}, "is inactive", 2},
		{"OneInactiveReadIsNotEnough", []string{"inactive", "active"}, "was still in", 3},
		{"AnActiveScopeIsWaitedOn", nil, "was still in", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			reads := 0
			readEnded := func() (string, bool) {
				reads++
				if reads > len(tt.states) {
					cancel()
					return "active", false
				}
				return tt.states[reads-1], tt.states[reads-1] == "inactive"
			}
			_, err := proc.WaitForScope(ctx, readEnded, os.Getpid(), "lym-no-such-scope-0.scope")
			if !errors.Is(err, proc.ErrMemoryCap) || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("WaitForScope() = %v; want an error wrapping ErrMemoryCap that holds %q", err, tt.wantErr)
			}
			if reads != tt.wantReads {
				t.Errorf("WaitForScope() read the scope's state %d time(s); want %d", reads, tt.wantReads)
			}
		})
	}
}
