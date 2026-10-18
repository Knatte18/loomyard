// command_test.go covers CommandProducer's per-call order over fake lookups and a recording runner: the switch, the command read, the placeholder quoting, the outcome mapping and the refusals that stop the run.

package shedadapters

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/verifyrun"
)

// TestCommandProducer table-drives Call.
// It stays serial: logcapture.Capture redirects the process-wide logger, which each row's Warn assertions read.
func TestCommandProducer(t *testing.T) {
	const (
		rerun      = "lyx batten run slug-one"
		logPath    = "/hub/logs/Open-Window.log"
		setCommand = "lyx config batten --layer default --set open_command="
	)
	quote := func(goos, value string) string {
		quoted, err := verifyrun.QuoteArg(goos, value)
		if err != nil {
			t.Fatal(err)
		}
		return quoted
	}
	awkward := `two words, 'single' and "double" and 100%`

	tests := []struct {
		name string
		// off turns the switch off; switchValue, when set, replaces the bool the switch answers.
		off         bool
		switchValue any
		switchErr   error
		forced      bool
		// command is what ExecutableSetting answers; commandValue, when set, replaces it with a non-string.
		command      string
		commandValue any
		commandErr   error
		// otherCommand is what the ordinary stack answers for the command key; otherErr makes that read fail.
		otherCommand string
		otherErr     error
		noLookup     bool
		taskDir      string
		taskDirErr   error
		goos         string
		result       CommandResult
		runErr       error
		cancelled    bool

		wantErr     []string
		wantRuns    int
		wantCommand string
		// wantWarn must all appear in the captured Warn records; none means the records must be empty.
		wantWarn []string
	}{
		{name: "switch off runs nothing", off: true, command: "echo hi"},
		{name: "empty command runs nothing and is quiet unforced", command: ""},
		{name: "empty command on a forced row warns", forced: true, command: "", wantWarn: []string{"empty command"}},
		{name: "exit zero", command: "echo hi", wantRuns: 1, wantCommand: "echo hi"},
		{name: "non-zero exit warns with the log path", command: "false", result: CommandResult{ExitCode: 3}, wantRuns: 1, wantCommand: "false", wantWarn: []string{logPath, "failed"}},
		{name: "timeout warns with the log path", command: "sleep 99", result: CommandResult{TimedOut: true}, wantRuns: 1, wantCommand: "sleep 99", wantWarn: []string{logPath, "timed out"}},
		{name: "runner error warns with the log path", command: "x", runErr: errors.New("no shell"), wantRuns: 1, wantCommand: "x", wantWarn: []string{logPath, "no shell"}},
		{name: "unreadable switch", switchErr: errors.New("bad yaml"), command: "x", wantErr: []string{"batten.open_window", rerun}},
		{name: "non-bool switch", switchValue: "yes", command: "x", wantErr: []string{"batten.open_window", rerun}},
		{name: "command lookup error", commandErr: errors.New("bad yaml"), wantErr: []string{"batten.open_command", setCommand, rerun}},
		{name: "non-string command", commandValue: 7, wantErr: []string{"batten.open_command", setCommand, rerun}},
		{name: "task dir error", command: "open {task_dir}", taskDirErr: errors.New("no worktree"), wantErr: []string{"task worktree", rerun}},
		{name: "nil executable lookup", noLookup: true, command: "x", wantErr: []string{"executable-key lookup", "way forward", rerun}},
		{name: "forced row runs past a failing switch lookup", forced: true, switchErr: errors.New("bad yaml"), command: "echo hi", wantRuns: 1, wantCommand: "echo hi"},
		{
			name:        "placeholder values reach sh as quoted literals",
			command:     "open {lyx} {slug} {task_dir}",
			taskDir:     awkward,
			goos:        "linux",
			wantRuns:    1,
			wantCommand: "open " + quote("linux", "/bin/lyx") + " " + quote("linux", "slug-one") + " " + quote("linux", awkward),
		},
		{
			name:     "a percent in task dir under cmd runs nothing and warns",
			command:  "open {task_dir}",
			taskDir:  `C:\hub 100%`,
			goos:     "windows",
			wantWarn: []string{"{task_dir}", "hub"},
		},
		{name: "other brace text and shell variables pass through", command: "echo ${HOME} {other}", wantRuns: 1, wantCommand: "echo ${HOME} {other}"},
		{name: "a different ordinary-stack command is ignored with a warning", command: "echo template", otherCommand: "echo local", wantRuns: 1, wantCommand: "echo template", wantWarn: []string{"ignores", "lyx config batten --layer default --set"}},
		{name: "a failing ordinary-stack read runs the template command quietly", command: "echo template", otherErr: errors.New("boom"), wantRuns: 1, wantCommand: "echo template"},
		{name: "cancelled context errors", cancelled: true, command: "x", wantErr: []string{"cancelled"}},
	}
	buf := logcapture.Capture(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf.Reset()
			var ranCommands []string
			var rowTimeout time.Duration
			setting := func(module, key string) (any, error) {
				switch module + "." + key {
				case "batten.open_window":
					if tt.switchErr != nil {
						return nil, tt.switchErr
					}
					if tt.switchValue != nil {
						return tt.switchValue, nil
					}
					return !tt.off, nil
				case "batten.open_command":
					if tt.otherErr != nil {
						return nil, tt.otherErr
					}
					if tt.otherCommand != "" {
						return tt.otherCommand, nil
					}
					return tt.command, nil
				}
				return nil, errors.New("unexpected key " + module + "." + key)
			}
			executable := func(module, key string) (any, error) {
				if module+"."+key != "batten.open_command" {
					return nil, errors.New("unexpected key " + module + "." + key)
				}
				if tt.commandErr != nil {
					return nil, tt.commandErr
				}
				if tt.commandValue != nil {
					return tt.commandValue, nil
				}
				return tt.command, nil
			}
			deps := CommandDeps{
				Setting:           setting,
				ExecutableSetting: executable,
				Lyx:               "/bin/lyx",
				Slug:              "slug-one",
				TaskDir:           func() (string, error) { return tt.taskDir, tt.taskDirErr },
				Dir:               "/hub/prime",
				LogPath:           func(string) string { return logPath },
				Run: func(_ context.Context, dir, command, gotLog string, timeout time.Duration) (CommandResult, error) {
					if dir != "/hub/prime" || gotLog != logPath {
						t.Errorf("Run dir = %q, log = %q; want /hub/prime and %q", dir, gotLog, logPath)
					}
					ranCommands = append(ranCommands, command)
					rowTimeout = timeout
					return tt.result, tt.runErr
				},
				Rerun: rerun,
				GOOS:  tt.goos,
			}
			if tt.noLookup {
				deps.ExecutableSetting = nil
			}
			if tt.forced {
				deps.Forced = []string{"Open-Window"}
			}
			row := CommandRow{SwitchModule: "batten", SwitchKey: "open_window", CommandModule: "batten", CommandKey: "open_command", Timeout: 30 * time.Second}
			ctx := context.Background()
			if tt.cancelled {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}

			outcome, _, err := NewCommandProducer("Open-Window", row, deps).Call(ctx)

			if len(tt.wantErr) > 0 {
				if err == nil {
					t.Fatalf("Call() error = nil; want one naming %v", tt.wantErr)
				}
				for _, want := range tt.wantErr {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("Call() error = %q; want it to contain %q", err, want)
					}
				}
			} else if err != nil || outcome != shedengine.Done {
				t.Fatalf("Call() = %q, %v; want Done, nil", outcome, err)
			}
			if len(ranCommands) != tt.wantRuns {
				t.Fatalf("ran %d commands %q; want %d", len(ranCommands), ranCommands, tt.wantRuns)
			}
			if tt.wantRuns > 0 {
				if ranCommands[0] != tt.wantCommand {
					t.Errorf("ran %q; want %q", ranCommands[0], tt.wantCommand)
				}
				if rowTimeout != 30*time.Second {
					t.Errorf("Run timeout = %v; want the row's 30s", rowTimeout)
				}
			}
			warned := buf.String()
			if len(tt.wantWarn) == 0 && warned != "" && len(tt.wantErr) == 0 {
				t.Errorf("Warn records = %q; want none", warned)
			}
			for _, want := range tt.wantWarn {
				if !strings.Contains(warned, want) {
					t.Errorf("Warn records = %q; want them to contain %q", warned, want)
				}
			}
		})
	}
}
