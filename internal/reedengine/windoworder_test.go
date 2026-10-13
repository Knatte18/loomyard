// windoworder_test.go pins the window order's fallback placements and the shell window check, driven through OpenWindow and AttachArgv against the scripted multiplexer fake.
// The placements a real tmux decides are pinned by windoworder_integration_test.go.

package reedengine

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

// TestWindowOrder pins the fallback placements of an own window, over listings that parseSessionWindows reads.
// The relaunch and shell placements are pinned by TestWindowOrder_RealTmux alone.
func TestWindowOrder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		listing  string
		shell    string
		previous string
		// overview selects overviewWindowInsertArgs, which reads the listing alone.
		overview bool
		want     []string
	}{
		{
			name:     "the overview goes before the lowest window, hidden ones included",
			listing:  "@4 3 0 0 explore\n@7 1 1 0 batten:one\n@2 5 0 0 shell\n",
			overview: true,
			want:     []string{"-b", "-t", "@7"},
		},
		{
			name:     "the overview with no window gives the plain form",
			overview: true,
		},
		{
			name:    "no shell window goes after the highest non-hidden window",
			listing: "@0 0 0 0 explore\n@3 2 0 0 review\n@5 4 1 0 batten:one\n@1 1 0 0 plan\n",
			want:    []string{"-a", "-t", "@3"},
		},
		{
			name:    "a line it cannot parse is skipped",
			listing: "garbage\n@0 x 0 0 explore\n@2 1 0 0 plan\n",
			want:    []string{"-a", "-t", "@2"},
		},
		{
			name:    "no window gives the plain form",
			listing: "",
			shell:   "@0",
		},
		{
			name:    "only hidden windows give the plain form",
			listing: "@4 0 1 0 batten:one\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			windows := parseSessionWindows(tt.listing)
			if tt.overview {
				if got := overviewWindowInsertArgs(windows); !slices.Equal(got, tt.want) {
					t.Errorf("overviewWindowInsertArgs() = %v, want %v", got, tt.want)
				}
				return
			}
			got := ownWindowInsertArgs(windows, tt.shell, tt.previous)
			if !slices.Equal(got, tt.want) {
				t.Errorf("ownWindowInsertArgs() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestEnsureShellWindow drives the shell window check through its callers, OpenWindow and AttachArgv.
// It captures the process-global logger, so it does not run in parallel.
func TestEnsureShellWindow(t *testing.T) {
	withInjectedExecutablePath(t, func() (string, error) { return "/opt/lyx/bin/lyx", nil })

	// The fake derives its session pane listing from list-panes, placing every pane in window @0.
	const (
		selvageInSession = "%1"
		selvageElsewhere = "%99"
	)
	tests := []struct {
		name string
		// selvage is the recorded console pane id; empty records none.
		selvage string
		windows string
		// livePanes is the list-panes answer; empty uses the two-pane listing.
		livePanes string
		renameErr error
		// attachOnly skips the OpenWindow run, for rows about AttachArgv's unknown size or lone pane.
		attachOnly bool
		cols, rows int

		wantRename bool
		wantMove   string
		wantWarn   int
		wantInfo   bool
		wantSet    bool
		wantUnset  bool
	}{
		{name: "no recorded console pane issues neither", windows: "@0 0 0 0 tmux\n@1 1 0 0 explore\n"},
		{name: "a recorded console pane the session does not carry issues neither", selvage: selvageElsewhere, windows: "@0 0 0 0 tmux\n@1 1 0 0 explore\n"},
		{name: "a shell window absent from the window listing issues neither", selvage: selvageInSession, windows: "@7 0 0 0 tmux\n"},
		{name: "a misnamed shell gets the rename and the name pin", selvage: selvageInSession, windows: "@0 0 0 0 tmux\n", wantRename: true},
		{name: "a shell before a visible window moves behind the last one", selvage: selvageInSession, windows: "@0 0 0 0 shell\n@1 1 0 0 explore\n@2 2 1 0 batten:one\n", wantMove: "@1"},
		{name: "a correct shell after every visible window and before hidden ones issues neither", selvage: selvageInSession, windows: "@1 0 0 0 explore\n@0 1 0 0 shell\n@2 2 1 0 batten:one\n"},
		{
			name: "a failed rename logs one Warn and sets the episode option", selvage: selvageInSession, windows: "@0 0 0 0 tmux\n",
			renameErr: errors.New("boom"), wantRename: true, wantWarn: 1, wantSet: true,
		},
		{
			name: "a failed rename with the episode option already set logs no Warn", selvage: selvageInSession, windows: "@0 0 0 1 tmux\n",
			renameErr: errors.New("boom"), wantRename: true,
		},
		{
			name: "a succeeding check with the episode option set logs at Info and unsets it", selvage: selvageInSession, windows: "@0 0 0 1 tmux\n",
			wantRename: true, wantInfo: true, wantUnset: true,
		},
		{
			name: "an attach of unknown size still renames and moves", selvage: selvageInSession, windows: "@0 0 0 0 tmux\n@1 1 0 0 explore\n",
			attachOnly: true, cols: 0, rows: 0, wantRename: true, wantMove: "@1",
		},
		{
			name: "an attach over a session whose only pane is the console still renames and moves", selvage: selvageInSession, windows: "@0 0 0 0 tmux\n@1 1 0 0 explore\n",
			livePanes: oneAttachListPane, attachOnly: true, cols: 80, rows: 24, wantRename: true, wantMove: "@1",
		},
	}
	for _, tt := range tests {
		vias := []string{"OpenWindow", "AttachArgv"}
		if tt.attachOnly {
			vias = vias[1:]
		}
		for _, via := range vias {
			t.Run(tt.name+"/"+via, func(t *testing.T) {
				buf := logcapture.CaptureVerbose(t)
				e := newTestEngine(t)
				if err := SaveState(e.stateDir(), &ReedState{SelvagePaneID: tt.selvage}); err != nil {
					t.Fatalf("SaveState: %v", err)
				}
				fake := installFakeTmux(t, e)
				livePanes := tt.livePanes
				if livePanes == "" {
					livePanes = goodAttachListPanes
				}
				fake.answer("list-panes", livePanes, nil)
				fake.answer("list-windows", tt.windows, nil)
				if tt.renameErr != nil {
					fake.answer("rename-window", "", tt.renameErr)
				}
				fake.answer("-V", fakeVersionOutput, nil)
				fake.answer("list-commands", fakeFullCommandsOutput(), nil)
				fake.answer("display-message", "$0|4321|1700000000", nil)
				fake.answerFormat("#{remain-on-exit}", "off\n", nil)
				fake.answer("new-window", "@9\n", nil)

				if via == "OpenWindow" {
					if _, err := e.OpenWindow("batten:one", []string{"300"}); err != nil {
						t.Fatalf("OpenWindow: %v", err)
					}
					// OpenWindow's own chained rename-window rides inside its one new-window call, so the verb count below is the check's alone.
				} else {
					cols, rows := tt.cols, tt.rows
					if cols == 0 && rows == 0 && !tt.attachOnly {
						cols, rows = 80, 24
					}
					e.AttachArgv(cols, rows)
				}

				if got := fake.Count("rename-window") > 0; got != tt.wantRename {
					t.Errorf("rename-window issued = %v, want %v: %v", got, tt.wantRename, fake.Sequence())
				}
				pins := 0
				for _, argv := range fake.ArgvFor("set-option") {
					if slices.Contains(argv, "automatic-rename") && slices.Contains(argv, "off") {
						pins++
					}
				}
				if (pins > 0) != (tt.wantRename && tt.renameErr == nil) {
					t.Errorf("automatic-rename pins = %d, want one exactly when the rename succeeds", pins)
				}
				moves := fake.ArgvFor("move-window")
				if tt.wantMove == "" && len(moves) != 0 {
					t.Errorf("move-window calls = %v, want none", moves)
				}
				if tt.wantMove != "" && (len(moves) != 1 || moves[0][len(moves[0])-1] != tt.wantMove || !slices.Contains(moves[0], "-a")) {
					t.Errorf("move-window calls = %v, want one -a move behind %s", moves, tt.wantMove)
				}
				if got := countLogLines(buf.String(), "WARN", "could not repair the shell window"); got != tt.wantWarn {
					t.Errorf("repair Warn lines = %d, want %d; log:\n%s", got, tt.wantWarn, buf.String())
				}
				if got := countLogLines(buf.String(), "INFO", "the shell window is in place again") > 0; got != tt.wantInfo {
					t.Errorf("recovery Info logged = %v, want %v; log:\n%s", got, tt.wantInfo, buf.String())
				}
				var set, unset bool
				for _, argv := range fake.ArgvFor("set-option") {
					if !strings.Contains(strings.Join(argv, " "), shellCheckFailedOption) {
						continue
					}
					if slices.Contains(argv, "-u") {
						unset = true
					} else {
						set = true
					}
				}
				if set != tt.wantSet || unset != tt.wantUnset {
					t.Errorf("episode option set = %v, unset = %v, want %v and %v", set, unset, tt.wantSet, tt.wantUnset)
				}
			})
		}
	}
}
