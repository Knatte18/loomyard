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
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/shell"
)

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
			e.tmux.execHook = func(capture bool, args ...string) (string, error) {
				if args[0] == "display-message" {
					return tt.answer, tt.err
				}
				return "", nil
			}

			got, ok := e.liveBoxLocked()
			want := render.Box{X: 0, Y: 0, W: tt.wantW, H: tt.wantH}
			if got != want || ok != tt.wantOK {
				t.Errorf("liveBoxLocked() = (%+v, %v), want (%+v, %v)", got, ok, want, tt.wantOK)
			}
		})
	}
}

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

func TestReadStatusRowsLocked(t *testing.T) {
	t.Run("ScriptedAnswer", func(t *testing.T) {
		e := newTestEngine(t)
		e.tmux.execHook = func(capture bool, args ...string) (string, error) {
			if args[0] == "display-message" {
				return "on", nil
			}
			return "", nil
		}
		rows, ok := e.readStatusRowsLocked()
		if !ok || rows != 1 {
			t.Errorf("readStatusRowsLocked() = (%d, %v), want (1, true)", rows, ok)
		}
	})

	t.Run("RoundTripError", func(t *testing.T) {
		e := newTestEngine(t)
		e.tmux.execHook = func(capture bool, args ...string) (string, error) {
			return "", errors.New("boom")
		}
		rows, ok := e.readStatusRowsLocked()
		if ok || rows != 0 {
			t.Errorf("readStatusRowsLocked() = (%d, %v), want (0, false)", rows, ok)
		}
	})
}

func TestReadWindowSizeLatestLocked(t *testing.T) {
	t.Run("ScriptedAnswer", func(t *testing.T) {
		e := newTestEngine(t)
		e.tmux.execHook = func(capture bool, args ...string) (string, error) {
			if args[0] == "display-message" {
				return "latest", nil
			}
			return "", nil
		}
		if got := e.readWindowSizeLatestLocked(); !got {
			t.Errorf("readWindowSizeLatestLocked() = %v, want true", got)
		}
	})

	t.Run("RoundTripError", func(t *testing.T) {
		e := newTestEngine(t)
		e.tmux.execHook = func(capture bool, args ...string) (string, error) {
			return "", errors.New("boom")
		}
		if got := e.readWindowSizeLatestLocked(); got {
			t.Errorf("readWindowSizeLatestLocked() = %v, want false", got)
		}
	})
}

// TestPinGeometryOptionsLocked records every set-option argv the hook receives, asserting both pins
// are issued session/window-targeted (never -g), and that a first-pin error does not stop the second
// pin from being issued.
func TestPinGeometryOptionsLocked(t *testing.T) {
	tests := []struct {
		name        string
		firstErrors bool
	}{
		{"BothSucceed", false},
		{"FirstPinErrors", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestEngine(t)
			var calls [][]string
			e.tmux.execHook = func(capture bool, args ...string) (string, error) {
				if args[0] != "set-option" {
					return "", nil
				}
				calls = append(calls, append([]string{}, args...))
				if tt.firstErrors && len(calls) == 1 {
					return "", errors.New("boom")
				}
				return "", nil
			}

			e.pinGeometryOptionsLocked()

			if len(calls) != 2 {
				t.Fatalf("pinGeometryOptionsLocked issued %d set-option calls, want 2: %v", len(calls), calls)
			}

			wantTarget := exactSessionWindowTarget(e.SessionName())

			first := calls[0]
			if !containsArg(first, "-t") || !containsArg(first, wantTarget) {
				t.Errorf("first pin args = %v, want it to carry -t %q", first, wantTarget)
			}
			if containsArg(first, "-g") {
				t.Errorf("first pin args = %v, want no -g", first)
			}
			if !containsArg(first, "status") || !containsArg(first, "off") {
				t.Errorf("first pin args = %v, want the status off pair", first)
			}

			second := calls[1]
			if !containsArg(second, "-w") {
				t.Errorf("second pin args = %v, want -w", second)
			}
			if containsArg(second, "-g") {
				t.Errorf("second pin args = %v, want no -g", second)
			}
			if !containsArg(second, "window-size") || !containsArg(second, "latest") {
				t.Errorf("second pin args = %v, want the window-size latest pair", second)
			}
		})
	}
}

// TestPinGeometryOptionsLocked_HookLifecycle covers the window-resized hook install/unset lifecycle
// pinGeometryOptionsLocked now owns, alongside the two pre-existing geometry pins.
func TestPinGeometryOptionsLocked_HookLifecycle(t *testing.T) {
	t.Run("WatchdogOnPinsGeometryOptionsOnly", func(t *testing.T) {
		e := newTestEngine(t)
		e.cfg.Watchdog = "on"
		var calls [][]string
		e.tmux.execHook = func(capture bool, args ...string) (string, error) {
			calls = append(calls, append([]string{}, args...))
			return "", nil
		}

		e.pinGeometryOptionsLocked()

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
		if setOptionCalls != 2 {
			t.Errorf("pinGeometryOptionsLocked calls = %v, want 2 set-option calls (status and window-size)", calls)
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

		var calls [][]string
		e.tmux.execHook = func(capture bool, args ...string) (string, error) {
			calls = append(calls, append([]string{}, args...))
			return "", nil
		}

		e.pinGeometryOptionsLocked()

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
		e.tmux.execHook = func(capture bool, args ...string) (string, error) {
			return "", nil
		}
		// Must not panic and pinGeometryOptionsLocked returns nothing, so simply calling it and
		// returning normally is the assertion.
		e.pinGeometryOptionsLocked()
	})

	t.Run("SetHookErrorIsNonFatalWhenWatchdogOff", func(t *testing.T) {
		e := newTestEngine(t)
		e.cfg.Watchdog = "off"
		var setOptionCalls int
		var setHookErrors int
		e.tmux.execHook = func(capture bool, args ...string) (string, error) {
			if args[0] == "set-option" {
				setOptionCalls++
				return "", nil
			}
			if args[0] == "set-hook" {
				setHookErrors++
				return "", errors.New("boom")
			}
			return "", nil
		}

		e.pinGeometryOptionsLocked()

		if setOptionCalls != 2 {
			t.Errorf("set-option calls = %d, want 2 (both preceding pins still attempted despite later set-hook error)", setOptionCalls)
		}
		if setHookErrors != 1 {
			t.Errorf("set-hook errors = %d, want 1", setHookErrors)
		}
	})

	t.Run("RemovingAnAbsentSignalFileIsSilent", func(t *testing.T) {
		e := newTestEngine(t)
		e.cfg.Watchdog = "off"
		e.tmux.execHook = func(capture bool, args ...string) (string, error) {
			return "", nil
		}
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

// TestResizePinHookArgvs pins the pure argv shape resizePinHookArgvs builds for zero, one, and
// several pins with no signal entry asked for: the unconditional clear always leads, every argv
// carries -w and the exact-match window target, each body is exactly "resize-pane -t <pane> -y
// <height>", the "-a" flag appears on every entry after the first pin, and no argv anywhere in the
// sequence carries a bare ";" element.
// The signal entry's own shape is TestResizePinHookArgvs_SignalEntry's business.
func TestResizePinHookArgvs(t *testing.T) {
	const session = "myproj"
	target := exactSessionWindowTarget(session)

	assertCommon := func(t *testing.T, argvs [][]string) {
		t.Helper()
		assertResizePinHookArgvsWellFormed(t, argvs, target)
	}

	t.Run("ZeroPins", func(t *testing.T) {
		argvs := resizePinHookArgvs(session, nil, "")
		assertCommon(t, argvs)
		if len(argvs) != 1 {
			t.Fatalf("resizePinHookArgvs(zero pins, no signal hook) = %v, want exactly one argv (the clear)", argvs)
		}
		want := []string{"set-hook", "-u", "-w", "-t", target, "window-resized"}
		if len(argvs[0]) != len(want) {
			t.Fatalf("argv[0] = %v, want %v", argvs[0], want)
		}
		for i := range want {
			if argvs[0][i] != want[i] {
				t.Errorf("argv[0][%d] = %q, want %q", i, argvs[0][i], want[i])
			}
		}
	})

	t.Run("OnePin", func(t *testing.T) {
		pins := []render.Pin{{PaneID: "%1", Height: 3}}
		argvs := resizePinHookArgvs(session, pins, "")
		assertCommon(t, argvs)
		if len(argvs) != 2 {
			t.Fatalf("resizePinHookArgvs(1 pin) = %v, want 2 argvs (clear + 1)", argvs)
		}
		if containsArg(argvs[0], "-a") {
			t.Errorf("clear argv = %v, want no -a", argvs[0])
		}
		if containsArg(argvs[1], "-a") {
			t.Errorf("first-pin argv = %v, want no -a on the non-a set-hook", argvs[1])
		}
		wantBody := "resize-pane -t %1 -y 3"
		if argvs[1][len(argvs[1])-1] != wantBody {
			t.Errorf("first-pin body = %q, want %q", argvs[1][len(argvs[1])-1], wantBody)
		}
	})

	t.Run("ThreePins", func(t *testing.T) {
		pins := []render.Pin{
			{PaneID: "%1", Height: 3},
			{PaneID: "%2", Height: 2},
			{PaneID: "%3", Height: 4},
		}
		argvs := resizePinHookArgvs(session, pins, "")
		assertCommon(t, argvs)
		if len(argvs) != 4 {
			t.Fatalf("resizePinHookArgvs(3 pins) = %v, want 4 argvs (clear + 3)", argvs)
		}
		if containsArg(argvs[0], "-a") {
			t.Errorf("clear argv = %v, want no -a", argvs[0])
		}
		if containsArg(argvs[1], "-a") {
			t.Errorf("first-pin argv = %v, want no -a", argvs[1])
		}
		for i, want := range []struct {
			pane   string
			height int
		}{{"%1", 3}, {"%2", 2}, {"%3", 4}} {
			argv := argvs[i+1]
			if i > 0 && !containsArg(argv, "-a") {
				t.Errorf("argv for pin %d = %v, want -a", i, argv)
			}
			wantBody := fmt.Sprintf("resize-pane -t %s -y %d", want.pane, want.height)
			if argv[len(argv)-1] != wantBody {
				t.Errorf("argv for pin %d body = %q, want %q", i, argv[len(argv)-1], wantBody)
			}
		}
	})

	t.Run("ZeroPinsWithSignalHook", func(t *testing.T) {
		const signal = `run-shell -b ": > '/tmp/reed-resize.signal'"`
		argvs := resizePinHookArgvs(session, nil, signal)
		assertCommon(t, argvs)
		if len(argvs) != 2 {
			t.Fatalf("resizePinHookArgvs(zero pins, signal hook) = %v, want 2 argvs (clear + the signal entry)", argvs)
		}
		if containsArg(argvs[0], "-a") {
			t.Errorf("clear argv = %v, want no -a", argvs[0])
		}
		// With zero pins the signal entry is the array's first (and only) content entry, so it must
		// land plain — carrying -a here would append onto an array the clear just emptied, which is
		// harmless to tmux but would misrepresent "is this the first entry" to a reader of the argv.
		if containsArg(argvs[1], "-a") {
			t.Errorf("signal-hook argv = %v, want no -a (it is the sole entry)", argvs[1])
		}
		if argvs[1][len(argvs[1])-1] != signal {
			t.Errorf("signal-hook body = %q, want %q", argvs[1][len(argvs[1])-1], signal)
		}
	})

	t.Run("PinsWithSignalHookAppendedLast", func(t *testing.T) {
		const signal = `run-shell -b ": > '/tmp/reed-resize.signal'"`
		pins := []render.Pin{{PaneID: "%1", Height: 3}, {PaneID: "%2", Height: 2}}
		argvs := resizePinHookArgvs(session, pins, signal)
		assertCommon(t, argvs)
		if len(argvs) != 4 {
			t.Fatalf("resizePinHookArgvs(2 pins, signal hook) = %v, want 4 argvs (clear + 2 pins + signal)", argvs)
		}
		if containsArg(argvs[0], "-a") {
			t.Errorf("clear argv = %v, want no -a", argvs[0])
		}
		if containsArg(argvs[1], "-a") {
			t.Errorf("first-pin argv = %v, want no -a", argvs[1])
		}
		last := argvs[len(argvs)-1]
		if !containsArg(last, "-a") {
			t.Errorf("signal-hook argv = %v, want -a (it follows existing pins)", last)
		}
		if last[len(last)-1] != signal {
			t.Errorf("signal-hook body = %q, want %q", last[len(last)-1], signal)
		}
	})
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

// TestResizePinHookArgvs_SignalEntry pins the half of the array that had no install site at all
// before this fix: the watchdog's own run-shell touch entry, which reapply.go's hookInstalledLocked
// probes for and which nothing was ever installing.
// Every case asserts the touch is the array's LAST entry, so a resize fires the pin fixups before the
// watcher is told about it, and that the zero-pin case still installs it.
func TestResizePinHookArgvs_SignalEntry(t *testing.T) {
	const session = "myproj"
	const signalCommand = `run-shell -b "sh -c 'touch \"/tmp/wt/.lyx/reed-resize.signal\"'"`
	target := exactSessionWindowTarget(session)

	t.Run("PinsThenSignalLast", func(t *testing.T) {
		pins := []render.Pin{{PaneID: "%1", Height: 3}, {PaneID: "%2", Height: 2}}
		argvs := resizePinHookArgvs(session, pins, signalCommand)
		assertResizePinHookArgvsWellFormed(t, argvs, target)

		if len(argvs) != 4 {
			t.Fatalf("resizePinHookArgvs(2 pins + signal) = %v, want 4 argvs (clear + 2 pins + signal)", argvs)
		}
		last := argvs[len(argvs)-1]
		if last[len(last)-1] != signalCommand {
			t.Errorf("last argv body = %q, want the signal command %q — the touch must be the array's last entry", last[len(last)-1], signalCommand)
		}
		if !containsArg(last, "-a") {
			t.Errorf("signal argv = %v, want -a (it appends onto the pins already at index 0 and 1)", last)
		}
		for i, argv := range argvs[1:3] {
			if !strings.HasPrefix(argv[len(argv)-1], "resize-pane ") {
				t.Errorf("argv for pin %d body = %q, want a resize-pane body ahead of the signal entry", i, argv[len(argv)-1])
			}
		}
	})

	t.Run("ZeroPinsStillInstallsTheSignalEntry", func(t *testing.T) {
		argvs := resizePinHookArgvs(session, nil, signalCommand)
		assertResizePinHookArgvsWellFormed(t, argvs, target)

		// "Nothing is pinned" and "nobody wants to hear about a resize" are different opinions: a
		// session with no fixed-height pane still needs its watcher told a resize happened, or the
		// probe can never promote it out of poll mode.
		if len(argvs) != 2 {
			t.Fatalf("resizePinHookArgvs(zero pins + signal) = %v, want 2 argvs (clear + signal)", argvs)
		}
		signal := argvs[1]
		if containsArg(signal, "-a") {
			t.Errorf("signal argv = %v, want no -a — with no pins ahead of it, it is the entry that establishes the array at index 0", signal)
		}
		if signal[len(signal)-1] != signalCommand {
			t.Errorf("signal argv body = %q, want %q", signal[len(signal)-1], signalCommand)
		}
	})

	t.Run("EmptySignalCommandEmitsNoEntry", func(t *testing.T) {
		pins := []render.Pin{{PaneID: "%1", Height: 3}}
		argvs := resizePinHookArgvs(session, pins, "")
		assertResizePinHookArgvsWellFormed(t, argvs, target)

		if len(argvs) != 2 {
			t.Fatalf("resizePinHookArgvs(1 pin, no signal) = %v, want 2 argvs (clear + 1 pin)", argvs)
		}
		for i, argv := range argvs {
			if argv[len(argv)-1] == "" {
				t.Errorf("argv[%d] = %v, want no empty body element", i, argv)
			}
		}
	})
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
func TestInstallResizePinsLocked_IssuesTheSignalEntryLast(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the hook is never installed on Windows")
	}

	t.Run("WatchdogOn", func(t *testing.T) {
		e := newTestEngine(t)
		e.cfg.Watchdog = "on"
		var calls [][]string
		e.tmux.execHook = func(capture bool, args ...string) (string, error) {
			calls = append(calls, append([]string{}, args...))
			return "", nil
		}

		e.installResizePinsLocked([]render.Pin{{PaneID: "%1", Height: 3}})

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
		var calls [][]string
		e.tmux.execHook = func(capture bool, args ...string) (string, error) {
			calls = append(calls, append([]string{}, args...))
			return "", nil
		}

		e.installResizePinsLocked([]render.Pin{{PaneID: "%1", Height: 3}})

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
		var calls [][]string
		e.tmux.execHook = func(capture bool, args ...string) (string, error) {
			calls = append(calls, append([]string{}, args...))
			return "", errors.New("boom")
		}

		// Every call errors; the contract is that each one is still attempted and nothing panics or
		// propagates (Shared Decision hook-failure-is-non-fatal-everywhere).
		e.installResizePinsLocked([]render.Pin{{PaneID: "%1", Height: 3}})

		if len(calls) != 3 {
			t.Fatalf("installResizePinsLocked calls = %v, want all 3 attempted despite every one erroring", calls)
		}
	})
}
