// windowsize_test.go covers windowsize.go's geometry pins and the window-resized unpin.
// Every case is driven through TmuxCmd's execHook seam (no live server, no external process spawn, no sleep), the shape generation_test.go and strand_test.go already use.

package reedengine

import (
	"errors"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

// resizedUnpinCount is how many window-resized unpins one pinGeometryOptionsLocked run issues: two wherever set-hook exists, none on Windows.
func resizedUnpinCount() int {
	if runtime.GOOS == "windows" {
		return 0
	}
	return 2
}

// bindKeyArgvCount is how many bind-key calls one pinBindingsLocked run issues with no session-switch command and no follow jump: every binding argv but the Alt+z unbind.
func bindKeyArgvCount() int {
	count := 0
	for _, argv := range bindingArgvs("", "", false) {
		if argv[0] == "bind-key" {
			count++
		}
	}
	return count
}

// wantResizedUnpins is the window-scoped then the session-scoped unpin of the window-resized hook on target.
func wantResizedUnpins(target string) [][]string {
	return [][]string{
		{"set-hook", "-u", "-w", "-t", target, windowResizedHookName},
		{"set-hook", "-u", "-t", target, windowResizedHookName},
	}
}

// TestPinGeometryOptionsLocked drives pinGeometryOptionsLocked against the fake tmux, recording every set-option argv issued.
// It asserts the two-line bar's pins plus the window-size pin are all issued with the expected target and value:
// both status formats global, status 2 and status-position on the session, and no pane-border pin.
// It asserts that no call's failure stops the calls after it,
// and that the window-resized hook is unpinned in both scopes, whatever the watchdog value, and never installed.
//
//testtiming:keep pins the set-option calls issued to pin the geometry: the two global status formats, status 2 and the position on the session, window-size and no pane-border pin, no failure stopping the calls after it, and the window-resized unpin in both scopes on every watchdog value; its covering tests run this code without asserting it
func TestPinGeometryOptionsLocked(t *testing.T) {
	t.Run("AllOptionsIssued", func(t *testing.T) {
		e := newTestEngine(t)
		fake := installFakeTmux(t, e)

		target := exactSessionWindowTarget(e.SessionName())
		e.pinGeometryOptionsLocked(target, "")
		calls := fake.ArgvFor("set-option")

		wantOptions := [][]string{
			{"set-option", "-g", "status-format[0]", statusFormatButtons},
			{"set-option", "-g", "status-format[1]", statusFormatSessions},
			{"set-option", "-g", "status-style", statusBarStyle},
			{"set-option", "-t", target, "status", "2"},
			{"set-option", "-t", target, "status-position", "bottom"},
			{"set-option", "-w", "-t", target, "window-size", "latest"},
			// The fixture's tmux binary does not resolve, so the follow jump line is unset.
			{"set-option", "-gu", "@lyx_follow_command"},
			// The test binary carries no VCS stamp, so the bindings' build stamp is unset.
			{"set-option", "-gu", "@lyx_bindings_build"},
		}
		if !slices.EqualFunc(calls, wantOptions, slices.Equal[[]string]) {
			t.Errorf("pinGeometryOptionsLocked set-option calls = %v, want %v", calls, wantOptions)
		}
	})

	t.Run("ShellPinsFollowTheOptionPins", func(t *testing.T) {
		e := newTestEngine(t)
		fake := installFakeTmux(t, e)
		target := exactSessionWindowTarget(e.SessionName())

		e.pinGeometryOptionsLocked(target, "/bin/sh")
		calls := fake.ArgvFor("set-option")

		wantTail := [][]string{
			{"set-option", "-w", "-t", target, "window-size", "latest"},
			{"set-option", "-t", target, "default-shell", "/bin/sh"},
			{"set-option", "-t", target, "default-command", "/bin/sh"},
		}
		wantLast := [][]string{
			{"set-option", "-gu", "@lyx_follow_command"},
			{"set-option", "-gu", "@lyx_bindings_build"},
		}
		if len(calls) < len(wantTail)+len(wantLast) {
			t.Fatalf("pinGeometryOptionsLocked set-option calls = %v, want at least the window-size pin, the two shell pins, the follow-command unset and the build-stamp unset", calls)
		}
		if got := calls[len(calls)-len(wantLast):]; !slices.EqualFunc(got, wantLast, slices.Equal[[]string]) {
			t.Errorf("last set-option calls = %v, want the follow-command and build-stamp unsets %v", got, wantLast)
		}
		if got := calls[len(calls)-len(wantLast)-len(wantTail) : len(calls)-len(wantLast)]; !slices.EqualFunc(got, wantTail, slices.Equal[[]string]) {
			t.Errorf("pinGeometryOptionsLocked set-option calls = %v, want them to end, before the follow-command and build-stamp unsets, with the window-size pin and the two session-scoped shell pins %v", calls, wantTail)
		}

		fake = installFakeTmux(t, e)
		e.pinGeometryOptionsLocked(target, "")
		for _, call := range fake.ArgvFor("set-option") {
			if slices.Contains(call, "default-shell") || slices.Contains(call, "default-command") {
				t.Errorf("pinGeometryOptionsLocked with an empty shell path issued %v, want no shell pin", call)
			}
		}
	})

	t.Run("BindingsAreIssuedAfterTheOptionPins", func(t *testing.T) {
		e := newTestEngine(t)
		fake := installFakeTmux(t, e)

		e.pinGeometryOptionsLocked(exactSessionWindowTarget(e.SessionName()), "")

		lastOption, firstBinding, followUnset, bindings := -1, -1, -1, 0
		var click []string
		for i, call := range fake.Calls() {
			switch call[0] {
			case "set-option":
				lastOption = i
				if slices.Equal(call, []string{"set-option", "-gu", "@lyx_follow_command"}) {
					followUnset = i
				}
			case "bind-key":
				bindings++
				if firstBinding == -1 {
					firstBinding = i
				}
				if call[2] == "MouseDown1Status" {
					click = call
				}
			}
		}
		// The fixture's tmux binary does not exist, so the two session-switch keys are left unbound and the follow jump is not pinned.
		if want := bindKeyArgvCount(); bindings != want {
			t.Errorf("pinGeometryOptionsLocked issued %d bind-key calls, want %d", bindings, want)
		}
		if got := fake.Count("unbind-key"); got != 1 {
			t.Errorf("pinGeometryOptionsLocked issued %d unbind-key calls, want 1 for the retired Alt+z", got)
		}
		if firstBinding < lastOption {
			t.Errorf("first bind-key call at %d precedes the last set-option call at %d, want the bindings after the option pins", firstBinding, lastOption)
		}
		if followUnset == -1 || followUnset > firstBinding {
			t.Errorf("follow-command unset at %d, first bind-key call at %d, want the unset issued before any binding", followUnset, firstBinding)
		}
		// The follow branch lives inside the one click argv, so only the argv itself shows that no jump is bound.
		var wantClick []string
		for _, argv := range bindingArgvs("", "", false) {
			if argv[2] == "MouseDown1Status" {
				wantClick = argv
			}
		}
		if !slices.Equal(click, wantClick) {
			t.Errorf("MouseDown1Status binding = %q, want the one without a follow jump %q", click, wantClick)
		}
	})

	t.Run("FailedFollowCommandWriteIsNonFatal", func(t *testing.T) {
		e := newTestEngine(t)
		fake := installFakeTmux(t, e)
		fake.answerFunc("set-option", func(args []string) (string, error) {
			if slices.Contains(args, followCommandOption) {
				return "", errors.New("boom")
			}
			return "", nil
		})
		logs := logcapture.CaptureVerbose(t)

		e.pinGeometryOptionsLocked(exactSessionWindowTarget(e.SessionName()), "")

		if got, want := fake.Count("bind-key"), bindKeyArgvCount(); got != want {
			t.Errorf("bind-key calls = %d after a failed follow-command write, want all %d still issued", got, want)
		}
		if !strings.Contains(logs.String(), "failed to unset the follow jump line") {
			t.Errorf("log = %q, want the failed follow-command write logged at Warn", logs.String())
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

		e.pinGeometryOptionsLocked(exactSessionWindowTarget(e.SessionName()), "")
		calls := fake.ArgvFor("set-option")

		const wantCalls = 8
		if len(calls) != wantCalls {
			t.Fatalf("pinGeometryOptionsLocked issued %d set-option calls despite one erroring, want all %d still attempted: %v", len(calls), wantCalls, calls)
		}
	})

	t.Run("UnpinsWindowResizedInBothScopesOnEveryWatchdogValue", func(t *testing.T) {
		for _, watchdog := range []string{"on", "off", "bogus"} {
			e := newTestEngine(t)
			e.cfg.Watchdog = watchdog
			fake := installFakeTmux(t, e)
			target := exactSessionWindowTarget(e.SessionName())

			e.pinGeometryOptionsLocked(target, "")

			var want [][]string
			if resizedUnpinCount() > 0 {
				want = wantResizedUnpins(target)
			}
			if got := fake.SetHookCalls(windowResizedHookName); !slices.EqualFunc(got, want, slices.Equal[[]string]) {
				t.Errorf("watchdog %q: set-hook calls naming %s = %v, want exactly the two unpins %v", watchdog, windowResizedHookName, got, want)
			}
			assertUnseenClearHookPinned(t, e, fake)
		}
	})

	t.Run("FailedUnpinsAreNonFatal", func(t *testing.T) {
		e := newTestEngine(t)
		fake := installFakeTmux(t, e)
		fake.answer("set-hook", "", errors.New("boom"))

		e.pinGeometryOptionsLocked(exactSessionWindowTarget(e.SessionName()), "")

		if got := fake.Count("set-option"); got != 8 {
			t.Errorf("set-option calls = %d, want 8 (every pin still attempted)", got)
		}
		if got, want := fake.Count("bind-key"), bindKeyArgvCount(); got != want {
			t.Errorf("bind-key calls = %d, want all %d still issued", got, want)
		}
		// The unseen-clear pin and both unpins are attempted while each errors, the session-scoped unpin after the failed window-scoped one.
		if got, want := fake.Count("set-hook"), unseenClearPinCount()+resizedUnpinCount(); got != want {
			t.Errorf("set-hook calls = %d, want %d", got, want)
		}
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
