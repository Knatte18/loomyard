// removename_test.go pins `lyx reed remove --name`'s Tier 1 surface: the argument rules, the
// parent-exit wait over a fake liveness check, and the help text of the three new flags. It spawns
// nothing and drives no tmux; the live self-removal is smoke_removebyname_test.go.

package reedcli

import (
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/reedengine"
)

func TestValidateRemoveArgs(t *testing.T) {
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

func TestWaitParentExit_ZeroPIDIsNoWait(t *testing.T) {
	old := pidAlive
	t.Cleanup(func() { pidAlive = old })
	pidAlive = func(int) bool { t.Fatal("liveness polled for a zero pid"); return true }
	if err := waitParentExit(0); err != nil {
		t.Fatalf("waitParentExit(0) = %v; want nil", err)
	}
}

func TestWaitParentExit_ReturnsOnceGone(t *testing.T) {
	old := pidAlive
	t.Cleanup(func() { pidAlive = old })
	polls := 0
	pidAlive = func(int) bool { polls++; return polls < 3 }
	// The production interval is 100ms; two waits stay well under any Tier 1 budget.
	if err := waitParentExit(4242); err != nil {
		t.Fatalf("waitParentExit = %v; want nil", err)
	}
	if polls != 3 {
		t.Errorf("polls = %d; want 3", polls)
	}
}

func TestWaitPIDGone_GivesUpAtCap(t *testing.T) {
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
