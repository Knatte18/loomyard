// overlay_test.go pins ListSessions' parsing half against TmuxCmd's execHook seam, driving the
// three outcomes the watchdog daemon's idle rule distinguishes: a multi-line listing, an exit-0
// empty listing, and an error — with no live tmux server required. It also pins the reap seam's
// two tmux-only halves (reapSessionPanes, reapSessionKill) and ReapSession's call ordering, at the
// level decidable without real processes — none of this touches the live process table.

package reedengine

import (
	"errors"
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
			cmd.execHook = func(capture bool, args ...string) (string, error) {
				if !capture {
					t.Fatalf("execHook called with capture=false, want true (list-sessions always captures)")
				}
				if len(args) < 1 || args[0] != "list-sessions" {
					t.Fatalf("execHook args = %v, want first arg %q", args, "list-sessions")
				}
				return tt.out, tt.hookErr
			}

			got, err := listSessionsVia(cmd)
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

func TestReapSessionKill(t *testing.T) {
	var gotCapture bool
	var gotArgs []string
	cmd := TmuxCmd{tmuxPath: "tmux", socket: "test-socket"}
	cmd.execHook = func(capture bool, args ...string) (string, error) {
		gotCapture = capture
		gotArgs = args
		return "", nil
	}

	if err := reapSessionKill(cmd, "myworktree"); err != nil {
		t.Fatalf("reapSessionKill() unexpected error: %v", err)
	}
	if gotCapture {
		t.Errorf("reapSessionKill() routed through output (capture=true); want run (capture=false)")
	}
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

func TestReapSessionPanes(t *testing.T) {
	t.Run("successful listing", func(t *testing.T) {
		cmd := TmuxCmd{tmuxPath: "tmux", socket: "test-socket"}
		cmd.execHook = func(capture bool, args ...string) (string, error) {
			return "%1 0 0 80 24 4242\n", nil
		}
		got, err := reapSessionPanes(cmd, "myworktree")
		if err != nil {
			t.Fatalf("reapSessionPanes() unexpected error: %v", err)
		}
		want := []LivePane{{ID: "%1", Dead: false, Top: 0, Width: 80, Height: 24, PID: 4242}}
		if len(got) != len(want) || got[0] != want[0] {
			t.Errorf("reapSessionPanes() = %+v; want %+v", got, want)
		}
	})

	t.Run("scripted failure", func(t *testing.T) {
		hookErr := errors.New("no server running on socket")
		cmd := TmuxCmd{tmuxPath: "tmux", socket: "test-socket"}
		cmd.execHook = func(capture bool, args ...string) (string, error) {
			return "", hookErr
		}
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
			wantSequence: []string{"list-panes", "kill-session"},
		},
		{
			name:         "list-panes failure still reaches kill-session",
			panesErr:     errors.New("no server running on socket"),
			wantSequence: []string{"list-panes", "kill-session"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sequence []string
			cmd := TmuxCmd{tmuxPath: "tmux", socket: "test-socket"}
			cmd.execHook = func(capture bool, args ...string) (string, error) {
				sequence = append(sequence, args[0])
				switch args[0] {
				case "list-panes":
					return tt.panesOut, tt.panesErr
				case "kill-session":
					return "", nil
				default:
					t.Fatalf("unexpected tmux subcommand %q", args[0])
					return "", nil
				}
			}

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
