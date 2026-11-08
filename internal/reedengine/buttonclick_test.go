// buttonclick_test.go covers buttonclick.go's click of a status-bar button and the re-entry lines its prompts run.
// The click is driven through TmuxCmd's execHook seam with a stub process runner, so nothing is spawned.

package reedengine

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shell"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// buttonRunCall is one call of the stub process runner.
type buttonRunCall struct {
	shell, flag, script, dir string
}

// TestRunButtonVia drives one click through each path of runButtonVia.
// It asserts what runs, what is prompted, and what the clicking client is shown.
// It reads the logger's output through logcapture, which is process-global, so it does not call t.Parallel.
func TestRunButtonVia(t *testing.T) {
	tiergate.Require(t, 0)
	posix := shell.Posix()
	const exe = "/opt/lym/bin/lym"
	const strandName = "tst:a-task:agent"
	plain := buttonSpecEntry{Index: 0, Label: "go", Run: "echo hi"}
	confirming := buttonSpecEntry{Index: 0, Label: "go", Run: "rm x", Confirm: "drop it?"}
	stranded := buttonSpecEntry{Index: 0, Label: "go", Run: "printf %s {strand}", Strand: true}
	strandedConfirming := buttonSpecEntry{Index: 0, Label: "go", Run: "kill {strand}", Confirm: "kill {strand}?", Strand: true}
	asking := buttonSpecEntry{Index: 0, Label: "go", Run: "open {answer}", Ask: "Name:", AskDefault: "dflt"}
	askingConfirming := buttonSpecEntry{Index: 0, Label: "go", Run: "open {answer}", Confirm: "open {answer}?", Ask: "Name:", AskDefault: "dflt"}
	strandedAsking := buttonSpecEntry{Index: 0, Label: "go", Run: "tell {strand} {answer}", Ask: "Say:", Strand: true}
	strandNamingAsk := buttonSpecEntry{Index: 0, Label: "go", Run: "tell {strand} {answer}", Ask: "Say to {strand}:", Strand: true}

	// promptRequest is the request every prompt's re-entry line is composed from.
	promptRequest := ButtonRequest{TmuxPath: "tmux", SocketPath: "/sock", Session: "sess", Range: "lymbtn0", Client: "c1"}

	// promptFor is the confirm-before argv a click on entry raises carrying values, with the run directory dir.
	promptFor := func(entry buttonSpecEntry, prompt string, values map[string]string) func(dir string) []string {
		return func(dir string) []string {
			line := composeButtonReentryLine(posix, exe, promptRequest, dir, buttonStageConfirmed, buttonRunHash(entry.Run), values)
			return []string{"confirm-before", "-t", "c1", "-p", prompt, "run-shell -b " + tmuxQuote(line)}
		}
	}

	// askFor is the command-prompt argv a click on entry raises carrying values, with the run directory dir.
	askFor := func(entry buttonSpecEntry, prompt string, values map[string]string) func(dir string) []string {
		return func(dir string) []string {
			line := composeButtonReentryLine(posix, exe, promptRequest, dir, buttonStageAnswered, buttonRunHash(entry.Run), values)
			return []string{"command-prompt", "-t", "c1", "-p", prompt, `set-option -t "=sess:" @lym_button_answer "%%%" ; run-shell -b ` + tmuxQuote(line)}
		}
	}

	tests := []struct {
		name       string
		entry      buttonSpecEntry
		rawSpec    *string
		specErr    error
		rangeName  string
		confirmed  bool
		answered   bool
		hash       string
		strand     string
		answer     string
		strandRead string
		strandErr  error
		answerRead string
		answerErr  error
		exeErr     error
		// testBinary hands the running test binary in as the lym executable.
		testBinary  bool
		runOut      string
		runCode     int
		runErr      error
		removeDir   bool
		anchorMoved bool
		anchorGone  bool
		anchorErr   error
		messageErr  bool
		wantRun     bool
		wantRunText string
		wantMessage string
		// wantLog must appear in the captured log; empty means no check.
		wantLog    string
		wantPrompt func(dir string) []string
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
		{name: "ConfirmEntryRaisesThePromptAndRunsNothing", entry: confirming, wantPrompt: promptFor(confirming, "drop it? (y/n)", nil)},
		{name: "ConfirmEntryWithAnUnresolvableExecutableRaisesNothing", entry: confirming, exeErr: errors.New("no exe"), anchorMoved: true, wantMessage: "lym: go could not raise its prompt; see the lym log in ANCHOR/.lym/logs"},
		{name: "ConfirmEntryHandedTheTestBinaryRaisesNothing", entry: confirming, testBinary: true, anchorMoved: true, wantMessage: "lym: go could not raise its prompt; see the lym log in ANCHOR/.lym/logs", wantLog: "could not resolve this binary"},
		{name: "ConfirmedCallWithTheMatchingHashRuns", entry: confirming, confirmed: true, hash: buttonRunHash(confirming.Run), wantRun: true, wantRunText: "rm x"},
		{name: "ConfirmedCallWithAnotherHashRunsNothing", entry: confirming, confirmed: true, hash: "other", wantMessage: "lym: the buttons changed since go asked; click again"},
		{name: "ConfirmedStrandThatIsNoNameRunsNothing", entry: strandedConfirming, confirmed: true, hash: buttonRunHash(strandedConfirming.Run), strand: "not a name", wantMessage: "lym: go: the strand name not a name is not a strand name; click the button again from the agent pane"},
		{name: "ConfirmedStrandRunsWithTheCarriedName", entry: strandedConfirming, confirmed: true, hash: buttonRunHash(strandedConfirming.Run), strand: strandName, wantRun: true, wantRunText: "kill " + posix.Quote(strandName)},
		{name: "PromptDoublesHashAndPercentOfAStrandValue", entry: strandedConfirming, strandRead: "a#b%c", wantPrompt: promptFor(strandedConfirming, "kill a##b%%c? (y/n)", map[string]string{"strand": "a#b%c"})},
		{name: "AskEntryRaisesTheAnswerPromptAndRunsNothing", entry: asking, wantPrompt: askFor(asking, "Name: ", nil)},
		{name: "AskEntryNamingStrandCarriesTheStrandInTheAnsweredLine", entry: strandedAsking, strandRead: strandName, wantPrompt: askFor(strandedAsking, "Say: ", map[string]string{"strand": strandName})},
		{name: "AskAndConfirmEntryRaisesTheAnswerPromptFirst", entry: askingConfirming, wantPrompt: askFor(askingConfirming, "Name: ", nil)},
		{name: "AskTextHoldingACommaFromAStrandValueRaisesNoPrompt", entry: strandNamingAsk, strandRead: "a,b", wantMessage: "lym: go: the ask text holds a ','"},
		{name: "AnsweredCallRunsWithTheStoredAnswerTrimmedAndQuoted", entry: asking, answered: true, hash: buttonRunHash(asking.Run), answerRead: " a b\n", wantRun: true, wantRunText: "open " + posix.Quote("a b")},
		{name: "AnsweredCallWithAnEmptyAnswerTakesTheAskDefault", entry: asking, answered: true, hash: buttonRunHash(asking.Run), answerRead: "\n", wantRun: true, wantRunText: "open " + posix.Quote("dflt")},
		{name: "AnsweredCallWithAFailedAnswerReadRunsNothing", entry: asking, answered: true, hash: buttonRunHash(asking.Run), answerErr: errors.New("no server"), wantMessage: "lym: go could not read its answer; click again"},
		{name: "AnsweredCallWithAnotherHashRunsNothing", entry: asking, answered: true, hash: "other", answerRead: "a", wantMessage: "lym: the buttons changed since go asked; click again"},
		{name: "AnsweredStrandEntryRunsWithTheCarriedNameNotThePanes", entry: strandedAsking, answered: true, hash: buttonRunHash(strandedAsking.Run), strand: strandName, answerRead: "hi", wantRun: true, wantRunText: "tell " + posix.Quote(strandName) + " " + posix.Quote("hi")},
		{name: "AnsweredConfirmEntryRaisesTheConfirmPromptCarryingTheAnswer", entry: askingConfirming, answered: true, hash: buttonRunHash(askingConfirming.Run), answerRead: "n#1", wantPrompt: promptFor(askingConfirming, "open n##1? (y/n)", map[string]string{"answer": "n#1"})},
		{name: "ConfirmedAskEntryRunsWithTheCarriedAnswerAndReadsNoOption", entry: askingConfirming, confirmed: true, hash: buttonRunHash(askingConfirming.Run), answer: "n m", answerErr: errors.New("must not be read"), wantRun: true, wantRunText: "open " + posix.Quote("n m")},
		{name: "RemovedDirectoryOfAPlainEntryRunsNothing", entry: plain, removeDir: true, wantMessage: "lym: go: DIR is gone; the row re-renders at the session's next attach"},
		{name: "RemovedDirectoryOfAConfirmEntryRaisesNoPrompt", entry: confirming, removeDir: true, wantMessage: "lym: go: DIR is gone; the row re-renders at the session's next attach"},
		{name: "AnchorMovedAfterTheRenderRunsInTheCurrentAnchor", entry: plain, anchorMoved: true, wantRun: true, wantRunText: "echo hi"},
		{name: "AnchorMovedAfterTheRenderPromptsWithALineOpeningInTheCurrentAnchor", entry: confirming, anchorMoved: true, wantPrompt: promptFor(confirming, "drop it? (y/n)", nil)},
		{name: "AnchorDirectoryThatIsGoneRunsNothingAndRaisesNoPrompt", entry: confirming, anchorGone: true, wantMessage: "lym: go: the anchor directory ANCHOR is gone; point the worktree's anchor at an existing directory"},
		{name: "UnresolvableAnchorRunsNothing", entry: plain, anchorErr: errors.New("no repository"), wantMessage: "lym: go: the anchor of DIR does not resolve: no repository"},
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
			runDir := dir
			if tt.anchorMoved || tt.anchorGone {
				runDir = filepath.Join(dir, "backend")
			}
			if tt.anchorMoved {
				if err := os.MkdirAll(runDir, 0o755); err != nil {
					t.Fatalf("create the moved anchor: %v", err)
				}
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
			fake.answerFunc("show-options", func(args []string) (string, error) {
				if slices.Contains(args, buttonAnswerOption) {
					return tt.answerRead, tt.answerErr
				}
				return specText, tt.specErr
			})
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
				executablePath: func() (string, error) {
					if tt.testBinary {
						return os.Executable()
					}
					return exe, tt.exeErr
				},
				anchorOf: func(specDir string) (string, error) {
					if specDir != dir {
						t.Errorf("anchorOf(%q), want it asked for the rendered directory %q", specDir, dir)
					}
					return runDir, tt.anchorErr
				},
				sh: posix,
			}
			rangeName := tt.rangeName
			if rangeName == "" {
				rangeName = "lymbtn0"
			}
			req := ButtonRequest{
				TmuxPath: "tmux", SocketPath: "/sock", Session: "sess", Range: rangeName, Client: "c1",
				Confirmed: tt.confirmed, Answered: tt.answered, Hash: tt.hash, Strand: tt.strand, Answer: tt.answer,
			}

			runButtonVia(cmd, req, deps)

			wantRuns := []buttonRunCall(nil)
			if tt.wantRun {
				wantRuns = []buttonRunCall{{"/bin/sh", "-c", tt.wantRunText, runDir}}
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
				wantMessages = []string{strings.NewReplacer("DIR", dir, "ANCHOR", runDir).Replace(tt.wantMessage)}
			}
			if !slices.Equal(messages, wantMessages) {
				t.Errorf("messages shown = %q, want %q", messages, wantMessages)
			}

			prompts := append(fake.ArgvFor("confirm-before"), fake.ArgvFor("command-prompt")...)
			if tt.wantPrompt == nil {
				if len(prompts) != 0 {
					t.Errorf("prompt calls = %v, want none", prompts)
				}
			} else {
				want := tt.wantPrompt(runDir)
				if len(prompts) != 1 || !slices.Equal(prompts[0], want) {
					t.Errorf("prompt calls = %q, want exactly %q", prompts, want)
				}
				if line := want[len(want)-1]; !strings.Contains(line, `run-shell -b "cd `) {
					t.Errorf("prompt command = %q, want its re-entry line to open with the change to the run directory", line)
				}
			}

			if tt.wantLog != "" && !strings.Contains(logs.String(), tt.wantLog) {
				t.Errorf("log = %q, want it to hold %q", logs.String(), tt.wantLog)
			}
			if tt.messageErr && logs.String() != "" {
				t.Errorf("log = %q, want a failed message logged below Warn", logs.String())
			}
		})
	}

	t.Run("ReentryLineAndTmuxQuoteForBothDialects", func(t *testing.T) {
		req := ButtonRequest{TmuxPath: "/t'mux", SocketPath: "/s ock", Session: "se ss", Range: "lymbtn0", Client: "/dev/pts/1"}
		const dir = "/d i#r"
		strandOnly := map[string]string{"strand": "tst:s:a"}
		tests := []struct {
			name       string
			sh         shell.Shell
			stage      string
			values     map[string]string
			wantLine   string
			wantQuoted string
		}{
			{
				name:       "PosixConfirmedWithStrand",
				sh:         shell.Posix(),
				stage:      buttonStageConfirmed,
				values:     strandOnly,
				wantLine:   `cd '/d i#r' >/dev/null 2>&1; '/o pt/lym' reed button --socket '/s ock' --tmux '/t'\''mux' --session 'se ss' --range 'lymbtn0' --client '/dev/pts/1' --confirmed --hash 'abc' --strand 'tst:s:a' >/dev/null 2>&1; exit 0`,
				wantQuoted: `"cd '/d i##r' >/dev/null 2>&1; '/o pt/lym' reed button --socket '/s ock' --tmux '/t'\\''mux' --session 'se ss' --range 'lymbtn0' --client '/dev/pts/1' --confirmed --hash 'abc' --strand 'tst:s:a' >/dev/null 2>&1; exit 0"`,
			},
			{
				name:       "PosixConfirmedWithoutValues",
				sh:         shell.Posix(),
				stage:      buttonStageConfirmed,
				wantLine:   `cd '/d i#r' >/dev/null 2>&1; '/o pt/lym' reed button --socket '/s ock' --tmux '/t'\''mux' --session 'se ss' --range 'lymbtn0' --client '/dev/pts/1' --confirmed --hash 'abc' >/dev/null 2>&1; exit 0`,
				wantQuoted: `"cd '/d i##r' >/dev/null 2>&1; '/o pt/lym' reed button --socket '/s ock' --tmux '/t'\\''mux' --session 'se ss' --range 'lymbtn0' --client '/dev/pts/1' --confirmed --hash 'abc' >/dev/null 2>&1; exit 0"`,
			},
			{
				name:       "PosixConfirmedWithStrandAndAnswer",
				sh:         shell.Posix(),
				stage:      buttonStageConfirmed,
				values:     map[string]string{"answer": "n'm", "strand": "tst:s:a"},
				wantLine:   `cd '/d i#r' >/dev/null 2>&1; '/o pt/lym' reed button --socket '/s ock' --tmux '/t'\''mux' --session 'se ss' --range 'lymbtn0' --client '/dev/pts/1' --confirmed --hash 'abc' --strand 'tst:s:a' --answer 'n'\''m' >/dev/null 2>&1; exit 0`,
				wantQuoted: `"cd '/d i##r' >/dev/null 2>&1; '/o pt/lym' reed button --socket '/s ock' --tmux '/t'\\''mux' --session 'se ss' --range 'lymbtn0' --client '/dev/pts/1' --confirmed --hash 'abc' --strand 'tst:s:a' --answer 'n'\\''m' >/dev/null 2>&1; exit 0"`,
			},
			{
				name:       "PosixAnsweredWithStrand",
				sh:         shell.Posix(),
				stage:      buttonStageAnswered,
				values:     strandOnly,
				wantLine:   `cd '/d i#r' >/dev/null 2>&1; '/o pt/lym' reed button --socket '/s ock' --tmux '/t'\''mux' --session 'se ss' --range 'lymbtn0' --client '/dev/pts/1' --answered --hash 'abc' --strand 'tst:s:a' >/dev/null 2>&1; exit 0`,
				wantQuoted: `"cd '/d i##r' >/dev/null 2>&1; '/o pt/lym' reed button --socket '/s ock' --tmux '/t'\\''mux' --session 'se ss' --range 'lymbtn0' --client '/dev/pts/1' --answered --hash 'abc' --strand 'tst:s:a' >/dev/null 2>&1; exit 0"`,
			},
			{
				name:       "PwshConfirmedWithStrand",
				sh:         shell.Pwsh(),
				stage:      buttonStageConfirmed,
				values:     strandOnly,
				wantLine:   `Set-Location -LiteralPath '/d i#r' *> $null; & '/o pt/lym' reed button --socket '/s ock' --tmux '/t''mux' --session 'se ss' --range 'lymbtn0' --client '/dev/pts/1' --confirmed --hash 'abc' --strand 'tst:s:a' *> $null; exit 0`,
				wantQuoted: `"Set-Location -LiteralPath '/d i##r' *> \$null; & '/o pt/lym' reed button --socket '/s ock' --tmux '/t''mux' --session 'se ss' --range 'lymbtn0' --client '/dev/pts/1' --confirmed --hash 'abc' --strand 'tst:s:a' *> \$null; exit 0"`,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				line := composeButtonReentryLine(tt.sh, "/o pt/lym", req, dir, tt.stage, "abc", tt.values)
				if line != tt.wantLine {
					t.Errorf("composeButtonReentryLine = %q, want %q", line, tt.wantLine)
				}
				if got := tmuxQuote(line); got != tt.wantQuoted {
					t.Errorf("tmuxQuote(line) = %q, want %q", got, tt.wantQuoted)
				}
			})
		}

		if got, want := tmuxQuote(`a"b$c\d#e`), `"a\"b\$c\\d##e"`; got != want {
			t.Errorf("tmuxQuote with every special character = %q, want %q", got, want)
		}
		if got, want := tmuxString(`a"b$c\d#e`), `"a\"b\$c\\d#e"`; got != want {
			t.Errorf("tmuxString with every special character = %q, want %q", got, want)
		}
	})
}

// ptr returns a pointer to s, for a table field where the zero string and absence differ.
func ptr(s string) *string { return &s }
