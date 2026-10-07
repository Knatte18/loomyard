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
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/shell"
	"github.com/Knatte18/loomyard/internal/tokenvocab"
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

			got, ok := e.liveBoxLocked()
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

// TestEscapeStatusText covers the pure doubling rule: every "#" becomes "##", regardless of position.
//
//testtiming:keep pins the doubling rule: every "#" becomes "##" at any position; its covering tests run this code without asserting it
func TestEscapeStatusText(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"NoHash", "plain text", "plain text"},
		{"OneHash", "a#b", "a##b"},
		{"SeveralHashes", "#a#b#c", "##a##b##c"},
		{"HashAtEachEnd", "#middle#", "##middle##"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := escapeStatusText(tt.in); got != tt.want {
				t.Errorf("escapeStatusText(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestStatusLeftLength covers the rune-counted floor-at-10 rule, including a multi-byte string whose
// rune count differs materially from its byte count — statusLeftLength must report the rune count, not
// the byte count.
//
//testtiming:keep pins the rune-counted floor-at-10 status-left length, a multi-byte string counted by runes not bytes, and escaping before measuring yielding a larger length; its covering tests run this code without asserting it
func TestStatusLeftLength(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want int
	}{
		{"ShorterThanFloor", "short", 10},
		{"ExactlyTen", "1234567890", 10},
		{"LongerThanFloor", "this is a long status line", 26},
		// 12 runes, 36 bytes (3 bytes per hiragana character): a byte-counting implementation would
		// wrongly answer 36 here.
		{"MultiByteRuneCountDiffersFromByteCount", "あいうえおかきくけこさし", 12},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := statusLeftLength(tt.in); got != tt.want {
				t.Errorf("statusLeftLength(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}

	// Measuring a string before escapeStatusText doubles its "#" characters can under-report the length tmux will actually receive:
	// "######" is 6 runes unescaped (floored to 10), but "############" once escaped is 12 runes, over the floor, so escaping first must yield a strictly larger answer.
	const s = "######"
	before := statusLeftLength(s)
	after := statusLeftLength(escapeStatusText(s))
	if after <= before {
		t.Errorf("statusLeftLength(escapeStatusText(%q)) = %d, want it to exceed statusLeftLength(%q) = %d — measuring the pre-escape string would truncate a hub path containing '#'", s, after, s, before)
	}
}

// TestReadbacksLocked pins the two single-value tmux readbacks: the status row count (on reserves one row) and the window-size latest check,
// each answering from a scripted display-message and degrading on a round-trip error.
//
//testtiming:keep pins the status-row and window-size-latest readbacks answering from a scripted display-message and degrading on a round-trip error; its covering tests run this code without asserting it
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
				rows, ok := e.readStatusRowsLocked()
				return fmt.Sprintf("(%d, %v)", rows, ok)
			},
			want: "(1, true)",
		},
		{
			name: "StatusRowsRoundTripError",
			err:  errors.New("boom"),
			read: func(e *Engine) string {
				rows, ok := e.readStatusRowsLocked()
				return fmt.Sprintf("(%d, %v)", rows, ok)
			},
			want: "(0, false)",
		},
		{
			name:   "WindowSizeLatestScriptedAnswer",
			answer: "latest",
			read:   func(e *Engine) string { return strconv.FormatBool(e.readWindowSizeLatestLocked()) },
			want:   "true",
		},
		{
			name: "WindowSizeLatestRoundTripError",
			err:  errors.New("boom"),
			read: func(e *Engine) string { return strconv.FormatBool(e.readWindowSizeLatestLocked()) },
			want: "false",
		},
	}
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

// TestPinGeometryOptionsLocked drives pinGeometryOptionsLocked against the fake tmux,
// recording every set-option argv issued. It asserts the seven status-line options plus the
// pre-existing window-size pin are all issued with the expected target/value, that status-left carries
// the escaped rendered text, that a StatusLineText render error skips only status-left and
// status-left-length while the other six calls (five status-line options plus window-size) still
// happen, and that no call's failure stops the calls after it.
//
//testtiming:keep pins the set-option calls issued to pin the geometry: the seven status-line options plus window-size with the escaped rendered text, only status-left and status-left-length skipped when the render errors, no failure stopping the calls after it, and the window-resized hook lifecycle left to unset and clean the signal file when the watchdog is off; its covering tests run this code without asserting it
func TestPinGeometryOptionsLocked(t *testing.T) {
	t.Run("AllOptionsIssuedWithEscapedText", func(t *testing.T) {
		e := newTestEngine(t)
		// newTestEngine's Geometry leaves WorktreeName unset; the default status-line template's
		// {{.worktree}} marker requires it, so this case sets it so StatusLineText() succeeds.
		e.geom.WorktreeName = "test-worktree"
		// A "#" in the hub path proves the identity text is escaped while the waits segment stays raw.
		e.geom.HubPath = "/hub/a#b"
		fake := installFakeTmux(t, e)

		wantText, err := e.StatusLineText()
		if err != nil {
			t.Fatalf("StatusLineText() unexpected error: %v", err)
		}
		wantEscaped := escapeStatusText(strings.TrimRight(wantText, "\r\n"))
		if !strings.Contains(wantEscaped, "a##b") {
			t.Fatalf("escaped status text = %q; want the hub path's # doubled", wantEscaped)
		}
		wantStatusLeft := strings.ReplaceAll(wantEscaped, tokenvocab.WaitsPlaceholder, waitsSegmentFormat)
		wantLength := statusLeftLength(strings.ReplaceAll(wantEscaped, tokenvocab.WaitsPlaceholder, "")) + waitsSegmentLengthAllowance

		e.pinGeometryOptionsLocked()
		calls := fake.ArgvFor("set-option")

		target := exactSessionWindowTarget(e.SessionName())
		wantOptions := [][]string{
			{"set-option", "-t", target, "status", "on"},
			{"set-option", "-t", target, "status-position", "bottom"},
			{"set-option", "-t", target, "status-left", wantStatusLeft},
			{"set-option", "-t", target, "status-right", ""},
			{"set-option", "-t", target, "status-left-length", strconv.Itoa(wantLength)},
			{"set-option", "-w", "-t", target, "window-status-format", ""},
			{"set-option", "-w", "-t", target, "window-status-current-format", ""},
			{"set-option", "-w", "-t", target, "window-size", "latest"},
		}

		if len(calls) != len(wantOptions) {
			t.Fatalf("pinGeometryOptionsLocked issued %d set-option calls, want %d: %v", len(calls), len(wantOptions), calls)
		}
		for i, want := range wantOptions {
			if len(calls[i]) != len(want) {
				t.Fatalf("call[%d] = %v, want %v", i, calls[i], want)
			}
			for j := range want {
				if calls[i][j] != want[j] {
					t.Errorf("call[%d][%d] = %q, want %q (full call %v, want %v)", i, j, calls[i][j], want[j], calls[i], want)
				}
			}
		}
	})

	t.Run("StatusLineTextErrorSkipsOnlyTheTwoTextDerivedOptions", func(t *testing.T) {
		e := newTestEngine(t)
		// An unknown top-level token forces StatusLineText() to error, the same shape
		// TestValidateStatusLine_UnknownTopLevelTokenErrors pins.
		e.cfg.StatusLine.Template = "{{.slug}}"
		fake := installFakeTmux(t, e)

		e.pinGeometryOptionsLocked()
		calls := fake.ArgvFor("set-option")

		for _, c := range calls {
			if containsArg(c, "status-left") || containsArg(c, "status-left-length") {
				t.Errorf("calls = %v, want no status-left or status-left-length call when StatusLineText errors", calls)
			}
		}
		const wantCalls = 6 // status, status-position, status-right, window-status-format, window-status-current-format, window-size
		if len(calls) != wantCalls {
			t.Fatalf("pinGeometryOptionsLocked issued %d set-option calls on a StatusLineText error, want %d: %v", len(calls), wantCalls, calls)
		}
	})

	t.Run("OneOptionFailureDoesNotStopTheRest", func(t *testing.T) {
		e := newTestEngine(t)
		e.geom.WorktreeName = "test-worktree"
		fake := installFakeTmux(t, e)
		fake.answerFunc("set-option", func([]string) (string, error) {
			if fake.Count("set-option") == 1 {
				return "", errors.New("boom")
			}
			return "", nil
		})

		e.pinGeometryOptionsLocked()
		calls := fake.ArgvFor("set-option")

		const wantCalls = 8
		if len(calls) != wantCalls {
			t.Fatalf("pinGeometryOptionsLocked issued %d set-option calls despite one erroring, want all %d still attempted: %v", len(calls), wantCalls, calls)
		}
	})
	t.Run("WatchdogOnPinsGeometryOptionsOnly", func(t *testing.T) {
		e := newTestEngine(t)
		e.geom.WorktreeName = "test-worktree"
		e.cfg.Watchdog = "on"
		fake := installFakeTmux(t, e)

		e.pinGeometryOptionsLocked()
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
			t.Errorf("pinGeometryOptionsLocked calls = %v, want 8 set-option calls (the seven status-line options and window-size)", calls)
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

		e.pinGeometryOptionsLocked()
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
		e.pinGeometryOptionsLocked()
	})

	t.Run("SetHookErrorIsNonFatalWhenWatchdogOff", func(t *testing.T) {
		e := newTestEngine(t)
		e.geom.WorktreeName = "test-worktree"
		e.cfg.Watchdog = "off"
		fake := installFakeTmux(t, e)
		fake.answer("set-hook", "", errors.New("boom"))

		e.pinGeometryOptionsLocked()

		if setOptionCalls := fake.Count("set-option"); setOptionCalls != 8 {
			t.Errorf("set-option calls = %d, want 8 (all preceding pins still attempted despite the later set-hook error)", setOptionCalls)
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
		e.pinGeometryOptionsLocked()
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
// the unconditional clear always leads (exactly "set-hook -u -w -t <target> window-resized"),
// then one entry per pin whose body is exactly "resize-pane -t <pane> -y <height>", then the watchdog's own touch entry last when a signal command is told,
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
			wantBodies: []string{"resize-pane -t %1 -y 3"},
		},
		{
			name:       "ThreePins",
			pins:       []render.Pin{{PaneID: "%1", Height: 3}, {PaneID: "%2", Height: 2}, {PaneID: "%3", Height: 4}},
			wantBodies: []string{"resize-pane -t %1 -y 3", "resize-pane -t %2 -y 2", "resize-pane -t %3 -y 4"},
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
			wantBodies: []string{"resize-pane -t %1 -y 3", "resize-pane -t %2 -y 2", signalCommand},
		},
		{
			name:       "EmptySignalCommandEmitsNoEntry",
			pins:       []render.Pin{{PaneID: "%1", Height: 3}},
			wantBodies: []string{"resize-pane -t %1 -y 3"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			argvs := resizePinHookArgvs(session, tt.pins, tt.signal)

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

		e.installResizePinsLocked([]render.Pin{{PaneID: "%1", Height: 3}})
		calls := fake.Calls()

		if len(calls) != 3 {
			t.Fatalf("installResizePinsLocked calls = %v, want 3 (clear + 1 pin + signal)", calls)
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

		e.installResizePinsLocked([]render.Pin{{PaneID: "%1", Height: 3}})
		calls := fake.Calls()

		if len(calls) != 2 {
			t.Fatalf("installResizePinsLocked calls = %v, want 2 (clear + 1 pin, no signal entry)", calls)
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
		e.installResizePinsLocked([]render.Pin{{PaneID: "%1", Height: 3}})

		if calls := fake.Calls(); len(calls) != 3 {
			t.Fatalf("installResizePinsLocked calls = %v, want all 3 attempted despite every one erroring", calls)
		}
	})
}
