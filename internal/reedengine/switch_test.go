// switch_test.go pins SwitchClient's session ordering and the exact tmux commands it issues, through TmuxCmd's execHook seam.

package reedengine

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestNextSessionID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		ids     []string
		current string
		next    bool
		want    string
		wantErr bool
	}{
		{name: "Forward", ids: []string{"$0", "$1", "$2"}, current: "$0", next: true, want: "$1"},
		{name: "Backward", ids: []string{"$0", "$1", "$2"}, current: "$2", next: false, want: "$1"},
		{name: "WrapsForward", ids: []string{"$0", "$1", "$2"}, current: "$2", next: true, want: "$0"},
		{name: "WrapsBackward", ids: []string{"$0", "$1", "$2"}, current: "$0", next: false, want: "$2"},
		{name: "NumericOrderAcrossNineAndTen", ids: []string{"$10", "$2", "$9"}, current: "$9", next: true, want: "$10"},
		{name: "NumericOrderBackwardFromTen", ids: []string{"$10", "$2", "$9"}, current: "$10", next: false, want: "$9"},
		{name: "SingleSession", ids: []string{"$4"}, current: "$4", next: true, want: "$4"},
		{name: "CurrentMissing", ids: []string{"$0", "$1"}, current: "$7", next: true, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := nextSessionID(tt.ids, tt.current, tt.next)
			if (err != nil) != tt.wantErr {
				t.Fatalf("nextSessionID() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				if !strings.Contains(err.Error(), tt.current) {
					t.Errorf("error %q does not name the missing session %q", err, tt.current)
				}
				return
			}
			if got != tt.want {
				t.Errorf("nextSessionID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSwitchClientVia_IssuesOnlyTheThreeCommands(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		listing  string
		listErr  error
		current  string
		curErr   error
		next     bool
		wantArgv [][]string
		wantErr  string
	}{
		{
			name:    "NextAcrossSessions",
			listing: "$0 alpha\n$2 beta\n$10 gamma\n",
			current: "$2\n",
			next:    true,
			wantArgv: [][]string{
				{"list-sessions", "-F", "#{session_id} #{session_name}"},
				{"display-message", "-p", "-c", "/dev/pts/3", "#{session_id}"},
				{"switch-client", "-c", "/dev/pts/3", "-t", "$10"},
			},
		},
		{
			name:    "PreviousWraps",
			listing: "$0 alpha\n$2 beta\n",
			current: "$0\n",
			next:    false,
			wantArgv: [][]string{
				{"list-sessions", "-F", "#{session_id} #{session_name}"},
				{"display-message", "-p", "-c", "/dev/pts/3", "#{session_id}"},
				{"switch-client", "-c", "/dev/pts/3", "-t", "$2"},
			},
		},
		{
			name:    "OneSessionIsANoOpWithoutSwitchClient",
			listing: "$0 alpha\n",
			next:    true,
			wantArgv: [][]string{
				{"list-sessions", "-F", "#{session_id} #{session_name}"},
			},
		},
		{
			name:     "UnreachableSocket",
			listErr:  errors.New("no server running"),
			next:     true,
			wantArgv: [][]string{{"list-sessions", "-F", "#{session_id} #{session_name}"}},
			wantErr:  "list sessions",
		},
		{
			name:    "UnknownClient",
			listing: "$0 alpha\n$1 beta\n",
			curErr:  errors.New("can't find client"),
			next:    true,
			wantArgv: [][]string{
				{"list-sessions", "-F", "#{session_id} #{session_name}"},
				{"display-message", "-p", "-c", "/dev/pts/3", "#{session_id}"},
			},
			wantErr: `client "/dev/pts/3"`,
		},
		{
			name:    "CurrentSessionMissingFromListing",
			listing: "$0 alpha\n$1 beta\n",
			current: "$9\n",
			next:    true,
			wantArgv: [][]string{
				{"list-sessions", "-F", "#{session_id} #{session_name}"},
				{"display-message", "-p", "-c", "/dev/pts/3", "#{session_id}"},
			},
			wantErr: "$9",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var argv [][]string
			cmd := newTmuxCmdForSocketPath("tmux", "/tmp/sock")
			cmd.execHook = func(capture bool, args ...string) (string, error) {
				argv = append(argv, slices.Clone(args))
				switch args[0] {
				case "list-sessions":
					return tt.listing, tt.listErr
				case "display-message":
					return tt.current, tt.curErr
				}
				return "", nil
			}

			err := switchClientVia(cmd, "/dev/pts/3", tt.next)

			if tt.wantErr == "" && err != nil {
				t.Fatalf("switchClientVia() error = %v, want nil", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Errorf("switchClientVia() error = %v, want it to contain %q", err, tt.wantErr)
			}
			if !slices.EqualFunc(argv, tt.wantArgv, slices.Equal[[]string]) {
				t.Errorf("tmux argv sequence = %v, want %v", argv, tt.wantArgv)
			}
		})
	}
}
