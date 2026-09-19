// overlay_test.go pins ListSessions' parsing half against TmuxCmd's execHook seam, driving the
// three outcomes the watchdog daemon's idle rule distinguishes: a multi-line listing, an exit-0
// empty listing, and an error — with no live tmux server required.

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
