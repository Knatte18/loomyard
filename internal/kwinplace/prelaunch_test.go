// prelaunch_test.go pins which sessions place a window.

package kwinplace

import (
	"errors"
	"maps"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestPreLaunch(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	passing := map[string]string{"KDE_SESSION_VERSION": "6", "XDG_CURRENT_DESKTOP": "KDE", "XDG_SESSION_TYPE": "wayland"}
	tests := []struct {
		name    string
		env     map[string]string
		missing string
		konsole bool
		// want is what the error names, "" for a session that passes.
		want string
	}{
		{name: "Passes", konsole: true},
		{name: "NoKDEVersion", env: map[string]string{"KDE_SESSION_VERSION": ""}, konsole: true, want: "KDE_SESSION_VERSION"},
		{name: "Plasma5", env: map[string]string{"KDE_SESSION_VERSION": "5"}, konsole: true, want: "KDE_SESSION_VERSION"},
		{name: "Gnome", env: map[string]string{"XDG_CURRENT_DESKTOP": "GNOME"}, konsole: true, want: "XDG_CURRENT_DESKTOP"},
		{name: "KDEInAList", env: map[string]string{"XDG_CURRENT_DESKTOP": "KDE:custom"}, konsole: true},
		{name: "X11", env: map[string]string{"XDG_SESSION_TYPE": "x11"}, konsole: true, want: "XDG_SESSION_TYPE"},
		{name: "NotKonsole", want: "Konsole"},
		{name: "NoQdbus", missing: "qdbus6", konsole: true, want: "qdbus6"},
		{name: "NoJournalctl", missing: "journalctl", konsole: true, want: "journalctl"},
		{name: "NoKscreenDoctor", missing: "kscreen-doctor", konsole: true, want: "kscreen-doctor"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			env := maps.Clone(passing)
			maps.Copy(env, tt.env)
			err := preLaunch(sessionProbe{
				getenv: func(key string) string { return env[key] },
				lookPath: func(program string) (string, error) {
					if program == tt.missing {
						return "", errors.New("not found")
					}
					return "/bin/" + program, nil
				},
			}, tt.konsole)

			if tt.want == "" {
				if err != nil {
					t.Errorf("preLaunch() = %v; want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("preLaunch() = %v; want an error naming %s", err, tt.want)
			}
		})
	}
}
