// follow_test.go pins the told-geometry follow jump: the window it selects from tmux facts and the exact commands it issues, through TmuxCmd's execHook seam.

package reedengine

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

// TestFollowSessionVia pins the commands followSessionVia issues for each session shape, and the error each failed command returns.
// The listing lines are followTargetFormat lines: window id, window index, own flag, overview flag, pane dead flag.
func TestFollowSessionVia(t *testing.T) {
	t.Parallel()
	const session = "tc-session"
	list := func(session string) []string {
		return []string{"list-panes", "-s", "-t", "=" + session, "-F", followTargetFormat}
	}
	setOn := func(session string) []string {
		return []string{"set-option", "-t", "=" + session, "@lyx_follow", "on"}
	}
	selectWindow := func(window string) []string { return []string{"select-window", "-t", window} }
	tests := []struct {
		name      string
		session   string
		listing   string
		listErr   error
		setErr    error
		selectErr error
		wantCalls func(session string) [][]string
		wantErr   []string
	}{
		{
			name:    "NewestLiveAgentWinsOverOlderAgentDeadNewerAgentOverviewAndNonOwnWindow",
			session: session,
			listing: "@0 5 0 0 0\n@1 1 1 0 0\n@2 2 1 0 0\n@3 0 1 1 0\n@4 4 1 0 1\n",
			wantCalls: func(s string) [][]string {
				return [][]string{list(s), setOn(s), selectWindow("@2")}
			},
		},
		{
			name:    "NoLiveAgentSelectsTheOverview",
			session: session,
			listing: "@0 5 0 0 0\n@3 0 1 1 0\n@4 4 1 0 1\n",
			wantCalls: func(s string) [][]string {
				return [][]string{list(s), setOn(s), selectWindow("@3")}
			},
		},
		{
			name:    "NeitherAgentNorOverviewSetsTheOptionAndSelectsNothing",
			session: session,
			listing: "@0 5 0 0 0\n",
			wantCalls: func(s string) [][]string {
				return [][]string{list(s), setOn(s)}
			},
		},
		{
			name:    "UnmarkedOwnWindowWithALivePaneIsSelectedWhenNoOtherAgentIsLive",
			session: session,
			listing: "@0 5 0 0 0\n@3 0 1 0 0\n",
			wantCalls: func(s string) [][]string {
				return [][]string{list(s), setOn(s), selectWindow("@3")}
			},
		},
		{
			name:    "UnparsableLinesAreSkippedAndTheRestStillDecides",
			session: session,
			listing: "garbage\n@x notanumber 1 0 0\n@1 1 1 0 0\n",
			wantCalls: func(s string) [][]string {
				return [][]string{list(s), setOn(s), selectWindow("@1")}
			},
		},
		{
			name:    "FailingListingNamesTheSessionAndWritesNothing",
			session: session,
			listErr: errors.New("can't find session"),
			wantCalls: func(s string) [][]string {
				return [][]string{list(s)}
			},
			wantErr: []string{`session "tc-session"`, "can't find session"},
		},
		{
			name:    "FailingOptionWriteSelectsNothing",
			session: session,
			listing: "@1 1 1 0 0\n",
			setErr:  errors.New("option refused"),
			wantCalls: func(s string) [][]string {
				return [][]string{list(s), setOn(s)}
			},
			wantErr: []string{`session "tc-session"`, "option refused"},
		},
		{
			name:      "FailingSelectNamesTheWindowAfterTheOptionWrite",
			session:   session,
			listing:   "@1 1 1 0 0\n",
			selectErr: errors.New("select refused"),
			wantCalls: func(s string) [][]string {
				return [][]string{list(s), setOn(s), selectWindow("@1")}
			},
			wantErr: []string{"@1", "select refused"},
		},
		{
			name:    "SessionNameWithASpaceAndAHashReachesBothTargetsUnchanged",
			session: "my sess #1",
			listing: "@1 1 1 0 0\n",
			wantCalls: func(s string) [][]string {
				return [][]string{list(s), setOn(s), selectWindow("@1")}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cmd := newTmuxCmdForSocketPath("tmux", "/tmp/sock")
			fake := installFakeTmuxOn(t, &cmd)
			fake.answer(sessionListVerb, tt.listing, tt.listErr)
			fake.answer("set-option", "", tt.setErr)
			fake.answer("select-window", "", tt.selectErr)

			err := followSessionVia(cmd, tt.session)

			if len(tt.wantErr) == 0 && err != nil {
				t.Fatalf("followSessionVia() error = %v, want nil", err)
			}
			if len(tt.wantErr) > 0 && err == nil {
				t.Fatalf("followSessionVia() = nil, want an error containing %q", tt.wantErr)
			}
			for _, want := range tt.wantErr {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("followSessionVia() error = %q, want it to contain %q", err, want)
				}
			}
			if want := tt.wantCalls(tt.session); !slices.EqualFunc(fake.Calls(), want, slices.Equal[[]string]) {
				t.Errorf("tmux calls = %v, want %v", fake.Calls(), want)
			}
		})
	}
}
