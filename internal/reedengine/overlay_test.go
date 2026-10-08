// overlay_test.go pins ListSessions' parsing half against TmuxCmd's execHook seam, driving the
// three outcomes the watchdog daemon's idle rule distinguishes: a multi-line listing, an exit-0
// empty listing, and an error — with no live tmux server required. It also pins the reap seam's
// two tmux-only halves (reapSessionPanes, reapSessionKill) and ReapSession's call ordering, at the
// level decidable without real processes — none of this touches the live process table.

package reedengine

import (
	"errors"
	"slices"
	"testing"
)

func TestListSessions(t *testing.T) {
	tests := []struct {
		name    string
		out     string
		hookErr error
		want    []string
		wantErr bool
	}{
		{
			name: "multi-line listing",
			out:  "alpha\nbeta\ngamma\n",
			want: []string{"alpha", "beta", "gamma"},
		},
		{
			name: "exit-0 empty listing",
			out:  "",
			want: nil,
		},
		{
			name:    "error",
			hookErr: errors.New("no server running on socket"),
			wantErr: true,
		},
		{
			name: "trailing whitespace and trailing newline",
			out:  "alpha  \n  beta\n\n",
			want: []string{"alpha", "beta"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// listSessionsVia is ListSessions' parsing half, factored out so a test can drive it
			// through TmuxCmd's execHook seam directly rather than a real server.
			cmd := TmuxCmd{tmuxPath: "tmux", socket: "test-socket"}
			fake := installFakeTmuxOn(t, &cmd)
			fake.answer("list-sessions", tt.out, tt.hookErr)

			got, err := listSessionsVia(cmd)
			if seq := fake.Sequence(); len(seq) != 1 || seq[0] != "list-sessions" {
				t.Fatalf("tmux calls = %v, want exactly one list-sessions", seq)
			}
			if captured := fake.CapturedFor("list-sessions"); !captured[0] {
				t.Fatalf("list-sessions called with capture=false, want true (list-sessions always captures)")
			}
			if tt.wantErr {
				if err == nil {
					t.Fatalf("listSessionsVia() error = nil, want non-nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("listSessionsVia() unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("listSessionsVia() = %v; want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("listSessionsVia()[%d] = %q; want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// unreachablePaneLine is a list-panes response line whose pid (999999) descendantClosurePIDs'
// real /proc walk finds absent, so its closure collapses to just itself — keeping these untagged
// reap-seam tests fast and free of real-process dependence, per the batch's Requirements.
const unreachablePaneLine = "%1 0 0 80 24 999999\n"

//testtiming:keep pins the exact kill-session argv with the exact-match target and that it runs uncaptured; its covering tests run this code without asserting it
func TestReapSessionKill(t *testing.T) {
	cmd := TmuxCmd{tmuxPath: "tmux", socket: "test-socket"}
	fake := installFakeTmuxOn(t, &cmd)

	if err := reapSessionKill(cmd, "myworktree"); err != nil {
		t.Fatalf("reapSessionKill() unexpected error: %v", err)
	}
	calls := fake.Calls()
	if len(calls) != 1 {
		t.Fatalf("reapSessionKill() made %d tmux calls; want 1: %v", len(calls), calls)
	}
	if fake.CapturedFor("kill-session")[0] {
		t.Errorf("reapSessionKill() routed through output (capture=true); want run (capture=false)")
	}
	gotArgs := calls[0]
	want := []string{"kill-session", "-t", "=myworktree"}
	if len(gotArgs) != len(want) {
		t.Fatalf("reapSessionKill() args = %v; want %v", gotArgs, want)
	}
	for i := range want {
		if gotArgs[i] != want[i] {
			t.Errorf("reapSessionKill() args[%d] = %q; want %q", i, gotArgs[i], want[i])
		}
	}
}

//testtiming:keep pins list-panes output parsed into LivePane rows and a listing failure surfacing as an error; its covering tests run this code without asserting it
func TestReapSessionPanes(t *testing.T) {
	t.Run("successful listing", func(t *testing.T) {
		cmd := TmuxCmd{tmuxPath: "tmux", socket: "test-socket"}
		fake := installFakeTmuxOn(t, &cmd)
		fake.answer("list-panes", "%1 0 0 80 24 4242\n", nil)
		got, err := reapSessionPanes(cmd, "myworktree")
		if err != nil {
			t.Fatalf("reapSessionPanes() unexpected error: %v", err)
		}
		// Session-wide, so a kill-session's reap covers every window of the session.
		if argv := fake.ArgvFor(sessionListVerb); len(argv) != 1 || !slices.Equal(argv[0][:4], []string{"list-panes", "-s", "-t", "=myworktree"}) {
			t.Errorf("reapSessionPanes() listed with %v; want one session-wide list-panes -s -t =myworktree", argv)
		}
		want := []LivePane{{ID: "%1", Dead: false, Top: 0, Width: 80, Height: 24, PID: 4242}}
		if len(got) != len(want) || got[0] != want[0] {
			t.Errorf("reapSessionPanes() = %+v; want %+v", got, want)
		}
	})

	t.Run("scripted failure", func(t *testing.T) {
		hookErr := errors.New("no server running on socket")
		cmd := TmuxCmd{tmuxPath: "tmux", socket: "test-socket"}
		installFakeTmuxOn(t, &cmd).answer("list-panes", "", hookErr)
		_, err := reapSessionPanes(cmd, "myworktree")
		if err == nil {
			t.Fatalf("reapSessionPanes() error = nil, want non-nil")
		}
	})
}

// TestReapSession_CallOrdering pins ReapSession's call ordering at the level decidable without
// real processes: list-panes must run before kill-session, a list-panes failure must not abort
// the reap, and the pane list reapSessionPanes returns is exactly what feeds sessionReapRoots —
// mirroring ReapSession's own steps 1-3 against one shared TmuxCmd carrying the execHook, since
// ReapSession itself builds a fresh, unhookable TmuxCmd internally via NewTmuxCmd. This does not
// re-test sessionReapRoots' own safeReapRoot filter, which strand.go's existing coverage already
// owns.
//
//testtiming:keep pins list-panes running before kill-session and a list-panes failure still reaching kill-session, with the pane list feeding the reap roots; its covering tests run this code without asserting it
func TestReapSession_CallOrdering(t *testing.T) {
	tests := []struct {
		name         string
		panesOut     string
		panesErr     error
		wantSequence []string
	}{
		{
			name:         "list-panes then kill-session",
			panesOut:     unreachablePaneLine,
			wantSequence: []string{sessionListVerb, "kill-session"},
		},
		{
			name:         "list-panes failure still reaches kill-session",
			panesErr:     errors.New("no server running on socket"),
			wantSequence: []string{sessionListVerb, "kill-session"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := TmuxCmd{tmuxPath: "tmux", socket: "test-socket"}
			fake := installFakeTmuxOn(t, &cmd)
			fake.answer("list-panes", tt.panesOut, tt.panesErr)

			// Step 1: reapSessionPanes, exactly as ReapSession calls it. A scripted listing
			// failure is recorded but does not stop the reap — ReapSession logs and continues
			// with a nil live.
			live, panesErr := reapSessionPanes(cmd, "myworktree")
			if tt.panesErr != nil {
				if panesErr == nil {
					t.Fatalf("reapSessionPanes() error = nil, want non-nil")
				}
				live = nil
			} else if panesErr != nil {
				t.Fatalf("reapSessionPanes() unexpected error: %v", panesErr)
			}

			// Step 2 (the part decidable without real processes): ReapSession feeds exactly
			// this pane list to sessionReapRoots.
			roots := sessionReapRoots(live)
			if tt.panesErr == nil && len(roots) != 1 {
				t.Fatalf("sessionReapRoots(live) = %v; want one root from the scripted pane", roots)
			}
			if tt.panesErr != nil && len(roots) != 0 {
				t.Fatalf("sessionReapRoots(live) = %v; want none, panes list was nil", roots)
			}

			// Step 3: reapSessionKill, exactly as ReapSession calls it.
			if err := reapSessionKill(cmd, "myworktree"); err != nil {
				t.Fatalf("reapSessionKill() unexpected error: %v", err)
			}

			sequence := fake.Sequence()
			if len(sequence) != len(tt.wantSequence) {
				t.Fatalf("call sequence = %v; want %v", sequence, tt.wantSequence)
			}
			for i := range tt.wantSequence {
				if sequence[i] != tt.wantSequence[i] {
					t.Errorf("call sequence[%d] = %q; want %q", i, sequence[i], tt.wantSequence[i])
				}
			}
		})
	}
}
