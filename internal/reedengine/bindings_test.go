// bindings_test.go pins the output of `bindingArgvs`: the Alt keys and the status click.
// Each is a root-table binding, and no other key is bound.
// It also pins the bindings' build stamp and the op lock's re-pin of a server an older build pinned.

package reedengine

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/buildvcs"
	"github.com/Knatte18/loomyard/internal/shell"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestBindingArgvs_BindsTheFiveAltKeysAndTheStatusClickInTheRootTableOnly(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const prev, next = "switch --prev", "switch --next"
	tests := []struct {
		name       string
		prev, next string
		wantKeys   []string
	}{
		{"both switch commands", prev, next, []string{"M-Up", "M-Down", "M-Left", "M-Right", "MouseDown1Status"}},
		{"no switch commands leaves their keys unbound", "", "", []string{"M-Up", "M-Down", "MouseDown1Status"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			argvs := bindingArgvs(tt.prev, tt.next, false, true, true)
			if want := []string{"unbind-key", "-n", "M-z"}; !slices.Equal(argvs[0], want) {
				t.Fatalf("first binding argv = %q, want %q ahead of every bind-key", argvs[0], want)
			}
			var keys []string
			for _, argv := range argvs[1:] {
				if len(argv) < 4 || argv[0] != "bind-key" || argv[1] != "-n" {
					t.Fatalf("binding argv %q, want it to start with bind-key -n <key> <command>", argv)
				}
				keys = append(keys, argv[2])
			}
			if !slices.Equal(keys, tt.wantKeys) {
				t.Errorf("bound keys = %v, want %v", keys, tt.wantKeys)
			}
		})
	}

	t.Run("M-Left and M-Right run the told command in the background", func(t *testing.T) {
		t.Parallel()

		got := map[string][]string{}
		for _, argv := range bindingArgvs(prev, next, true, true, true) {
			got[argv[2]] = argv
		}
		if want := []string{"bind-key", "-n", "M-Left", "run-shell", "-b", prev}; !slices.Equal(got["M-Left"], want) {
			t.Errorf("M-Left argv = %q, want %q", got["M-Left"], want)
		}
		if want := []string{"bind-key", "-n", "M-Right", "run-shell", "-b", next}; !slices.Equal(got["M-Right"], want) {
			t.Errorf("M-Right argv = %q, want %q", got["M-Right"], want)
		}
	})

	t.Run("the steps move across windows past hidden ones", func(t *testing.T) {
		t.Parallel()

		got := map[string][]string{}
		for _, argv := range bindingArgvs(prev, next, true, true, true) {
			got[argv[2]] = argv
		}
		want := map[string][]string{
			"M-Up":   {"bind-key", "-n", "M-Up", "set-option", "@lym_follow", "off", `\;`, "previous-window", `\;`, "if-shell", "-F", "-t", ":", "#{@lym_hidden}", selectStrandsWindowCommand},
			"M-Down": {"bind-key", "-n", "M-Down", "set-option", "@lym_follow", "off", `\;`, "next-window", `\;`, "if-shell", "-F", "-t", ":", "#{@lym_hidden}", "select-window -t :^"},
		}
		for key, wantArgv := range want {
			if !slices.Equal(got[key], wantArgv) {
				t.Errorf("%s argv = %q, want %q", key, got[key], wantArgv)
			}
		}
	})

	t.Run("the status click dispatches on every range type", func(t *testing.T) {
		t.Parallel()

		click := clickArgv(t, bindingArgvs(prev, next, true, true, true))
		joined := strings.Join(click, " ")
		for _, rangeType := range []string{"shell", "window", "session", "follow"} {
			if !strings.Contains(joined, "#{==:#{mouse_status_range},"+rangeType+"}") {
				t.Errorf("status click binding %q does not dispatch on range %q", joined, rangeType)
			}
		}
		if strings.Contains(joined, "mouse_status_range},pane}") {
			t.Errorf("status click binding %q dispatches on the retired pane range", joined)
		}
		// The listed branches, the follow branch, the button branch and the Slack toggle branch are joined by separators.
		// There is one separator fewer than there are branches.
		if got := strings.Count(joined, `\;`); got != len(statusClickBranches)+2 {
			t.Errorf("status click binding joins its branches with %d escaped separators, want %d", got, len(statusClickBranches)+2)
		}
	})

	t.Run("the button and Slack toggle branches each end the click only when its line is pinned", func(t *testing.T) {
		t.Parallel()

		buttonBranch := []string{`\;`, "if-shell", "-F", "#{m:lymbtn*,#{mouse_status_range}}", `run-shell -b "#{E:@lym_button_command}"`}
		slackBranch := []string{`\;`, "if-shell", "-F", "#{||:#{==:#{mouse_status_range},slack},#{==:#{mouse_status_range},away}}", `run-shell -b "#{E:@lym_slack_toggle_command}"`}
		tests := []struct {
			name                    string
			buttonClick, slackClick bool
			wantTail                []string
		}{
			{"Both", true, true, slices.Concat(buttonBranch, slackBranch)},
			{"ButtonOnly", true, false, buttonBranch},
			{"SlackOnly", false, true, slackBranch},
			{"Neither", false, false, nil},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				click := clickArgv(t, bindingArgvs(prev, next, true, tt.buttonClick, tt.slackClick))
				followAt := slices.Index(click, followClickCommand(true))
				if followAt < 0 {
					t.Fatalf("click binding %q holds no follow branch", click)
				}
				if got := click[followAt+1:]; !slices.Equal(got, tt.wantTail) {
					t.Errorf("click binding after the follow branch = %q; want %q", got, tt.wantTail)
				}
			})
		}
	})

	t.Run("the shell and window clicks write follow off first and the session click does not", func(t *testing.T) {
		t.Parallel()

		click := clickArgv(t, bindingArgvs(prev, next, true, true, true))
		for _, rangeType := range []string{"shell", "window"} {
			if got := clickBranchCommand(t, click, rangeType); !strings.HasPrefix(got, "set-option @lym_follow off ; ") {
				t.Errorf("%s click command = %q, want it to start with the follow-off write", rangeType, got)
			}
		}
		if got := clickBranchCommand(t, click, "session"); strings.Contains(got, "@lym_follow") {
			t.Errorf("session click command = %q, want it to leave the follow state alone", got)
		}
	})

	t.Run("the follow click toggles the state and runs the jump line only when it is pinned", func(t *testing.T) {
		t.Parallel()

		withJump := clickBranchCommand(t, clickArgv(t, bindingArgvs(prev, next, true, true, true)), "follow")
		wantWithJump := `if-shell -F '#{==:#{@lym_follow},off}' 'set-option @lym_follow on ; run-shell -b "#{E:@lym_follow_command}"' 'set-option @lym_follow off'`
		if withJump != wantWithJump {
			t.Errorf("follow click command with the jump pinned = %q, want %q", withJump, wantWithJump)
		}

		withoutJump := clickBranchCommand(t, clickArgv(t, bindingArgvs(prev, next, false, true, true)), "follow")
		wantWithoutJump := `if-shell -F '#{==:#{@lym_follow},off}' 'set-option @lym_follow on' 'set-option @lym_follow off'`
		if withoutJump != wantWithoutJump {
			t.Errorf("follow click command without the jump = %q, want %q", withoutJump, wantWithoutJump)
		}
		if strings.Contains(withoutJump, "run-shell") {
			t.Errorf("follow click command without the jump = %q, want no run-shell", withoutJump)
		}
	})
}

// clickArgv returns the MouseDown1Status binding of argvs.
func clickArgv(t *testing.T, argvs [][]string) []string {
	t.Helper()
	for _, argv := range argvs {
		if argv[2] == "MouseDown1Status" {
			return argv
		}
	}
	t.Fatal("no MouseDown1Status binding")
	return nil
}

// clickBranchCommand returns the command the click binding runs for rangeType.
// It is the element after that branch's range condition in click.
func clickBranchCommand(t *testing.T, click []string, rangeType string) string {
	t.Helper()
	condition := "#{==:#{mouse_status_range}," + rangeType + "}"
	at := slices.Index(click, condition)
	if at == -1 || at+1 >= len(click) {
		t.Fatalf("click binding %q has no branch for range %q", click, rangeType)
	}
	return click[at+1]
}

// TestPinGlobalLineLocked pins that a told line is set on the server globally and an empty one unsets the option.
// It also pins that a failed call is logged naming what the line is for and never returned.
// It captures the process-global logger, so it does not run in parallel.
func TestPinGlobalLineLocked(t *testing.T) {
	tiergate.Require(t, 0)

	const option, what = "@lym_test_line", "the test line"
	tests := []struct {
		name    string
		line    string
		callErr error
		want    []string
		wantLog string
	}{
		{"SetsAToldLine", "echo hi", nil, []string{"set-option", "-g", option, "echo hi"}, ""},
		{"UnsetsAnEmptyLine", "", nil, []string{"set-option", "-gu", option}, ""},
		{"FailedSetIsLogged", "echo hi", errors.New("boom"), []string{"set-option", "-g", option, "echo hi"}, "failed to pin the test line"},
		{"FailedUnsetIsLogged", "", errors.New("boom"), []string{"set-option", "-gu", option}, "failed to unset the test line"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestEngine(t)
			fake := installFakeTmux(t, e)
			fake.answer("set-option", "", tt.callErr)
			logs := logcapture.CaptureVerbose(t)

			e.pinGlobalLineLocked(option, tt.line, what)

			if got := fake.Calls(); !slices.EqualFunc(got, [][]string{tt.want}, slices.Equal[[]string]) {
				t.Errorf("pinGlobalLineLocked tmux calls = %q; want %q", got, [][]string{tt.want})
			}
			if tt.wantLog == "" && logs.String() != "" {
				t.Errorf("log = %q; want nothing logged", logs.String())
			}
			if tt.wantLog != "" && !strings.Contains(logs.String(), tt.wantLog) {
				t.Errorf("log = %q; want it to contain %q", logs.String(), tt.wantLog)
			}
		})
	}
}

// TestPinBindingsLocked_PinsTheSlackLinesOnlyWhenEverythingResolves pins the two Slack options.
// It also pins the click's toggle branch.
// A resolved socket, tmux binary and executable pin the toggle click line and the state line and bind the toggle branch.
// Any one failing to resolve unsets both lines and binds no toggle branch.
// An executable that resolves to this test binary unsets both lines too and pins no button line.
func TestPinBindingsLocked_PinsTheSlackLinesOnlyWhenEverythingResolves(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("the tmux binary stand-in resolves through the executable bit")
	}

	const exe, socketPath = "/opt/lym/bin/lym", "/tmp/tmux-1000/sock"
	tests := []struct {
		name             string
		executable       func() (string, error)
		socketAnswer     string
		wantResolved     bool
		wantButtonPinned bool
	}{
		{"Resolved", func() (string, error) { return exe, nil }, socketPath, true, true},
		{"UnresolvableExecutable", func() (string, error) { return "", errors.New("no executable") }, socketPath, false, false},
		{"UnreadableSocketPath", func() (string, error) { return exe, nil }, "", false, false},
		{"TestBinaryExecutable", os.Executable, socketPath, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tmuxPath := filepath.Join(t.TempDir(), "tmux")
			if err := os.WriteFile(tmuxPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
				t.Fatalf("WriteFile tmux stand-in: %v", err)
			}
			e := newTestEngine(t)
			e.cfg.Tmux = tmuxPath
			e.executablePath = tt.executable
			fake := installFakeTmux(t, e)
			fake.answerFormat("#{socket_path}", tt.socketAnswer, nil)

			e.pinBindingsLocked()

			sh := shell.ForGOOS()
			wantToggle, _ := composeSlackToggleCommand(sh, tt.executable, socketPath, tmuxPath)
			wantState, _ := composeSlackStateCommand(sh, tt.executable)
			var wantToggleCall, wantStateCall []string
			if tt.wantResolved {
				wantToggleCall = []string{"set-option", "-g", slackToggleCommandOption, wantToggle}
				wantStateCall = []string{"set-option", "-g", slackStateCommandOption, wantState}
			} else {
				wantToggleCall = []string{"set-option", "-gu", slackToggleCommandOption}
				wantStateCall = []string{"set-option", "-gu", slackStateCommandOption}
			}
			setOptions := fake.ArgvFor("set-option")
			for _, want := range [][]string{wantToggleCall, wantStateCall} {
				if !slices.ContainsFunc(setOptions, func(call []string) bool { return slices.Equal(call, want) }) {
					t.Errorf("pinBindingsLocked set-option calls = %q; want %q among them", setOptions, want)
				}
			}
			buttonPinned := slices.ContainsFunc(setOptions, func(call []string) bool {
				return len(call) == 4 && call[1] == "-g" && call[2] == buttonCommandOption
			})
			if buttonPinned != tt.wantButtonPinned {
				t.Errorf("pinBindingsLocked set-option calls = %q; button line pinned = %v, want %v", setOptions, buttonPinned, tt.wantButtonPinned)
			}
			followPinned := slices.ContainsFunc(setOptions, func(call []string) bool {
				return len(call) == 4 && call[1] == "-g" && call[2] == followCommandOption
			})
			if followPinned != tt.wantResolved {
				t.Errorf("pinBindingsLocked set-option calls = %q; follow line pinned = %v, want %v", setOptions, followPinned, tt.wantResolved)
			}
			var boundKeys []string
			for _, argv := range fake.ArgvFor("bind-key") {
				boundKeys = append(boundKeys, argv[2])
			}
			for _, key := range []string{"M-Left", "M-Right"} {
				if got := slices.Contains(boundKeys, key); got != tt.wantResolved {
					t.Errorf("pinBindingsLocked bound keys = %q; %s bound = %v, want %v", boundKeys, key, got, tt.wantResolved)
				}
			}
			joinedClick := strings.Join(clickArgv(t, fake.ArgvFor("bind-key")), " ")
			if got := strings.Contains(joinedClick, "#{E:"+slackToggleCommandOption+"}"); got != tt.wantResolved {
				t.Errorf("click binding %q holds the Slack toggle branch = %v; want %v", joinedClick, got, tt.wantResolved)
			}
		})
	}
}

// TestBindingsBuildStamp pins the bindings' build stamp.
// It covers when a stamp is stale, what the pin writes, and the op lock re-pinning a server an older build pinned.
func TestBindingsBuildStamp(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	commitTime := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	running := buildvcs.Identity{Revision: "474fb6680aaaabbbbccccddddeeeeffff0000111", Time: commitTime}
	older := buildvcs.Identity{Revision: "675728800aaaabbbbccccddddeeeeffff0000111", Time: commitTime.Add(-time.Hour)}
	newer := buildvcs.Identity{Revision: "999999999aaaabbbbccccddddeeeeffff0000111", Time: commitTime.Add(time.Hour)}
	dirty := buildvcs.Identity{Revision: running.Revision, Modified: true, Time: commitTime}

	t.Run("StaleRule", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name    string
			stamp   string
			running buildvcs.Identity
			want    bool
		}{
			{"Empty", "", running, true},
			{"Garbled", "not-a-stamp", running, true},
			{"UnparseableTime", "yesterday 474fb6680aaa", running, true},
			{"Own", bindingsBuildStamp(running), running, false},
			{"OwnWithTrailingNewline", bindingsBuildStamp(running) + "\n", running, false},
			{"Older", bindingsBuildStamp(older), running, true},
			{"Newer", bindingsBuildStamp(newer), running, false},
			{"SameCommitTimeOtherBuild", bindingsBuildStamp(dirty), running, true},
			{"UnstampedRunning_Empty", "", buildvcs.Identity{}, false},
			{"UnstampedRunning_Older", bindingsBuildStamp(older), buildvcs.Identity{}, false},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				if got := bindingsStale(tt.stamp, tt.running); got != tt.want {
					t.Errorf("bindingsStale(%q, %v) = %v; want %v", tt.stamp, tt.running, got, tt.want)
				}
			})
		}
	})

	t.Run("PinWritesStamp", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name  string
			build buildvcs.Identity
			want  []string
		}{
			{"Stamped", running, []string{"set-option", "-g", bindingsBuildOption, "2026-10-15T12:00:00Z 474fb6680aaa"}},
			{"Unstamped", buildvcs.Identity{}, []string{"set-option", "-gu", bindingsBuildOption}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				e := newTestEngine(t)
				e.build = tt.build
				fake := installFakeTmux(t, e)

				e.pinBindingsLocked()

				var got [][]string
				for _, call := range fake.ArgvFor("set-option") {
					if slices.Contains(call, bindingsBuildOption) {
						got = append(got, call)
					}
				}
				if want := [][]string{tt.want}; !slices.EqualFunc(got, want, slices.Equal[[]string]) {
					t.Errorf("pinBindingsLocked stamp writes = %q; want %q", got, want)
				}
			})
		}
	})

	t.Run("OpLockRepinsStaleBindings", func(t *testing.T) {
		t.Parallel()
		readFailed := errors.New("no server running")
		tests := []struct {
			name       string
			build      buildvcs.Identity
			stamp      string
			readErr    error
			wantRepin  bool
			wantStamps int
		}{
			{"OlderBuildPinned", running, bindingsBuildStamp(older), nil, true, 1},
			{"NoStamp", running, "", nil, true, 1},
			{"OwnBuildPinned", running, bindingsBuildStamp(running), nil, false, 0},
			{"NewerBuildPinned", running, bindingsBuildStamp(newer), nil, false, 0},
			{"ReadFails", running, "", readFailed, false, 0},
			{"UnstampedRunning", buildvcs.Identity{}, bindingsBuildStamp(older), nil, false, 0},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				e := newTestEngine(t)
				e.build = tt.build
				fake := installFakeTmux(t, e)
				stampFormat := "#{" + bindingsBuildOption + "}"
				fake.answerFormat(stampFormat, tt.stamp, tt.readErr)

				bindKeysBeforeStep := -1
				err := e.withOpLock(func() error {
					bindKeysBeforeStep = fake.Count("bind-key")
					return nil
				})
				if err != nil {
					t.Fatalf("withOpLock() = %v; want nil", err)
				}

				if got := bindKeysBeforeStep > 0; got != tt.wantRepin {
					t.Errorf("withOpLock issued %d bind-key calls before its step; want a re-pin %v", bindKeysBeforeStep, tt.wantRepin)
				}
				if got := fake.Count("bind-key"); got != bindKeysBeforeStep {
					t.Errorf("withOpLock issued %d bind-key calls in all, %d before its step; want none after it", got, bindKeysBeforeStep)
				}
				stamps := 0
				for _, call := range fake.ArgvFor("set-option") {
					if slices.Contains(call, bindingsBuildOption) {
						stamps++
					}
				}
				if stamps != tt.wantStamps {
					t.Errorf("withOpLock wrote the build stamp %d times; want %d", stamps, tt.wantStamps)
				}
				reads := 0
				for _, call := range fake.ArgvFor("display-message") {
					if call[len(call)-1] == stampFormat {
						reads++
					}
				}
				wantReads := 0
				if tt.build.Revision != "" {
					wantReads = 1
				}
				if reads != wantReads {
					t.Errorf("withOpLock read the build stamp %d times; want %d", reads, wantReads)
				}
				var formatPins [][]string
				for _, call := range fake.Calls() {
					if call[0] == "bind-key" {
						break
					}
					if call[0] == "set-option" && slices.ContainsFunc(statusFormatPinArgvs(), func(pin []string) bool { return slices.Equal(pin, call) }) {
						formatPins = append(formatPins, call)
					}
				}
				wantFormatPins := [][]string(nil)
				if tt.wantRepin {
					wantFormatPins = statusFormatPinArgvs()
				}
				if !slices.EqualFunc(formatPins, wantFormatPins, slices.Equal[[]string]) {
					t.Errorf("withOpLock issued the status format pins %q before the first bind-key; want %q", formatPins, wantFormatPins)
				}
			})
		}
	})

	// This subtest reads the process-global logger output, so it does not run in parallel.
	t.Run("FailedFormatRepinIsNonFatal", func(t *testing.T) {
		e := newTestEngine(t)
		e.build = running
		fake := installFakeTmux(t, e)
		fake.answerFormat("#{"+bindingsBuildOption+"}", bindingsBuildStamp(older), nil)
		fake.answerFunc("set-option", func(args []string) (string, error) {
			if strings.HasPrefix(args[2], "status-format") {
				return "", errors.New("boom")
			}
			return "", nil
		})
		logs := logcapture.CaptureVerbose(t)

		if err := e.withOpLock(func() error { return nil }); err != nil {
			t.Fatalf("withOpLock() = %v; want nil", err)
		}

		if fake.Count("bind-key") == 0 {
			t.Error("withOpLock issued no bind-key call after the failed format pins; want the bindings still re-pinned")
		}
		if got, want := strings.Count(logs.String(), "failed to re-pin a status bar option"), 3; got != want {
			t.Errorf("log holds %d failed-pin Warns; want %d, one per failed status format", got, want)
		}
	})
}
