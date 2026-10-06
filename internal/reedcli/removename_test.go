// removename_test.go pins `lyx reed remove --name`'s Tier 1 surface: the argument rules, the
// parent-exit wait over a fake liveness check, and the help text of the three new flags. It spawns
// nothing and drives no tmux; the live self-removal is smoke_removebyname_test.go.
// TestWaitParentExit swaps the package-level pidAlive, so it does not call t.Parallel.

package reedcli

import (
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/reedengine"
)

func TestValidateRemoveArgs(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		args    []string
		strand  string
		detach  bool
		wantErr string
	}{
		{"neither", nil, "", false, "a strand is required"},
		{"both", []string{"abc"}, "driver", false, "not both"},
		{"detach without name", []string{"abc"}, "", true, "--detach requires --name"},
		{"detach alone", nil, "", true, "a strand is required"},
		{"guid only", []string{"abc"}, "", false, ""},
		{"name only", nil, "driver", false, ""},
		{"name and detach", nil, "driver", true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateRemoveArgs(tc.args, tc.strand, tc.detach)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v; want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

//testtiming:keep pins that a zero pid is never polled and that the wait returns after the third poll finds the pid gone, counts its covering tests do not assert
func TestWaitParentExit(t *testing.T) {
	tests := []struct {
		name      string
		pid       int
		aliveFor  int
		wantPolls int
	}{
		{name: "ZeroPIDIsNoWait", pid: 0, aliveFor: 0, wantPolls: 0},
		// The production interval is 100ms; two waits stay well under any Tier 1 budget.
		{name: "ReturnsOnceGone", pid: 4242, aliveFor: 2, wantPolls: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			old := pidAlive
			t.Cleanup(func() { pidAlive = old })
			polls := 0
			pidAlive = func(int) bool { polls++; return polls <= tt.aliveFor }
			if err := waitParentExit(tt.pid); err != nil {
				t.Fatalf("waitParentExit(%d) = %v; want nil", tt.pid, err)
			}
			if polls != tt.wantPolls {
				t.Errorf("polls = %d; want %d", polls, tt.wantPolls)
			}
		})
	}
}

func TestWaitPIDGone_GivesUpAtCap(t *testing.T) {
	t.Parallel()
	polls := 0
	alive := func(int) bool { polls++; return true }
	if reedengine.WaitPIDGone(1, alive, 5, time.Millisecond) {
		t.Fatal("WaitPIDGone = true for an always-alive pid; want false")
	}
	if polls < 5 || polls > 6 {
		t.Errorf("polls = %d; want the cap (5) plus at most one final check", polls)
	}
}

func TestRemoveCmd_FlagsDescribedAndPIDHidden(t *testing.T) {
	t.Parallel()
	cmd := (&reedCLI{}).removeCmd()
	for _, n := range []string{"name", "detach", "wait-pid", "recursive"} {
		f := cmd.Flags().Lookup(n)
		if f == nil {
			t.Fatalf("flag --%s missing", n)
		}
		if strings.TrimSpace(f.Usage) == "" {
			t.Errorf("flag --%s has no description", n)
		}
	}
	if !cmd.Flags().Lookup("wait-pid").Hidden {
		t.Error("--wait-pid must be hidden")
	}
}
