// reopen_window_test.go asserts battenPreRun reopens the terminal window of a resumed run only when no client is attached.

package battencli

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// TestBattenPreRun_ReopensTheWindowWithNoClientAttached runs the pre-run over a fake receiver whose command runner records each command, for every status and client answer that decides whether the Open-Window command runs.
func TestBattenPreRun_ReopensTheWindowWithNoClientAttached(t *testing.T) {
	t.Parallel()

	pastOpenWindowStatus := shedengine.Status{
		CurrentProducer: battenrecipe.NameRunShed,
		State:           shedengine.StateRunning,
		History:         []shedengine.HistoryEntry{{Producer: battenrecipe.NameOpenWindow, Outcome: shedengine.Done}},
	}
	atOpenWindowStatus := shedengine.Status{
		CurrentProducer: battenrecipe.NameOpenWindow,
		State:           shedengine.StateRunning,
		History:         []shedengine.HistoryEntry{{Producer: battenrecipe.NameOpenWindow, Outcome: shedengine.Done}},
	}
	attached := func(value bool, err error) func() (bool, error) {
		return func() (bool, error) { return value, err }
	}

	tests := []struct {
		name           string
		status         *shedengine.Status
		clientAttached func() (bool, error)
		switchOn       bool
		forced         bool
		wantCommands   int
	}{
		{"PastOpenWindowWithNoClient", &pastOpenWindowStatus, attached(false, nil), true, false, 1},
		{"ClientAttached", &pastOpenWindowStatus, attached(true, nil), true, false, 0},
		{"SwitchOff", &pastOpenWindowStatus, attached(false, nil), false, false, 0},
		{"SwitchOffAndOpenTerminalForced", &pastOpenWindowStatus, attached(false, nil), false, true, 1},
		{"ClientCheckErrorOpensNone", &pastOpenWindowStatus, attached(false, errors.New("tmux unreachable")), true, false, 0},
		{"StillAtOpenWindow", &atOpenWindowStatus, attached(false, nil), true, false, 0},
		{"FreshStart", nil, attached(false, nil), true, false, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := newFakeReceiver(t, nil)
			var commands []string
			c.env.Setting = func(module, key string) (any, error) { return tt.switchOn, nil }
			c.env.ExecutableSetting = func(module, key string) (any, error) { return "open-window {slug}", nil }
			c.env.Command = shedadapters.CommandDeps{
				Slug:    c.slug,
				Dir:     t.TempDir(),
				LogPath: func(row string) string { return row + ".log" },
				Run: func(ctx context.Context, dir, command, logPath string, timeout time.Duration) (shedadapters.CommandResult, error) {
					commands = append(commands, command)
					return shedadapters.CommandResult{}, nil
				},
				GOOS: "linux",
			}
			if tt.forced {
				c.env.Command.Forced = []string{battenrecipe.NameOpenWindow}
			}
			c.clientAttached = tt.clientAttached
			if tt.status != nil {
				writeStatus(t, c, *tt.status)
			}

			if err := c.battenPreRun(context.Background()); err != nil {
				t.Fatalf("battenPreRun() = %v; want nil", err)
			}
			if len(commands) != tt.wantCommands {
				t.Errorf("battenPreRun() ran commands %q; want %d", commands, tt.wantCommands)
			}
		})
	}
}
