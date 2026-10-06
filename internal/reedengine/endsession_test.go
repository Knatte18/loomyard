// endsession_test.go drives endSessionByNameVia through the fake tmux seam.

package reedengine

import (
	"errors"
	"testing"
)

// exitCodeErr stands in for tmux's exit-1 "no such session" answer without spawning a process.
type exitCodeErr struct{ code int }

func (e exitCodeErr) Error() string { return "exit status" }
func (e exitCodeErr) ExitCode() int { return e.code }

func endSessionFixture(t *testing.T, present bool, listed string) *fakeTmux {
	t.Helper()
	var cmd TmuxCmd
	fake := installFakeTmuxOn(t, &cmd)
	if !present {
		fake.answer("has-session", "", exitCodeErr{code: 1})
	}
	fake.answer("list-sessions", listed, nil)
	got, err := endSessionByNameVia(cmd, "", "sock", "pair")
	if err != nil {
		t.Fatalf("endSessionByNameVia() error: %v", err)
	}
	if got != present {
		t.Fatalf("endSessionByNameVia() = %v, want %v", got, present)
	}
	return fake
}

// TestEndSessionByName pins which kills endSessionByNameVia issues: none for an absent session,
// an exact-match kill-session alone while sibling sessions remain, and kill-server after the last session.
func TestEndSessionByName(t *testing.T) {
	tests := []struct {
		name            string
		present         bool
		listed          string
		wantKillSession int
		wantKillServer  int
	}{
		{"AbsentSessionTouchesNothing", false, "", 0, 0},
		{"SiblingsRemainKeepsServer", true, "other\n", 1, 0},
		{"LastSessionKillsServer", true, "", 1, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := endSessionFixture(t, tt.present, tt.listed)
			if got := fake.Count("kill-session"); got != tt.wantKillSession {
				t.Errorf("kill-session calls = %d, want %d: %v", got, tt.wantKillSession, fake.Sequence())
			}
			if got := fake.Count("kill-server"); got != tt.wantKillServer {
				t.Errorf("kill-server calls = %d, want %d: %v", got, tt.wantKillServer, fake.Sequence())
			}
			if tt.wantKillSession > 0 {
				argv := fake.LastArgv("kill-session")
				if len(argv) != 3 || argv[1] != "-t" || argv[2] != "=pair" {
					t.Errorf("kill-session argv = %v, want exact-match target =pair", argv)
				}
			}
		})
	}
}

func TestEndSessionByName_HasSessionFailureIsAnError(t *testing.T) {
	var cmd TmuxCmd
	fake := installFakeTmuxOn(t, &cmd)
	fake.answer("has-session", "", errors.New("tmux unreachable"))
	fake.mustNotCall("kill-session", "kill-server")
	if _, err := endSessionByNameVia(cmd, "", "sock", "pair"); err == nil {
		t.Fatal("want error when has-session fails other than exit 1")
	}
}
