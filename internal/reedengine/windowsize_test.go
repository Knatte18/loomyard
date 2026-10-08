// windowsize_test.go covers windowsize.go's pure parsers/predicates and its four *Locked tmux
// round trips, every one driven through TmuxCmd's execHook seam (no live server, no external process
// spawn, no sleep) — the shape generation_test.go and strand_test.go already use.

package reedengine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"testing"

	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/shell"
)

//testtiming:keep pins the window-size parser: a well-formed pair with trailing newline or extra whitespace parses, and an empty, one-field, three-field, non-numeric, zero or negative answer is rejected; its covering tests run this code without asserting it
func TestParseWindowSize(t *testing.T) {
	tests := []struct {
		name   string
		out    string
		wantW  int
		wantH  int
		wantOK bool
	}{
		{"WellFormed", "220 50", 220, 50, true},
		{"TrailingNewline", "220 50\n", 220, 50, true},
		{"ExtraWhitespace", "  220   50  ", 220, 50, true},
		{"Empty", "", 0, 0, false},
		{"OneField", "220", 0, 0, false},
		{"ThreeFields", "220 50 7", 0, 0, false},
		{"NonNumeric", "abc def", 0, 0, false},
		{"ZeroWidth", "0 50", 0, 0, false},
		{"ZeroHeight", "220 0", 0, 0, false},
		{"NegativeWidth", "-1 50", 0, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, h, ok := parseWindowSize(tt.out)
			if w != tt.wantW || h != tt.wantH || ok != tt.wantOK {
				t.Errorf("parseWindowSize(%q) = (%d, %d, %v), want (%d, %d, %v)", tt.out, w, h, ok, tt.wantW, tt.wantH, tt.wantOK)
			}
		})
	}
}

//testtiming:keep pins the live box readback: a well-formed pair is returned live and a garbage, empty, non-positive or errored answer falls back to the configured size and reports not live; its covering tests run this code without asserting it
func TestLiveBoxLocked(t *testing.T) {
	tests := []struct {
		name   string
		answer string
		err    error
		wantW  int
		wantH  int
		wantOK bool
	}{
		{"WellFormedLivePair", "220 50", nil, 220, 50, true},
		{"Garbage", "abc def", nil, 999, 111, false},
		{"Empty", "", nil, 999, 111, false},
		{"NonPositiveDimension", "220 0", nil, 999, 111, false},
		{"RoundTripError", "", errors.New("boom"), 999, 111, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestEngine(t)
			// Distinct from the scripted live pair (220x50) so a fallback
			// cannot pass by coincidence.
			e.cfg.Width, e.cfg.Height = 999, 111
			installFakeTmux(t, e).answer("display-message", tt.answer, tt.err)

			got, ok := e.liveBoxLocked(exactSessionWindowTarget(e.SessionName()))
			want := render.Box{X: 0, Y: 0, W: tt.wantW, H: tt.wantH}
			if got != want || ok != tt.wantOK {
				t.Errorf("liveBoxLocked() = (%+v, %v), want (%+v, %v)", got, ok, want, tt.wantOK)
			}
		})
	}
}

//testtiming:keep pins the status readback as the reserved-row source: off is zero rows, on one, a non-negative integer that many, case and padding are tolerated, and an empty, garbage or negative answer is rejected; its covering tests run this code without asserting it
func TestReservedRowsFromStatus(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		wantRows int
		wantOK   bool
	}{
		{"Off", "off", 0, true},
		{"On", "on", 1, true},
		{"NumericTwo", "2", 2, true},
		{"UppercaseOff", "OFF", 0, true},
		{"PaddedOn", " on ", 1, true},
		{"Empty", "", 0, false},
		{"Garbage", "garbage", 0, false},
		{"Negative", "-1", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows, ok := reservedRowsFromStatus(tt.raw)
			if rows != tt.wantRows || ok != tt.wantOK {
				t.Errorf("reservedRowsFromStatus(%q) = (%d, %v), want (%d, %v)", tt.raw, rows, ok, tt.wantRows, tt.wantOK)
			}
		})
	}
}

//testtiming:keep pins which window-size readbacks allow the chain: only latest, in any case and padding, while manual, largest, smallest and empty do not; its covering tests run this code without asserting it
func TestWindowSizeAllowsChain(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want bool
	}{
		{"Latest", "latest", true},
		{"UppercaseLatest", "LATEST", true},
		{"PaddedLatest", " latest ", true},
		{"Manual", "manual", false},
		{"Largest", "largest", false},
		{"Smallest", "smallest", false},
		{"Empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := windowSizeAllowsChain(tt.raw); got != tt.want {
				t.Errorf("windowSizeAllowsChain(%q) = %v, want %v", tt.raw, got, tt.want)
			}
		})
	}
}

// TestReadbacksLocked pins the two single-value tmux readbacks: the status row count (on reserves one row) and the window-size latest check,
// each answering from a scripted display-message and degrading on a round-trip error.
//
//testtiming:keep pins the status-row, window-size-latest and border-title-row readbacks answering from a scripted display-message and degrading on a round-trip error; its covering tests run this code without asserting it
func TestReadbacksLocked(t *testing.T) {
	tests := []struct {
		name   string
		answer string
		err    error
		read   func(e *Engine) string
		want   string
	}{
		{
			name:   "StatusRowsScriptedAnswer",
			answer: "on",
			read: func(e *Engine) string {
				rows, ok := e.readStatusRowsLocked(exactSessionWindowTarget(e.SessionName()))
				return fmt.Sprintf("(%d, %v)", rows, ok)
			},
			want: "(1, true)",
		},
		{
			name: "StatusRowsRoundTripError",
			err:  errors.New("boom"),
			read: func(e *Engine) string {
				rows, ok := e.readStatusRowsLocked(exactSessionWindowTarget(e.SessionName()))
				return fmt.Sprintf("(%d, %v)", rows, ok)
			},
			want: "(0, false)",
		},
		{
			name:   "WindowSizeLatestScriptedAnswer",
			answer: "latest",
			read: func(e *Engine) string {
				return strconv.FormatBool(e.readWindowSizeLatestLocked(exactSessionWindowTarget(e.SessionName())))
			},
			want: "true",
		},
		{
			name: "WindowSizeLatestRoundTripError",
			err:  errors.New("boom"),
			read: func(e *Engine) string {
				return strconv.FormatBool(e.readWindowSizeLatestLocked(exactSessionWindowTarget(e.SessionName())))
			},
			want: "false",
		},
	}
	readBorderTitleRow := func(e *Engine) string {
		return strconv.FormatBool(e.readBorderTitleRowLocked(exactSessionWindowTarget(e.SessionName())))
	}
	for _, answer := range []struct{ name, answer, want string }{
		{"Top", "top\n", "true"},
		{"Bottom", "bottom", "false"},
		{"Off", "off", "false"},
		{"Empty", "", "false"},
	} {
		tests = append(tests, struct {
			name   string
			answer string
			err    error
			read   func(e *Engine) string
			want   string
		}{"BorderTitleRow" + answer.name, answer.answer, nil, readBorderTitleRow, answer.want})
	}
	tests = append(tests, struct {
		name   string
		answer string
		err    error
		read   func(e *Engine) string
		want   string
	}{"BorderTitleRowRoundTripError", "", errors.New("boom"), readBorderTitleRow, "false"})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestEngine(t)
			installFakeTmux(t, e).answer("display-message", tt.answer, tt.err)
			if got := tt.read(e); got != tt.want {
				t.Errorf("readback = %s, want %s", got, tt.want)
			}
		})
	}
}

// TestPinGeometryOptionsLocked drives pinGeometryOptionsLocked against the fake tmux, recording every set-option argv issued.
// It asserts the two-line bar's pins plus the pre-existing window-size pin are all issued with the expected target and value:
// both status formats global, status 2 and status-position on the session, the pane-border status and format on the strand window only.
// that no call's failure stops the calls after it,
// and that the window-resized hook lifecycle is left to unset the hook and clean the signal file when the watchdog is off.
//
//testtiming:keep pins the set-option calls issued to pin the geometry: the two global status formats, status 2 and the position on the session, the border status and format on the strand window and window-size, no failure stopping the calls after it, and the window-resized hook lifecycle left to unset and clean the signal file when the watchdog is off; its covering tests run this code without asserting it
func TestPinGeometryOptionsLocked(t *testing.T) {
	t.Run("AllOptionsIssued", func(t *testing.T) {
		e := newTestEngine(t)
		fake := installFakeTmux(t, e)

		target := exactSessionWindowTarget(e.SessionName())
		e.pinGeometryOptionsLocked(target)
		calls := fake.ArgvFor("set-option")

		wantOptions := [][]string{
			{"set-option", "-g", "status-format[0]", statusFormatButtons},
			{"set-option", "-g", "status-format[1]", statusFormatSessions},
			{"set-option", "-g", "status-style", statusBarStyle},
			{"set-option", "-t", target, "status", "2"},
			{"set-option", "-t", target, "status-position", "bottom"},
			{"set-option", "-w", "-t", target, "pane-border-status", "top"},
			{"set-option", "-w", "-t", target, "pane-border-format", paneBorderFormat},
			{"set-option", "-w", "-t", target, "window-size", "latest"},
		}
		if !slices.EqualFunc(calls, wantOptions, slices.Equal[[]string]) {
			t.Errorf("pinGeometryOptionsLocked set-option calls = %v, want %v", calls, wantOptions)
		}
	})

	t.Run("BindingsAreIssuedAfterTheOptionPins", func(t *testing.T) {
		e := newTestEngine(t)
		fake := installFakeTmux(t, e)

		e.pinGeometryOptionsLocked(exactSessionWindowTarget(e.SessionName()))

		lastOption, firstBinding, bindings := -1, -1, 0
		for i, call := range fake.Calls() {
			switch call[0] {
			case "set-option":
				lastOption = i
			case "bind-key":
				bindings++
				if firstBinding == -1 {
					firstBinding = i
				}
			}
		}
		// The fixture's tmux binary does not exist, so the two session-switch keys are left unbound.
		if want := len(bindingArgvs("", "")); bindings != want {
			t.Errorf("pinGeometryOptionsLocked issued %d bind-key calls, want %d", bindings, want)
		}
		if firstBinding < lastOption {
			t.Errorf("first bind-key call at %d precedes the last set-option call at %d, want the bindings after the option pins", firstBinding, lastOption)
		}
	})

	t.Run("OneOptionFailureDoesNotStopTheRest", func(t *testing.T) {
		e := newTestEngine(t)
		fake := installFakeTmux(t, e)
		fake.answerFunc("set-option", func([]string) (string, error) {
			if fake.Count("set-option") == 1 {
				return "", errors.New("boom")
			}
			return "", nil
		})

		e.pinGeometryOptionsLocked(exactSessionWindowTarget(e.SessionName()))
		calls := fake.ArgvFor("set-option")

		const wantCalls = 8
		if len(calls) != wantCalls {
			t.Fatalf("pinGeometryOptionsLocked issued %d set-option calls despite one erroring, want all %d still attempted: %v", len(calls), wantCalls, calls)
		}
	})
	t.Run("WatchdogOnPinsGeometryOptionsOnly", func(t *testing.T) {
		e := newTestEngine(t)
		e.cfg.Watchdog = "on"
		fake := installFakeTmux(t, e)

		e.pinGeometryOptionsLocked(exactSessionWindowTarget(e.SessionName()))
		calls := fake.Calls()

		// With the new resize-pin mechanism, pinGeometryOptionsLocked no longer installs the
		// window-resized hook — that is now the job of installResizePinsLocked, called from
		// apply.go and attach.go. This function only pins the geometry options.
		var hookCall []string
		for _, c := range calls {
			if c[0] == "set-hook" {
				hookCall = c
			}
		}
		if hookCall != nil {
			t.Fatalf("pinGeometryOptionsLocked calls = %v, want no set-hook call (hook installation moved to installResizePinsLocked)", calls)
		}
		// Verify that geometry options were pinned instead.
		var setOptionCalls int
		for _, c := range calls {
			if c[0] == "set-option" {
				setOptionCalls++
			}
		}
		if setOptionCalls != 8 {
			t.Errorf("pinGeometryOptionsLocked calls = %v, want 8 set-option calls (the seven bar and border options and window-size)", calls)
		}
	})

	t.Run("WatchdogOffUnsetsHookAndRemovesSignalFile", func(t *testing.T) {
		e := newTestEngine(t)
		e.cfg.Watchdog = "off"
		signalPath := e.resizeSignalPath()
		if err := os.MkdirAll(filepath.Dir(signalPath), 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(signalPath, nil, 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}

		fake := installFakeTmux(t, e)

		e.pinGeometryOptionsLocked(exactSessionWindowTarget(e.SessionName()))
		calls := fake.Calls()

		wantTarget := exactSessionWindowTarget(e.SessionName())
		want := []string{"set-hook", "-u", "-t", wantTarget, windowResizedHookName}
		var hookCall []string
		for _, c := range calls {
			if c[0] == "set-hook" {
				hookCall = c
			}
		}
		if hookCall == nil {
			t.Fatalf("pinGeometryOptionsLocked calls = %v, want a set-hook -u call", calls)
		}
		if len(hookCall) != len(want) {
			t.Fatalf("set-hook argv = %v, want %v", hookCall, want)
		}
		for i := range want {
			if hookCall[i] != want[i] {
				t.Errorf("set-hook argv[%d] = %q, want %q (full argv %v)", i, hookCall[i], want[i], hookCall)
			}
		}
		if _, err := os.Stat(signalPath); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("signal file stat err = %v, want fs.ErrNotExist (file should be removed)", err)
		}
	})

	t.Run("InvalidWatchdogBehavesLikeOff", func(t *testing.T) {
		e := newTestEngine(t)
		e.cfg.Watchdog = "bogus"
		installFakeTmux(t, e)
		// Must not panic and pinGeometryOptionsLocked returns nothing, so simply calling it and
		// returning normally is the assertion.
		e.pinGeometryOptionsLocked(exactSessionWindowTarget(e.SessionName()))
	})

	t.Run("SetHookErrorIsNonFatalWhenWatchdogOff", func(t *testing.T) {
		e := newTestEngine(t)
		e.cfg.Watchdog = "off"
		fake := installFakeTmux(t, e)
		fake.answer("set-hook", "", errors.New("boom"))

		e.pinGeometryOptionsLocked(exactSessionWindowTarget(e.SessionName()))

		if setOptionCalls := fake.Count("set-option"); setOptionCalls != 8 {
			t.Errorf("set-option calls = %d, want 8(all preceding pins still attempted despite the later set-hook error)", setOptionCalls)
		}
		if setHookErrors := fake.Count("set-hook"); setHookErrors != 1 {
			t.Errorf("set-hook errors = %d, want 1", setHookErrors)
		}
	})

	t.Run("RemovingAnAbsentSignalFileIsSilent", func(t *testing.T) {
		e := newTestEngine(t)
		e.cfg.Watchdog = "off"
		installFakeTmux(t, e)
		// The signal file's parent dir may not even exist yet; removeResizeSignalFileLocked must not
		// panic or log anything above Warn-worthy for a genuinely absent file.
		e.pinGeometryOptionsLocked(exactSessionWindowTarget(e.SessionName()))
	})
}

// containsArg reports whether want appears verbatim anywhere in args.
func containsArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

// TestResizePinHookArgvs pins the pure argv shape resizePinHookArgvs builds:
// the unconditional clear always leads (exactly "set-hook -u -w -t <target> window-resized").
// Then, when there is any pin, come the zoom record entry, one entry per pin whose body is exactly "resize-pane -t <pane> -y <height>", and the zoom restore entry.
// Then the watchdog's own touch entry comes last when a signal command is told,
// so a resize fires the pin fixups before the watcher is told about it and a zero-pin session still installs the touch.
// The "-a" flag appears on every content entry after the first, none carries a bare ";" element, and no repaint entry ships:
// neither measured repaint candidate cleared the repaint-must-not-self-retrigger decision's exactly-one-fire criterion
// (the Measurement record in internal/reedengine/doc.go), so the array holds exactly the clear, the pins and the signal entry.
//
//testtiming:keep pins the resize hook array: the exact clear first, one resize-pane entry per pin, the watchdog's touch entry last and even for zero pins, -a only after the first entry, no empty or bare ; element and no repaint entry; its covering tests run this code without asserting it
func TestResizePinHookArgvs(t *testing.T) {
	const session = "myproj"
	const signalCommand = `run-shell -b "sh -c 'touch \"/tmp/wt/.lyx/reed-resize.signal\"'"`
	target := exactSessionWindowTarget(session)
	tests := []struct {
		name       string
		pins       []render.Pin
		signal     string
		wantBodies []string
	}{
		{name: "ZeroPins"},
		{
			name:       "OnePin",
			pins:       []render.Pin{{PaneID: "%1", Height: 3}},
			wantBodies: []string{zoomRecordHookBody, "resize-pane -t %1 -y 3", zoomRestoreHookBody},
		},
		{
			name:       "ThreePins",
			pins:       []render.Pin{{PaneID: "%1", Height: 3}, {PaneID: "%2", Height: 2}, {PaneID: "%3", Height: 4}},
			wantBodies: []string{zoomRecordHookBody, "resize-pane -t %1 -y 3", "resize-pane -t %2 -y 2", "resize-pane -t %3 -y 4", zoomRestoreHookBody},
		},
		{
			name:       "ZeroPinsStillInstallsTheSignalEntry",
			signal:     signalCommand,
			wantBodies: []string{signalCommand},
		},
		{
			name:       "PinsThenTheSignalEntryLast",
			pins:       []render.Pin{{PaneID: "%1", Height: 3}, {PaneID: "%2", Height: 2}},
			signal:     signalCommand,
			wantBodies: []string{zoomRecordHookBody, "resize-pane -t %1 -y 3", "resize-pane -t %2 -y 2", zoomRestoreHookBody, signalCommand},
		},
		{
			name:       "EmptySignalCommandEmitsNoEntry",
			pins:       []render.Pin{{PaneID: "%1", Height: 3}},
			wantBodies: []string{zoomRecordHookBody, "resize-pane -t %1 -y 3", zoomRestoreHookBody},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			argvs := resizePinHookArgvs(exactSessionWindowTarget(session), tt.pins, tt.signal)

			assertResizePinHookArgvsWellFormed(t, argvs, target)
			if len(argvs) != 1+len(tt.wantBodies) {
				t.Fatalf("resizePinHookArgvs() = %v, want %d argvs (the clear + %d entries)", argvs, 1+len(tt.wantBodies), len(tt.wantBodies))
			}
			wantClear := []string{"set-hook", "-u", "-w", "-t", target, "window-resized"}
			if !slices.Equal(argvs[0], wantClear) {
				t.Errorf("clear argv = %v, want %v", argvs[0], wantClear)
			}
			for i, wantBody := range tt.wantBodies {
				argv := argvs[i+1]
				if got := argv[len(argv)-1]; got != wantBody {
					t.Errorf("entry %d body = %q, want %q", i, got, wantBody)
				}
				if appended := containsArg(argv, "-a"); appended != (i > 0) {
					t.Errorf("entry %d argv = %v, want -a only on entries after the first", i, argv)
				}
			}
		})
	}
}

// assertResizePinHookArgvsWellFormed asserts the invariants every argv resizePinHookArgvs emits
// carries, whatever the pin set or signal command: the sequence is non-empty, every argv is a
// set-hook against the exact-match window target's window-resized option, and none of them carries a
// bare ";" element (set-hook takes its body as one argument, so a separate ";" would terminate the
// set-hook itself).
func assertResizePinHookArgvsWellFormed(t *testing.T, argvs [][]string, target string) {
	t.Helper()
	if len(argvs) == 0 {
		t.Fatal("resizePinHookArgvs() = empty slice, want at least the clear")
	}
	for i, argv := range argvs {
		if argv[0] != "set-hook" {
			t.Errorf("argv[%d][0] = %q, want %q", i, argv[0], "set-hook")
		}
		if !containsArg(argv, "-w") {
			t.Errorf("argv[%d] = %v, want -w", i, argv)
		}
		if !containsArg(argv, target) {
			t.Errorf("argv[%d] = %v, want the exact-match window target %q", i, argv, target)
		}
		if !containsArg(argv, "window-resized") {
			t.Errorf("argv[%d] = %v, want the window-resized hook name", i, argv)
		}
		for _, elem := range argv {
			if elem == ";" {
				t.Errorf("argv[%d] = %v, want no bare \";\" element", i, argv)
			}
		}
	}
}

// TestResizeSignalHookCommand covers the gate deciding whether the touch entry belongs in the array
// at all: on for a watchdog: on session, off for watchdog: off, and off for an invalid value, which
// this non-fatal path treats as off rather than propagating.
func TestResizeSignalHookCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the hook is never installed on Windows; resizeSignalHookCommand answers \"\" unconditionally there")
	}

	tests := []struct {
		name           string
		watchdog       string
		wantOwnCommand bool
	}{
		{"On", "on", true},
		{"Off", "off", false},
		{"Invalid", "bogus", false},
		{"Empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestEngine(t)
			e.cfg.Watchdog = tt.watchdog

			got := e.resizeSignalHookCommand()
			want := ""
			if tt.wantOwnCommand {
				want = resizeHookCommand(shell.ForGOOS(), e.resizeSignalPath())
			}
			if got != want {
				t.Errorf("resizeSignalHookCommand() = %q; want %q for watchdog %q", got, want, tt.watchdog)
			}
		})
	}
}

// TestInstallResizePinsLocked_IssuesTheSignalEntryLast is the call-site half of the fix: the argv
// builder above is pure, so only this proves installResizePinsLocked actually hands tmux the touch
// entry — the statement whose absence left resizeHookCommand orphaned and every watcher in poll mode.
//
//testtiming:keep pins installResizePinsLocked handing tmux the touch entry last for watchdog on, none for off, and attempting every call when each errors; its covering tests run this code without asserting it
func TestInstallResizePinsLocked_IssuesTheSignalEntryLast(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the hook is never installed on Windows")
	}

	t.Run("WatchdogOn", func(t *testing.T) {
		e := newTestEngine(t)
		e.cfg.Watchdog = "on"
		fake := installFakeTmux(t, e)

		e.installResizePinsLocked(exactSessionWindowTarget(e.SessionName()), []render.Pin{{PaneID: "%1", Height: 3}})
		calls := fake.Calls()

		if len(calls) != 5 {
			t.Fatalf("installResizePinsLocked calls = %v, want 5 (clear + zoom record + 1 pin + zoom restore + signal)", calls)
		}
		last := calls[len(calls)-1]
		want := resizeHookCommand(shell.ForGOOS(), e.resizeSignalPath())
		if last[len(last)-1] != want {
			t.Errorf("last set-hook body = %q, want reed's own touch command %q", last[len(last)-1], want)
		}
	})

	t.Run("WatchdogOff", func(t *testing.T) {
		e := newTestEngine(t)
		e.cfg.Watchdog = "off"
		fake := installFakeTmux(t, e)

		e.installResizePinsLocked(exactSessionWindowTarget(e.SessionName()), []render.Pin{{PaneID: "%1", Height: 3}})
		calls := fake.Calls()

		if len(calls) != 4 {
			t.Fatalf("installResizePinsLocked calls = %v, want 4 (clear + zoom record + 1 pin + zoom restore, no signal entry)", calls)
		}
		own := resizeHookCommand(shell.ForGOOS(), e.resizeSignalPath())
		for i, argv := range calls {
			if argv[len(argv)-1] == own {
				t.Errorf("calls[%d] = %v, want no touch entry with the watchdog off", i, argv)
			}
		}
	})

	t.Run("SignalEntryFailureIsNonFatal", func(t *testing.T) {
		e := newTestEngine(t)
		e.cfg.Watchdog = "on"
		fake := installFakeTmux(t, e)
		fake.answer("set-hook", "", errors.New("boom"))

		// Every call errors; the contract is that each one is still attempted and nothing panics or
		// propagates (Shared Decision hook-failure-is-non-fatal-everywhere).
		e.installResizePinsLocked(exactSessionWindowTarget(e.SessionName()), []render.Pin{{PaneID: "%1", Height: 3}})

		if calls := fake.Calls(); len(calls) != 5 {
			t.Fatalf("installResizePinsLocked calls = %v, want all 5 attempted despite every one erroring", calls)
		}
	})
}

// TestPinsForContent pins the title-row adjustment: only the row-0 pane's pin loses a row, floored at one content row, and the hook array built from the adjusted pins carries those heights.
func TestPinsForContent(t *testing.T) {
	t.Parallel()

	pins := []render.Pin{{PaneID: "%9", Height: 1}, {PaneID: "%1", Height: 3}, {PaneID: "%2", Height: 2}}
	tests := []struct {
		name      string
		rowZero   string
		titleRow  bool
		pins      []render.Pin
		wantPins  []render.Pin
		wantBodys []string
	}{
		{
			name: "multi-row top cell loses the title row", rowZero: "%1", titleRow: true, pins: pins,
			wantPins:  []render.Pin{{PaneID: "%9", Height: 1}, {PaneID: "%1", Height: 2}, {PaneID: "%2", Height: 2}},
			wantBodys: []string{"resize-pane -t %9 -y 1", "resize-pane -t %1 -y 2", "resize-pane -t %2 -y 2"},
		},
		{
			name: "one-row top cell is floored at one content row", rowZero: "%9", titleRow: true, pins: pins,
			wantPins:  pins,
			wantBodys: []string{"resize-pane -t %9 -y 1", "resize-pane -t %1 -y 3", "resize-pane -t %2 -y 2"},
		},
		{
			name: "row-0 pane without a pin gets none", rowZero: "%5", titleRow: true, pins: pins,
			wantPins:  pins,
			wantBodys: []string{"resize-pane -t %9 -y 1", "resize-pane -t %1 -y 3", "resize-pane -t %2 -y 2"},
		},
		{
			name: "no title row passes the pins through", rowZero: "%1", titleRow: false, pins: pins,
			wantPins:  pins,
			wantBodys: []string{"resize-pane -t %9 -y 1", "resize-pane -t %1 -y 3", "resize-pane -t %2 -y 2"},
		},
		{name: "no pins", rowZero: "%1", titleRow: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			input := slices.Clone(tt.pins)
			got := pinsForContent(tt.pins, tt.rowZero, tt.titleRow)
			if !slices.Equal(got, tt.wantPins) {
				t.Errorf("pinsForContent(%v, %q, %v) = %v, want %v", tt.pins, tt.rowZero, tt.titleRow, got, tt.wantPins)
			}
			if !slices.Equal(tt.pins, input) {
				t.Errorf("pinsForContent modified its input: %v, want %v", tt.pins, input)
			}
			var bodies []string
			entries := resizePinHookArgvs("@1", got, "")[1:]
			if len(got) > 0 {
				// The first and last entries are the zoom record and restore bracket.
				entries = entries[1 : len(entries)-1]
			}
			for _, argv := range entries {
				bodies = append(bodies, argv[len(argv)-1])
			}
			if !slices.Equal(bodies, tt.wantBodys) {
				t.Errorf("hook entries from the adjusted pins = %v, want %v", bodies, tt.wantBodys)
			}
		})
	}
}
