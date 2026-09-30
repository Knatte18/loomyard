// handover_test.go covers each decideHandover row.

package orchcli

import "testing"

func TestDecideHandover(t *testing.T) {
	cases := []struct {
		name     string
		noAttach bool
		tmuxEnv  string
		reedOwns bool
		current  string
		target   string
		want     handover
	}{
		{"no-attach wins", true, "", false, "", "s", handoverEnvelope},
		{"no-attach inside tmux", true, "/tmp/tmux", true, "other", "s", handoverEnvelope},
		{"outside tmux attaches", false, "", false, "", "s", handoverAttach},
		{"foreign tmux server hints", false, "/tmp/foreign", false, "", "s", handoverHint},
		{"reed server, session unreadable hints", false, "/tmp/reed", true, "", "s", handoverHint},
		{"reed server, already in session", false, "/tmp/reed", true, "s", "s", handoverEnvelope},
		{"reed server, other session switches", false, "/tmp/reed", true, "other", "s", handoverSwitch},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := decideHandover(c.noAttach, c.tmuxEnv, c.reedOwns, c.current, c.target); got != c.want {
				t.Errorf("decideHandover = %v; want %v", got, c.want)
			}
		})
	}
}
