// window_test.go covers OpenWindow against the scripted multiplexer fake: each outcome of the op and the command line it composes.

package reedengine

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shell"
)

// TestOpenWindow drives every outcome of OpenWindow.
// The injected executable path makes it process-global, so it does not run in parallel.
func TestOpenWindow(t *testing.T) {
	withInjectedExecutablePath(t, func() (string, error) { return "/opt/lyx/bin/lyx", nil })
	const name = "batten:slug"
	sh := shell.ForGOOS()
	wantCommand := sh.Chain(paneBinPrelude(sh, "/opt/lyx/bin/lyx"), sh.Invoke("lyx")+" "+sh.Quote("batten")+" "+sh.Quote("run")+" "+sh.Quote("my slug"))
	args := []string{"batten", "run", "my slug"}

	tests := []struct {
		name   string
		script func(f *fakeTmux)
		// wantPlacement is what follows new-window -d; nil means the plain form, -t on the exact session target.
		wantPlacement []string
		wantErr       error
		wantCapErr    bool
		want          WindowResult
		wantNewCalls  int
		wantKilled    []string
	}{
		{
			name:         "OpensADetachedWindow",
			script:       func(f *fakeTmux) { f.answer("new-window", "@7\n", nil) },
			want:         WindowResult{WindowID: "@7", Name: name},
			wantNewCalls: 1,
		},
		{
			name: "AHiddenWindowOpensAfterTheHighestWindow",
			script: func(f *fakeTmux) {
				f.answer("list-windows", "@0 0 0 0 shell\n@5 3 1 0 batten:old\n@2 1 0 0 explore\n", nil)
				f.answer("new-window", "@7\n", nil)
			},
			wantPlacement: []string{"-a", "-t", "@5"},
			want:          WindowResult{WindowID: "@7", Name: name},
			wantNewCalls:  1,
		},
		{
			name: "ALiveWindowOfThatNameIsReportedAndNothingStarts",
			script: func(f *fakeTmux) {
				f.answer(sessionListVerb, "@0 %1 0 0 tmux\n@4 %9 0 0 "+name+"\n", nil)
			},
			want: WindowResult{WindowID: "@4", Name: name, Existing: true},
		},
		{
			name: "ALiveOwnWindowOfThatNameIsNeitherReturnedNorKilledAndANewWindowOpens",
			script: func(f *fakeTmux) {
				f.answer(sessionListVerb, "@4 %9 0 1 "+name+"\n", nil)
				f.answer("new-window", "@8\n", nil)
			},
			want:         WindowResult{WindowID: "@8", Name: name},
			wantNewCalls: 1,
		},
		{
			name: "ADeadWindowOfThatNameIsKilledAndReplaced",
			script: func(f *fakeTmux) {
				f.answer(sessionListVerb, "@4 %9 1 0 "+name+"\n", nil)
				f.answer("new-window", "@8\n", nil)
			},
			want:         WindowResult{WindowID: "@8", Name: name},
			wantNewCalls: 1,
			wantKilled:   []string{"%9"},
		},
		{
			name:    "NoSessionCreatesNothing",
			script:  func(f *fakeTmux) { f.answer("has-session", "", exitCodeErr{code: 1}) },
			wantErr: ErrNoSession,
		},
		{
			name: "AMultiplexerWithoutNewWindowIsACapabilityError",
			script: func(f *fakeTmux) {
				f.answer("list-commands", strings.ReplaceAll(fakeFullCommandsOutput(), "new-window", "new-wind0w"), nil)
			},
			wantCapErr: true,
		},
		{
			name:         "AFailedNewWindowReturnsItsSentinel",
			script:       func(f *fakeTmux) { f.answer("new-window", "", errors.New("no space")) },
			wantErr:      ErrNewWindowFailed,
			wantNewCalls: 1,
		},
		{
			name: "AReadBackOtherThanOffKillsTheWindowByIdAndReturnsItsSentinel",
			script: func(f *fakeTmux) {
				f.answer("new-window", "@7\n", nil)
				f.answerFormat("#{remain-on-exit}", "on\n", nil)
			},
			wantErr:      ErrWindowReadBack,
			wantNewCalls: 1,
			wantKilled:   []string{"@7"},
		},
		{
			name: "AWindowGoneBeforeItsReadBackIsStillOpened",
			script: func(f *fakeTmux) {
				f.answer("new-window", "@7\n", nil)
				f.answerFormat("#{remain-on-exit}", "", errors.New("can't find window @7"))
			},
			want:         WindowResult{WindowID: "@7", Name: name},
			wantNewCalls: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestEngine(t)
			fake := installFakeTmux(t, e)
			fake.answer("-V", fakeVersionOutput, nil)
			fake.answer("list-commands", fakeFullCommandsOutput(), nil)
			fake.answerFormat("#{remain-on-exit}", "off\n", nil)
			tt.script(fake)

			got, err := e.OpenWindow(name, args)

			switch {
			case tt.wantCapErr:
				var capErr *CapabilityError
				if !errors.As(err, &capErr) {
					t.Fatalf("OpenWindow() error = %v, want *CapabilityError", err)
				}
			case tt.wantErr != nil:
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("OpenWindow() error = %v, want it to wrap %v", err, tt.wantErr)
				}
			case err != nil:
				t.Fatalf("OpenWindow() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("OpenWindow() = %+v, want %+v", got, tt.want)
			}
			newCalls := fake.ArgvFor("new-window")
			if len(newCalls) != tt.wantNewCalls {
				t.Fatalf("new-window calls = %d, want %d: %v", len(newCalls), tt.wantNewCalls, fake.Calls())
			}
			var killed []string
			for _, argv := range fake.ArgvFor("kill-pane") {
				killed = append(killed, argv[len(argv)-1])
			}
			if !slices.Equal(killed, tt.wantKilled) {
				t.Errorf("kill-pane targets = %v, want %v", killed, tt.wantKilled)
			}
			if tt.wantNewCalls == 0 || len(newCalls) == 0 {
				return
			}
			nameFlag := slices.Index(newCalls[0], "-n")
			if nameFlag < 0 || nameFlag+1 >= len(newCalls[0]) || !strings.HasPrefix(newCalls[0][nameFlag+1], temporaryWindowNamePrefix) {
				t.Fatalf("new-window argv = %v, want a temporary window name after -n", newCalls[0])
			}
			temporaryName := newCalls[0][nameFlag+1]
			temporaryTarget := exactSessionWindowTarget(e.SessionName()) + "=" + temporaryName
			placement := tt.wantPlacement
			if placement == nil {
				placement = []string{"-t", exactSessionWindowTarget(e.SessionName())}
			}
			wantArgv := append(append([]string{"new-window", "-d"}, placement...),
				"-P", "-F", "#{window_id}", "-n", temporaryName, wantCommand,
				";", "set-option", "-w", "-t", temporaryTarget, "remain-on-exit", "off",
				";", "set-option", "-w", "-t", temporaryTarget, "@lyx_hidden", "1",
				";", "rename-window", "-t", temporaryTarget, name)
			if !slices.Equal(newCalls[0], wantArgv) {
				t.Errorf("new-window argv = %v, want %v", newCalls[0], wantArgv)
			}
			if got := slices.Index(newCalls[0], ";"); strings.Count(strings.Join(newCalls[0][:got], " "), " -t ") != 1 {
				t.Errorf("new-window command %v, want exactly one -t", newCalls[0][:got])
			}
		})
	}
}
