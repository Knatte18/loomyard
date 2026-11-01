//go:build tier0

// buttonclick_test.go covers buttonclick.go's click of a status-bar button and the confirmed line it composes.
// The click is driven through TmuxCmd's execHook seam with a stub process runner, so nothing is spawned.

package reedengine

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shell"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

// buttonRunCall is one call of the stub process runner.
type buttonRunCall struct {
	shell, flag, script, dir string
}

// TestRunButtonVia drives one click through each path of runButtonVia and asserts what runs, what is prompted and what the clicking client is shown.
// It reads the logger's output through logcapture, which is process-global, so it does not call t.Parallel.
func TestRunButtonVia(t *testing.T) {
	posix := shell.Posix()
	const exe = "/opt/lym/bin/lym"
	const strandName = "tst:a-task:agent"
	plain := buttonSpecEntry{Index: 0, Label: "go", Run: "echo hi"}
	confirming := buttonSpecEntry{Index: 0, Label: "go", Run: "rm x", Confirm: "drop it?"}
	stranded := buttonSpecEntry{Index: 0, Label: "go", Run: "printf %s {strand}", Strand: true}
	strandedConfirming := buttonSpecEntry{Index: 0, Label: "go", Run: "kill {strand}", Confirm: "kill {strand}?", Strand: true}

	// promptFor is the confirm-before argv a click on entry raises for the given strand, with the run directory dir.
	promptFor := func(entry buttonSpecEntry, prompt, strand string) func(dir string) []string {
		return func(dir string) []string {
			req := ButtonRequest{TmuxPath: "tmux", SocketPath: "/sock", Session: "sess", Range: "lymbtn0", Client: "c1"}
			line := composeConfirmedButtonLine(posix, exe, req, dir, buttonRunHash(entry.Run), strand)
			return []string{"confirm-before", "-t", "c1", "-p", prompt, "run-shell -b " + tmuxQuote(line)}
		}
	}

	tests := []struct {
		name        string
		entry       buttonSpecEntry
		rawSpec     *string
		specErr     error
		rangeName   string
		confirmed   bool
		hash        string
		strand      string
		strandRead  string
		strandErr   error
		exeErr      error
		runOut      string
		runCode     int
		runErr      error
		removeDir   bool
		messageErr  bool
		wantRun     bool
		wantRunText string
		wantMessage string
		wantConfirm func(dir string) []string
	}{
		{name: "PlainEntryRunsWithTheSpecsShellFlagAndDirectory", entry: plain, wantRun: true, wantRunText: "echo hi"},
		{name: "RangeWithNoEntryAtItsIndexShowsTheChangedMessage", entry: plain, rangeName: "lymbtn7", wantMessage: "lym: the buttons changed; click again"},
		{name: "RangeWithANonNumberSuffixShowsTheChangedMessage", entry: plain, rangeName: "lymbtnx", wantMessage: "lym: the buttons changed; click again"},
		{name: "UnreadableSpecShowsTheNotRenderedMessage", entry: plain, specErr: errors.New("no server"), wantMessage: "lym: the buttons are not rendered for this session; reattach it with lym reed attach"},
		{name: "SpecThatIsNotJSONShowsTheNotRenderedMessage", entry: plain, rawSpec: ptr("{"), wantMessage: "lym: the buttons are not rendered for this session; reattach it with lym reed attach"},
		{name: "EmptySpecShowsTheNotRenderedMessage", entry: plain, rawSpec: ptr(""), wantMessage: "lym: the buttons are not rendered for this session; reattach it with lym reed attach"},
		{name: "StrandEntryRunsWithTheClickedPanesNameQuoted", entry: stranded, strandRead: strandName, wantRun: true, wantRunText: "printf %s " + posix.Quote(strandName)},
		{name: "StrandEntryWithAnEmptyReadRunsNothing", entry: stranded, wantMessage: "lym: go needs the agent's name, which this pane lacks; reattach the session with lym reed attach and click again"},
		{name: "StrandEntryWithAFailedReadRunsNothing", entry: stranded, strandErr: errors.New("no client"), wantMessage: "lym: go needs the agent's name, which this pane lacks; reattach the session with lym reed attach and click again"},
		{name: "ConfirmEntryRaisesThePromptAndRunsNothing", entry: confirming, wantConfirm: promptFor(confirming, "drop it? (y/n)", "")},
		{name: "ConfirmEntryWithAnUnresolvableExecutableRaisesNothing", entry: confirming, exeErr: errors.New("no exe"), wantMessage: "lym: go could not raise its prompt; see the lym log in DIR/.lym/logs"},
		{name: "ConfirmedCallWithTheMatchingHashRuns", entry: confirming, confirmed: true, hash: buttonRunHash(confirming.Run), wantRun: true, wantRunText: "rm x"},
		{name: "ConfirmedCallWithAnotherHashRunsNothing", entry: confirming, confirmed: true, hash: "other", wantMessage: "lym: the buttons changed since go asked; click again"},
		{name: "ConfirmedStrandThatIsNoNameRunsNothing", entry: strandedConfirming, confirmed: true, hash: buttonRunHash(strandedConfirming.Run), strand: "not a name", wantMessage: "lym: go: the strand name not a name is not a strand name; click the button again from the agent pane"},
		{name: "ConfirmedStrandRunsWithTheCarriedName", entry: strandedConfirming, confirmed: true, hash: buttonRunHash(strandedConfirming.Run), strand: strandName, wantRun: true, wantRunText: "kill " + posix.Quote(strandName)},
		{name: "PromptDoublesHashAndPercentOfAStrandValue", entry: strandedConfirming, strandRead: "a#b%c", wantConfirm: promptFor(strandedConfirming, "kill a##b%%c? (y/n)", "a#b%c")},
		{name: "RemovedDirectoryOfAPlainEntryRunsNothing", entry: plain, removeDir: true, wantMessage: "lym: go: DIR is gone; the row re-renders at the session's next attach"},
		{name: "RemovedDirectoryOfAConfirmEntryRaisesNoPrompt", entry: confirming, removeDir: true, wantMessage: "lym: go: DIR is gone; the row re-renders at the session's next attach"},
		{name: "NonZeroExitShowsTheFirstOutputLine", entry: plain, runOut: "first\nsecond", runCode: 3, wantRun: true, wantRunText: "echo hi", wantMessage: "lym: go failed (exit 3): first"},
		{name: "LabelAndOutputAreShownLiterally", entry: buttonSpecEntry{Index: 0, Label: "a#b", Run: "echo hi"}, runOut: "100% done", runCode: 1, wantRun: true, wantRunText: "echo hi", wantMessage: "lym: a##b failed (exit 1): 100%% done"},
		{name: "SpawnFailureShowsItsError", entry: plain, runCode: -1, runErr: errors.New("fork failed"), wantRun: true, wantRunText: "echo hi", wantMessage: "lym: go failed (exit -1): fork failed"},
		{name: "FailedMessageIsLoggedAtDebugOnly", entry: plain, runOut: "x", runCode: 2, messageErr: true, wantRun: true, wantRunText: "echo hi", wantMessage: "lym: go failed (exit 2): x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.removeDir {
				dir = filepath.Join(dir, "gone")
			}
			spec := buttonSpec{Shell: "/bin/sh", Flag: "-c", Dir: dir, Entries: []buttonSpecEntry{tt.entry}}
			specJSON, err := json.Marshal(spec)
			if err != nil {
				t.Fatalf("marshal spec: %v", err)
			}
			specText := string(specJSON)
			if tt.rawSpec != nil {
				specText = *tt.rawSpec
			}

			cmd := newTmuxCmdForSocketPath("tmux", "/sock")
			fake := installFakeTmuxOn(t, &cmd)
			fake.answer("show-options", specText, tt.specErr)
			fake.answerFunc("display-message", func(args []string) (string, error) {
				if slices.Contains(args, "-p") {
					return tt.strandRead, tt.strandErr
				}
				if tt.messageErr {
					return "", errors.New("client gone")
				}
				return "", nil
			})
			logs := logcapture.Capture(t)

			var runs []buttonRunCall
			deps := buttonDeps{
				run: func(shellPath, flag, script, runDir string) (string, int, error) {
					runs = append(runs, buttonRunCall{shellPath, flag, script, runDir})
					return tt.runOut, tt.runCode, tt.runErr
				},
				executablePath: func() (string, error) { return exe, tt.exeErr },
				sh:             posix,
			}
			rangeName := tt.rangeName
			if rangeName == "" {
				rangeName = "lymbtn0"
			}
			req := ButtonRequest{
				TmuxPath: "tmux", SocketPath: "/sock", Session: "sess", Range: rangeName, Client: "c1",
				Confirmed: tt.confirmed, Hash: tt.hash, Strand: tt.strand,
			}

			runButtonVia(cmd, req, deps)

			wantRuns := []buttonRunCall(nil)
			if tt.wantRun {
				wantRuns = []buttonRunCall{{"/bin/sh", "-c", tt.wantRunText, dir}}
			}
			if !slices.Equal(runs, wantRuns) {
				t.Errorf("process runs = %+v, want %+v", runs, wantRuns)
			}

			var messages []string
			for _, call := range fake.ArgvFor("display-message") {
				if !slices.Contains(call, "-p") {
					if len(call) != 4 || call[1] != "-c" || call[2] != "c1" {
						t.Fatalf("message call = %v, want display-message -c c1 <text>", call)
					}
					messages = append(messages, call[3])
				}
			}
			var wantMessages []string
			if tt.wantMessage != "" {
				wantMessages = []string{strings.ReplaceAll(tt.wantMessage, "DIR", dir)}
			}
			if !slices.Equal(messages, wantMessages) {
				t.Errorf("messages shown = %q, want %q", messages, wantMessages)
			}

			confirms := fake.ArgvFor("confirm-before")
			if tt.wantConfirm == nil {
				if len(confirms) != 0 {
					t.Errorf("confirm-before calls = %v, want none", confirms)
				}
			} else {
				want := tt.wantConfirm(dir)
				if len(confirms) != 1 || !slices.Equal(confirms[0], want) {
					t.Errorf("confirm-before calls = %q, want exactly %q", confirms, want)
				}
				if line := want[len(want)-1]; !strings.HasPrefix(line, `run-shell -b "cd `) {
					t.Errorf("confirmed line command = %q, want it to open with the change to the run directory", line)
				}
			}

			if tt.messageErr && logs.String() != "" {
				t.Errorf("log = %q, want a failed message logged below Warn", logs.String())
			}
		})
	}

	t.Run("ConfirmedLineAndTmuxQuoteForBothDialects", func(t *testing.T) {
		req := ButtonRequest{TmuxPath: "/t'mux", SocketPath: "/s ock", Session: "se ss", Range: "lymbtn0", Client: "/dev/pts/1"}
		const dir = "/d i#r"
		tests := []struct {
			name         string
			sh           shell.Shell
			strand       string
			wantLine     string
			wantQuoted   string
			wantQuoteArg string
		}{
			{
				name:       "PosixWithStrand",
				sh:         shell.Posix(),
				strand:     "tst:s:a",
				wantLine:   `cd '/d i#r' >/dev/null 2>&1; '/o pt/lym' reed button --socket '/s ock' --tmux '/t'\''mux' --session 'se ss' --range 'lymbtn0' --client '/dev/pts/1' --confirmed --hash 'abc' --strand 'tst:s:a' >/dev/null 2>&1; exit 0`,
				wantQuoted: `"cd '/d i##r' >/dev/null 2>&1; '/o pt/lym' reed button --socket '/s ock' --tmux '/t'\\''mux' --session 'se ss' --range 'lymbtn0' --client '/dev/pts/1' --confirmed --hash 'abc' --strand 'tst:s:a' >/dev/null 2>&1; exit 0"`,
			},
			{
				name:       "PosixWithoutStrand",
				sh:         shell.Posix(),
				wantLine:   `cd '/d i#r' >/dev/null 2>&1; '/o pt/lym' reed button --socket '/s ock' --tmux '/t'\''mux' --session 'se ss' --range 'lymbtn0' --client '/dev/pts/1' --confirmed --hash 'abc' >/dev/null 2>&1; exit 0`,
				wantQuoted: `"cd '/d i##r' >/dev/null 2>&1; '/o pt/lym' reed button --socket '/s ock' --tmux '/t'\\''mux' --session 'se ss' --range 'lymbtn0' --client '/dev/pts/1' --confirmed --hash 'abc' >/dev/null 2>&1; exit 0"`,
			},
			{
				name:       "Pwsh",
				sh:         shell.Pwsh(),
				strand:     "tst:s:a",
				wantLine:   `Set-Location -LiteralPath '/d i#r' *> $null; & '/o pt/lym' reed button --socket '/s ock' --tmux '/t''mux' --session 'se ss' --range 'lymbtn0' --client '/dev/pts/1' --confirmed --hash 'abc' --strand 'tst:s:a' *> $null; exit 0`,
				wantQuoted: `"Set-Location -LiteralPath '/d i##r' *> \$null; & '/o pt/lym' reed button --socket '/s ock' --tmux '/t''mux' --session 'se ss' --range 'lymbtn0' --client '/dev/pts/1' --confirmed --hash 'abc' --strand 'tst:s:a' *> \$null; exit 0"`,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				line := composeConfirmedButtonLine(tt.sh, "/o pt/lym", req, dir, "abc", tt.strand)
				if line != tt.wantLine {
					t.Errorf("composeConfirmedButtonLine = %q, want %q", line, tt.wantLine)
				}
				if got := tmuxQuote(line); got != tt.wantQuoted {
					t.Errorf("tmuxQuote(line) = %q, want %q", got, tt.wantQuoted)
				}
			})
		}

		if got, want := tmuxQuote(`a"b$c\d#e`), `"a\"b\$c\\d##e"`; got != want {
			t.Errorf("tmuxQuote with every special character = %q, want %q", got, want)
		}
	})
}

// ptr returns a pointer to s, for a table field where the zero string and absence differ.
func ptr(s string) *string { return &s }
