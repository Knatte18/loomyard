// redfirstgate_test.go exercises NewRedFirstGate against a real scratch repo and real shell processes.

package loomshed

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestRedFirstGate_VerdictPerLabelRecordAndExitCode(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const lowerCaseNoTestSeamRecord = validDecisionRecord +
		"\n### Added after Discussion (parent, 2026-11-19): no test seam: the bug lives in a tmux race\n\nDecision text.\n"
	labelsErr := errors.New("board unreadable")
	bugLabels := func() ([]string, error) { return []string{"area", gateslot.BugLabel}, nil }
	timeoutLog := filepath.Join(t.TempDir(), "timeout.txt")
	if err := os.WriteFile(timeoutLog, []byte(gateslot.TierTimeoutLine("tier1", time.Minute)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name         string
		labels       func() ([]string, error)
		record       string
		command      string
		wantErr      error
		wantPassed   bool
		wantPending  bool
		wantTerminal bool
		// final runs the entry's Final closure in place of its Gate.
		final        bool
		wantRan      bool
		wantFindings []string
		// wantSend are substrings of the pending result's one line to the writer.
		wantSend []string
	}{
		{
			name:       "an entry without the bug label passes and runs nothing",
			labels:     func() ([]string, error) { return []string{"feature"}, nil },
			command:    "exit 9",
			wantPassed: true,
		},
		{
			name:       "a record whose added entry opens No test seam passes and runs nothing",
			labels:     bugLabels,
			record:     noTestSeamRecord,
			command:    "exit 9",
			wantPassed: true,
		},
		{
			name:       "a record whose added entry opens no test seam in lower case passes and runs nothing",
			labels:     bugLabels,
			record:     lowerCaseNoTestSeamRecord,
			command:    "exit 9",
			wantPassed: true,
		},
		{
			name:       "a record whose added entry opens No bug fixed passes and runs nothing",
			labels:     bugLabels,
			record:     noBugFixedRecord,
			command:    "exit 3",
			wantPassed: true,
		},
		{
			name:    "a bug entry whose decision record cannot be read is the gate's error",
			labels:  bugLabels,
			command: "exit 0",
			wantErr: os.ErrNotExist,
		},
		{
			name:       "a red-first check that exits 0 passes",
			labels:     bugLabels,
			record:     validDecisionRecord,
			command:    "exit 0",
			wantPassed: true,
			wantRan:    true,
		},
		{
			name:    "a check that exits 3 fails with the green way forward for a bug and for a change that fixes none",
			labels:  bugLabels,
			record:  validDecisionRecord,
			command: "echo green-marker; exit 3",
			wantRan: true,
			wantFindings: []string{
				"Every test the diff adds or changes passed on the merge base, so none of them fails before the change.",
				"Way forward: when the change fixes a bug, make a test fail on the merge base and pass with the fix, reaching the bug through code the base already has, and commit it.",
				"When the change fixes no bug, as one that only adds tests pinning existing behaviour, invent no bug and keep the tests:",
				"lym creel post --parent --expect-reply",
				`under a title opening "No bug fixed"`,
				"Full log: ", "green-marker",
			},
		},
		{
			name:        "a check that exits 4 holds the run and tells the writer to add a test or ask for the No test seam record",
			labels:      bugLabels,
			record:      validDecisionRecord,
			command:     "exit 4",
			wantPending: true,
			wantRan:     true,
			wantSend: []string{
				"holds your handoff, spending no attempt, since the diff adds or changes no test",
				"add and commit a test that fails on the merge base and passes with the fix",
				"lym creel post --parent --expect-reply",
				`under a title opening "No test seam"`,
			},
		},
		{
			name:        "a check that exits 5 holds the run and tells the writer to ask for the No test seam record",
			labels:      bugLabels,
			record:      validDecisionRecord,
			command:     "exit 5",
			wantPending: true,
			wantRan:     true,
			wantSend: []string{
				"cannot judge the diff's tests on the merge base: the hub's scoper is not go",
				"`lym loom decision add`",
				`under a title opening "No test seam"`,
				"then end your turn.",
			},
		},
		{
			name:    "a final evaluation of a check that exits 4 fails naming the No test seam record",
			labels:  bugLabels,
			record:  validDecisionRecord,
			command: "exit 4",
			final:   true,
			wantRan: true,
			wantFindings: []string{
				"The diff adds or changes no test, so nothing shows the bug the fix fixes.",
				"lym creel post --parent --expect-reply",
				"`lym loom decision add`",
				`under a title opening "No test seam"`,
				"then end your turn.",
				"Full log: ",
			},
		},
		{
			name:    "a final evaluation of a check that exits 5 fails naming the scoper it cannot judge under and the No test seam record",
			labels:  bugLabels,
			record:  validDecisionRecord,
			command: "exit 5",
			final:   true,
			wantRan: true,
			wantFindings: []string{
				"The red-first check cannot judge the diff's tests on the merge base: the hub's scoper is not go.",
				"lym creel post --parent --expect-reply",
				"`lym loom decision add`",
				`under a title opening "No test seam"`,
				"then end your turn.",
				"Full log: ",
			},
		},
		{
			name:         "a check that exits 4 after a tier timeout fails terminally instead of holding",
			labels:       bugLabels,
			record:       validDecisionRecord,
			command:      "cat " + timeoutLog + "; exit 4",
			wantTerminal: true,
			wantRan:      true,
			wantFindings: []string{"The red-first check timed out"},
		},
		{
			name:         "a check that exits 5 after a tier timeout fails terminally instead of holding",
			labels:       bugLabels,
			record:       validDecisionRecord,
			command:      "cat " + timeoutLog + "; exit 5",
			wantTerminal: true,
			wantRan:      true,
			wantFindings: []string{"The red-first check timed out"},
		},
		{
			name:       "a final evaluation of an entry without the bug label passes and runs nothing",
			labels:     func() ([]string, error) { return []string{"feature"}, nil },
			command:    "exit 4",
			final:      true,
			wantPassed: true,
		},
		{
			name:       "a final evaluation of a record whose added entry opens No test seam passes and runs nothing",
			labels:     bugLabels,
			record:     noTestSeamRecord,
			command:    "exit 4",
			final:      true,
			wantPassed: true,
		},
		{
			name:         "a check whose log holds the tier timeout line fails terminally",
			labels:       bugLabels,
			record:       validDecisionRecord,
			command:      "cat " + timeoutLog + "; exit 1",
			wantTerminal: true,
			wantRan:      true,
			wantFindings: []string{"The red-first check timed out", "resuming it runs the check again."},
		},
		{
			name:         "a check that exits any other code fails terminally",
			labels:       bugLabels,
			record:       validDecisionRecord,
			command:      "echo odd-marker; exit 9",
			wantTerminal: true,
			wantRan:      true,
			wantFindings: []string{
				"The red-first check failed with exit code 9, so it judged none of the diff's tests.",
				"resuming it runs the check again.",
				"Full log: ", "odd-marker",
			},
		},
		{
			name:    "a labels read error is the gate's error",
			labels:  func() ([]string, error) { return nil, labelsErr },
			command: "exit 0",
			wantErr: labelsErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			worktree, verifyDir := t.TempDir(), t.TempDir()
			gitkit.Git(t, worktree, "init", "-b", "main")
			gitkit.Git(t, worktree, "config", "user.email", "test@test.com")
			gitkit.Git(t, worktree, "config", "user.name", "Test")
			gitkit.CommitFile(t, worktree, "a.txt", "a\n", "init")
			recordPath := filepath.Join(t.TempDir(), "decision-record.md")
			if tt.record != "" {
				if err := os.WriteFile(recordPath, []byte(tt.record), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			gate := NewRedFirstGate(worktree, verifyDir, "Darn-Write red-first gate", tt.command, tt.labels, recordPath, nil)
			closure := gate.Gate
			if tt.final {
				closure = gate.Final
			}

			got, err := closure()

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("gate() error = %v; want %v", err, tt.wantErr)
			}
			if got.Passed != tt.wantPassed || got.Pending != tt.wantPending || got.Terminal != tt.wantTerminal {
				t.Errorf("gate() = passed %v, pending %v, terminal %v; want passed %v, pending %v, terminal %v (findings %q)", got.Passed, got.Pending, got.Terminal, tt.wantPassed, tt.wantPending, tt.wantTerminal, got.Findings)
			}
			for _, want := range tt.wantFindings {
				if !strings.Contains(got.Findings, want) {
					t.Errorf("Findings = %q; want it to contain %q", got.Findings, want)
				}
			}
			for _, want := range tt.wantSend {
				if !strings.Contains(got.Send, want) {
					t.Errorf("Send = %q; want it to contain %q", got.Send, want)
				}
			}
			if strings.Contains(got.Send, "\n") {
				t.Errorf("Send = %q; want one line", got.Send)
			}
			if ran := len(verifyLogs(t, verifyDir)) > 0; ran != tt.wantRan {
				t.Errorf("command ran = %v; want %v", ran, tt.wantRan)
			}
		})
	}
}

// noTestSeamRecord is a decision record whose added entry records the bug as having no test seam.
const noTestSeamRecord = validDecisionRecord +
	"\n### Added after Discussion (parent, 2026-11-19): No test seam: the bug lives in a tmux race\n\nDecision text.\n"

// noBugFixedRecord is a decision record whose added entry records that the bug-labelled entry fixes no bug.
const noBugFixedRecord = validDecisionRecord +
	"\n### Added after Discussion (parent, 2026-11-23): No bug fixed: the task only adds golden tests\n\nDecision text.\n"

// TestRedFirstGate_FinalOnTheHeldTreeFailsFromTheHoldWithoutRunningTheCheck pins the final evaluation of a held tree.
// It answers the hold's refusal as a failure with its findings.
// It runs the check no second time.
func TestRedFirstGate_FinalOnTheHeldTreeFailsFromTheHoldWithoutRunningTheCheck(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	worktree, verifyDir := t.TempDir(), t.TempDir()
	gitkit.Git(t, worktree, "init", "-b", "main")
	gitkit.Git(t, worktree, "config", "user.email", "test@test.com")
	gitkit.Git(t, worktree, "config", "user.name", "Test")
	gitkit.CommitFile(t, worktree, "a.txt", "a\n", "init")
	recordPath := filepath.Join(t.TempDir(), "decision-record.md")
	if err := os.WriteFile(recordPath, []byte(validDecisionRecord), 0o644); err != nil {
		t.Fatal(err)
	}
	bugLabels := func() ([]string, error) { return []string{gateslot.BugLabel}, nil }
	gate := NewRedFirstGate(worktree, verifyDir, "Darn-Write red-first gate", "exit 4", bugLabels, recordPath, nil)
	if held, err := gate.Gate(); err != nil || !held.Pending {
		t.Fatalf("Gate() = %+v, %v; want a pending hold", held, err)
	}

	got, err := gate.Final()

	if err != nil {
		t.Fatalf("Final() error = %v; want nil", err)
	}
	if got.Passed || got.Pending || got.Terminal {
		t.Errorf("Final() = passed %v, pending %v, terminal %v; want a plain failure", got.Passed, got.Pending, got.Terminal)
	}
	if want := "The diff adds or changes no test, so nothing shows the bug the fix fixes."; !strings.Contains(got.Findings, want) {
		t.Errorf("Findings = %q; want it to contain %q", got.Findings, want)
	}
	if runs := len(verifyLogs(t, verifyDir)); runs != 1 {
		t.Errorf("the check ran %d times; want 1", runs)
	}
}

// TestRedFirstGate_HoldRunsNothingUntilACommitOrTheRecordLands pins the hold on a refusal.
// Only a commit or the parent's record lifts it.
// The writer is offered one line until the loop reports it delivered.
// The check runs again only on a new HEAD tree.
// A refusal after a passing run holds afresh, and the parent's record lifts the hold.
func TestRedFirstGate_HoldRunsNothingUntilACommitOrTheRecordLands(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	worktree, verifyDir := t.TempDir(), t.TempDir()
	gitkit.Git(t, worktree, "init", "-b", "main")
	gitkit.Git(t, worktree, "config", "user.email", "test@test.com")
	gitkit.Git(t, worktree, "config", "user.name", "Test")
	gitkit.CommitFile(t, worktree, "a.txt", "a\n", "init")
	recordPath := filepath.Join(t.TempDir(), "decision-record.md")
	writeRecord := func(text string) {
		t.Helper()
		if err := os.WriteFile(recordPath, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeRecord(validDecisionRecord)
	bugLabels := func() ([]string, error) { return []string{gateslot.BugLabel}, nil }
	exitCodePath := filepath.Join(t.TempDir(), "exit-code")
	writeExitCode := func(code string) {
		t.Helper()
		if err := os.WriteFile(exitCodePath, []byte(code), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeExitCode("5")
	gate := NewRedFirstGate(worktree, verifyDir, "Darn-Write red-first gate", "exit $(cat "+exitCodePath+")", bugLabels, recordPath, nil).Gate

	steps := []struct {
		name        string
		before      func()
		wantPassed  bool
		wantPending bool
		wantSend    bool
		// deliver reports the pending answer's line as delivered, as the loop does after a send that landed.
		deliver  bool
		wantRuns int
	}{
		{name: "the first refusal holds and offers the writer a line, the send failing", wantPending: true, wantSend: true, wantRuns: 1},
		{name: "a re-check on the same tree offers the line again and runs nothing", wantPending: true, wantSend: true, deliver: true, wantRuns: 1},
		{name: "an arrival on the same tree holds silently and runs nothing", wantPending: true, wantRuns: 1},
		{
			name:        "an arrival on a new commit runs the check again and holds silently",
			before:      func() { gitkit.CommitFile(t, worktree, "b.txt", "b\n", "next") },
			wantPending: true,
			wantRuns:    2,
		},
		{
			name:       "a commit that makes the check pass lifts the hold",
			before:     func() { writeExitCode("0"); gitkit.CommitFile(t, worktree, "c.txt", "c\n", "fix") },
			wantPassed: true,
			wantRuns:   3,
		},
		{
			name:        "a refusal after the passing run holds afresh and offers the line again",
			before:      func() { writeExitCode("5"); gitkit.CommitFile(t, worktree, "d.txt", "d\n", "regress") },
			wantPending: true,
			wantSend:    true,
			deliver:     true,
			wantRuns:    4,
		},
		{name: "the parent's No test seam record lifts the hold", before: func() { writeRecord(noTestSeamRecord) }, wantPassed: true, wantRuns: 4},
	}
	for _, step := range steps {
		if step.before != nil {
			step.before()
		}

		got, err := gate()

		if err != nil {
			t.Fatalf("%s: gate() error = %v; want nil", step.name, err)
		}
		if got.Passed != step.wantPassed || got.Pending != step.wantPending || (got.Send != "") != step.wantSend {
			t.Errorf("%s: gate() = passed %v, pending %v, send %q; want passed %v, pending %v, a send %v", step.name, got.Passed, got.Pending, got.Send, step.wantPassed, step.wantPending, step.wantSend)
		}
		if runs := len(verifyLogs(t, verifyDir)); runs != step.wantRuns {
			t.Errorf("%s: the check ran %d times; want %d", step.name, runs, step.wantRuns)
		}
		if step.wantSend && got.Delivered == nil {
			t.Errorf("%s: the pending answer has no Delivered flag; want one so the loop reports the delivery", step.name)
		}
		if step.deliver && got.Delivered != nil {
			*got.Delivered = true
		}
	}
}
