package reedengine

import (
	"reflect"
	"testing"
)

func TestOwnsTmuxEnv(t *testing.T) {
	e := newTestEngine(t)
	sock := e.Socket()

	cases := []struct {
		name string
		env  string
		want bool
	}{
		{"matching socket path", "/tmp/tmux-1000/" + sock + ",1234,0", true},
		{"another socket name", "/tmp/tmux-1000/other,1234,0", false},
		{"windows-style path", `C:\Users\x\AppData\` + sock + ",1234,0", true},
		{"windows-style other", `C:\Users\x\AppData\other,1234,0`, false},
		{"bare socket name", sock + ",1234,0", true},
		{"empty", "", false},
		{"no commas", "/tmp/tmux-1000/" + sock, false},
		{"too few fields", "/tmp/tmux-1000/" + sock + ",1234", false},
		{"empty path", ",1234,0", false},
		{"trailing separator", "/tmp/tmux-1000/,1234,0", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := e.OwnsTmuxEnv(tc.env); got != tc.want {
				t.Errorf("OwnsTmuxEnv(%q) = %v, want %v", tc.env, got, tc.want)
			}
		})
	}
}

func TestSwitchClientArgv(t *testing.T) {
	e := newTestEngine(t)
	want := []string{"-L", e.Socket(), "switch-client", "-t", "=" + e.SessionName()}
	if got := e.SwitchClientArgv(); !reflect.DeepEqual(got, want) {
		t.Errorf("SwitchClientArgv() = %v, want %v", got, want)
	}
}
