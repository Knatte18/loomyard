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

func TestEndSessionByName_AbsentSessionTouchesNothing(t *testing.T) {
	fake := endSessionFixture(t, false, "")
	if n := fake.Count("kill-session") + fake.Count("kill-server"); n != 0 {
		t.Errorf("absent session issued %d kill calls: %v", n, fake.Calls())
	}
}

func TestEndSessionByName_SiblingsRemainKeepsServer(t *testing.T) {
	fake := endSessionFixture(t, true, "other\n")
	argv := fake.LastArgv("kill-session")
	if len(argv) != 3 || argv[1] != "-t" || argv[2] != "=pair" {
		t.Errorf("kill-session argv = %v, want exact-match target =pair", argv)
	}
	if fake.Count("kill-server") != 0 {
		t.Errorf("kill-server ran with a sibling session remaining")
	}
}

func TestEndSessionByName_LastSessionKillsServer(t *testing.T) {
	fake := endSessionFixture(t, true, "")
	if fake.Count("kill-session") != 1 || fake.Count("kill-server") != 1 {
		t.Errorf("calls = %v, want one kill-session and one kill-server", fake.Sequence())
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
