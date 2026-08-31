//go:build smoke

// smoke_dotfill_measure_test.go is the measurement gate for the repaint candidates named in reed's
// repaint-mechanism decision: it installs each candidate's hook body into reed's own window-resized
// array directly from this file, as a literal string, rather than from any production repaint code —
// no such code exists yet, and the whole point of this file is to measure a body before one is
// written. A candidate is accepted only when it clears the dot-fill artifact this package's control
// scenarios reproduce AND satisfies both criteria in the repaint-must-not-self-retrigger decision: no
// repeated hook fire, no resize storm.

package reedcli

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shell"
)

// tmuxHookQuote mirrors reedengine's unexported tmuxQuoteValue: it wraps s in tmux double quotes,
// backslash-escaping \, ", and $ first. tmux parses a hook's value as a tmux command line with its
// own word splitting, so a shell fragment inside it has to survive as one tmux word; $ is escaped
// because tmux expands it inside double quotes.
func tmuxHookQuote(s string) string {
	escaped := strings.NewReplacer(
		`\`, `\\`,
		`"`, `\"`,
		`$`, `\$`,
	).Replace(s)
	return `"` + escaped + `"`
}

// fireCounterEntry returns a window-resized array entry body that appends one line to countPath
// every time the hook fires.
// It carries "-b" — without it the tmux server blocks while the command runs — and it is the
// cheapest available instrument: it is appended to the array for the duration of one measurement
// only, and it is never part of any shipped array.
func fireCounterEntry(t *testing.T, countPath string) string {
	t.Helper()
	fragment := "echo fire >> " + shell.Posix().Quote(countPath)
	return "run-shell -b " + tmuxHookQuote(fragment)
}

// fireCount reads countPath and returns its line count, treating an absent file as zero.
func fireCount(t *testing.T, countPath string) int {
	t.Helper()
	data, err := os.ReadFile(countPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return 0
		}
		t.Fatalf("read fire count file %s: %v", countPath, err)
	}
	trimmed := strings.TrimRight(string(data), "\n")
	if trimmed == "" {
		return 0
	}
	return len(strings.Split(trimmed, "\n"))
}

// sampleWindowSize polls `display-message -p -t "=<session>:" "#{window_width} #{window_height}"`
// every 100 ms for the whole of window and returns every answer in order.
func sampleWindowSize(t *testing.T, tmuxPath, socket, session string, window time.Duration) []string {
	t.Helper()
	target := "=" + session + ":"
	deadline := time.Now().Add(window)
	var samples []string
	for time.Now().Before(deadline) {
		out, err := exec.Command(tmuxPath, "-L", socket, "display-message", "-p", "-t", target, "#{window_width} #{window_height}").Output()
		if err != nil {
			t.Fatalf("display-message window size: %v", err)
		}
		samples = append(samples, strings.TrimSpace(string(out)))
		time.Sleep(100 * time.Millisecond)
	}
	return samples
}

// assertWindowSizeSettles fails unless the final third of samples is a single repeated value.
//
// This is the no resize storm criterion: the window's size must be observably stable after the
// trigger settles, rather than oscillating between two clients' sizes.
func assertWindowSizeSettles(t *testing.T, samples []string) {
	t.Helper()
	if len(samples) == 0 {
		t.Fatalf("no window-size samples collected")
	}
	n := len(samples) / 3
	if n == 0 {
		n = len(samples)
	}
	tail := samples[len(samples)-n:]
	want := tail[0]
	for _, got := range tail {
		if got != want {
			t.Fatalf("window size did not settle within the measurement window — this is the no-resize-storm criterion: final third of samples = %v, want a single repeated value", tail)
		}
	}
}

// assertSingleHookFire fails unless got is exactly 1.
//
// This is the no repeated hook fire criterion: one settled resize must yield the documented single
// fire, not a growing series. A growing series is the resize-storm feedback path where a
// server-issued repaint would move the most-recently-used client pointer that window-size latest
// keys on.
func assertSingleHookFire(t *testing.T, got int) {
	t.Helper()
	if got != 1 {
		t.Fatalf("hook fired %d times; want exactly 1 — this is the no-repeated-hook-fire criterion", got)
	}
}

// candidateOneBody returns candidate 1's window-resized array-entry body, from the
// repaint-mechanism decision: a run-shell -b invocation that enumerates the session's clients and
// refreshes each one, so it reaches clients other than the one whose resize fired the hook.
//
// The multiplexer binary path and reed's socket are POSIX-single-quoted in both tmux invocations —
// the tmux server's run-shell inherits no reed context, so neither can be omitted. list-clients uses
// the bare "=<session>" session target (exactSessionTarget's shape, never
// exactSessionWindowTarget's trailing-colon window form): list-clients -t takes a session target,
// and the "=" prefix is what stops tmux prefix-matching a sibling worktree's session on the shared
// per-hub server.
//
// Known hazard this body's format string settles empirically (see card 8's brief): tmux performs
// format expansion on a run-shell argument, so a literal "#{client_name}" may be expanded by tmux
// itself before the shell ever sees it, collapsing the enumeration to the hook's own client or to an
// empty string. The doubled form below ("##{client_name}", which tmux reduces to a literal "#{") is
// the documented escape and is used here; whichever form the measurement actually proves working is
// the form card 11 must transcribe into the measurement record, not this comment's prose.
func candidateOneBody(tmuxPath, socket, session string) string {
	sh := shell.Posix()
	tmuxQ := sh.Quote(tmuxPath)
	socketQ := sh.Quote(socket)
	sessionTargetQ := sh.Quote("=" + session)
	fragment := tmuxQ + " -L " + socketQ + " list-clients -t " + sessionTargetQ + " -F '##{client_name}'" +
		" | while IFS= read -r line; do " + tmuxQ + " -L " + socketQ + ` refresh-client -t "$line"; done`
	return "run-shell -b " + tmuxHookQuote(fragment)
}

// candidateTwoBody returns candidate 2's window-resized array-entry body, from the
// repaint-mechanism decision: the literal tmux command "refresh-client", with no target.
//
// This is a tmux command, not a shell fragment: it carries no run-shell, no -b, no tmuxHookQuote
// wrapping, and no shell involvement at all — forcing it through candidateOneBody's machinery would
// be wrong by construction. With no target it reaches only the hook's own client, which is why it is
// measured second.
func candidateTwoBody() string {
	return "refresh-client"
}

// TestSmokeRepaintCandidateMeasurement is the measurement-gate scenario: one subtest per candidate,
// named Candidate1 and Candidate2, driven by measureRepaintCandidate so both are measured
// identically.
//
// Neither subtest asserts a pass/fail verdict for the candidate itself — a negative reading (the
// candidate did not clear the artifact, or it cleared it but tripped a criterion) is a valid,
// recordable result, never a t.Fatal. Both subtests fail only on a harness fault: the setup failing,
// the readback not matching what was installed, or the trigger not firing at all.
func TestSmokeRepaintCandidateMeasurement(t *testing.T) {
	t.Run("Candidate1", func(t *testing.T) {
		measureRepaintCandidate(t, "candidate1", func(h *dotFillHarness) string {
			return candidateOneBody(h.tmuxPath, h.reedSocket, h.reedSession)
		})
	})
	t.Run("Candidate2", func(t *testing.T) {
		measureRepaintCandidate(t, "candidate2", func(*dotFillHarness) string {
			return candidateTwoBody()
		})
	})
}

// measureRepaintCandidate runs one candidate's measurement: boot the same harness the resize control
// uses, install candidateBodyFor(h)'s body into reed's window-resized array (after the pins, before
// the fire counter), fire the resize trigger exactly as TestSmokeDotFillResizeControl does, and log
// the REPAINT-MEASUREMENT line card 11 transcribes into the measurement record.
func measureRepaintCandidate(t *testing.T, name string, candidateBodyFor func(*dotFillHarness) string) {
	h := newDotFillHarness(t, 140, 42)
	paneID := harnessOnlyPaneID(t, h.tmuxPath, h.harnessSocket, "h")
	h.attachIn(t, paneID, "DOTFILL-MARKER-ALPHA")

	countPath := filepath.Join(t.TempDir(), "fires")

	// Last setup step, after the attach: rewrite reed's own array as every pin, then the candidate
	// body, then the counting entry — the position the repaint-mechanism decision specifies for the
	// shipped entry relative to the pins.
	pins := pinOnlyEntries(windowResizedEntries(t, h.tmuxPath, h.reedSocket, h.reedSession))
	candidateBody := candidateBodyFor(h)
	entries := append(append([]string{}, pins...), candidateBody, fireCounterEntry(t, countPath))
	rewriteWindowResizedArray(t, h.tmuxPath, h.reedSocket, h.reedSession, entries)

	readBack := windowResizedEntries(t, h.tmuxPath, h.reedSocket, h.reedSession)
	assertCandidateInstalled(t, readBack, pins, candidateBody)

	// Fire the trigger exactly as TestSmokeDotFillResizeControl does: resize-window on reed's own
	// socket and session, shrink then grow.
	if err := exec.Command(h.tmuxPath, "-L", h.reedSocket, "resize-window", "-t", h.reedSession, "-x", "80", "-y", "24").Run(); err != nil {
		t.Fatalf("resize-window shrink: %v", err)
	}
	if err := exec.Command(h.tmuxPath, "-L", h.reedSocket, "resize-window", "-t", h.reedSession, "-x", "160", "-y", "50").Run(); err != nil {
		t.Fatalf("resize-window grow: %v", err)
	}

	// Record three readings rather than asserting a pass/fail verdict for the whole subtest.
	cleared := paneStaysCleanOfDotRun(t, h.tmuxPath, h.harnessSocket, paneID, 3*time.Second)
	fires := fireCount(t, countPath)
	samples := sampleWindowSize(t, h.tmuxPath, h.reedSocket, h.reedSession, 3*time.Second)

	singleFireOK := isolatedCriterionResult(func(t *testing.T) { assertSingleHookFire(t, fires) })
	noStormOK := isolatedCriterionResult(func(t *testing.T) { assertWindowSizeSettles(t, samples) })

	t.Logf("REPAINT-MEASUREMENT candidate=%s tmux=%s cleared=%v single_fire_ok=%v(fires=%d) no_storm_ok=%v(samples=%v) tmux_path=%s socket=%s session=%s body=%q",
		name, tmuxVersionString(t, h.tmuxPath), cleared, singleFireOK, fires, noStormOK, samples, h.tmuxPath, h.reedSocket, h.reedSession, candidateBody)
}

// isolatedCriterionResult runs check against a throwaway *testing.T with no parent, in its own
// goroutine, and reports whether it passed.
//
// A throwaway T is required here: assertSingleHookFire and assertWindowSizeSettles are Fatal-style,
// and testing.common.Fail propagates to every ancestor T, so calling them against the scenario's own
// *testing.T would fail the whole Candidate1/Candidate2 subtest merely because a candidate tripped a
// criterion — exactly the outcome this file's doc comment and this function's callers must avoid. The
// check runs in its own goroutine because a Fatal call's runtime.Goexit unwinds only the calling
// goroutine; running it inline would abort measureRepaintCandidate itself before it could log the
// reading.
func isolatedCriterionResult(check func(t *testing.T)) bool {
	scratch := &testing.T{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		check(scratch)
	}()
	<-done
	return !scratch.Failed()
}

// tmuxVersionString returns the tmux binary's `-V` output, trimmed. A failure here is a harness
// fault, not a candidate reading, so it fails the test.
func tmuxVersionString(t *testing.T, tmuxPath string) string {
	t.Helper()
	out, err := exec.Command(tmuxPath, "-V").Output()
	if err != nil {
		t.Fatalf("tmux -V: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// assertCandidateInstalled fails the test unless entries contains, verbatim, every pin in pins and
// candidateBody itself — the mirror of the control's assertOnlyPinEntries. Finding the body here also
// proves it round-trips byte-identically through show-options -v, which hookInstalledLocked's
// per-entry matching depends on.
func assertCandidateInstalled(t *testing.T, entries, pins []string, candidateBody string) {
	t.Helper()
	nonEmpty := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry != "" {
			nonEmpty = append(nonEmpty, entry)
		}
	}
	sawBody := false
	for _, entry := range nonEmpty {
		if entry == candidateBody {
			sawBody = true
			break
		}
	}
	if !sawBody {
		t.Fatalf("candidate body not found verbatim in the read-back window-resized array; installed=%q read-back entries=%v", candidateBody, nonEmpty)
	}
	for _, pin := range pins {
		found := false
		for _, entry := range nonEmpty {
			if entry == pin {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("pin %q not found in the read-back window-resized array; entries=%v", pin, nonEmpty)
		}
	}
}
