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

	const noTestSeamRecord = validDecisionRecord +
		"\n### Added after Discussion (parent, 2026-11-19): No test seam: the bug lives in a tmux race\n\nDecision text.\n"
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
		wantTerminal bool
		wantRan      bool
		wantFindings []string
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
			name:    "a check that exits 3 fails with the green way forward",
			labels:  bugLabels,
			record:  validDecisionRecord,
			command: "echo green-marker; exit 3",
			wantRan: true,
			wantFindings: []string{
				"Every test the diff adds or changes passed on the merge base, so none of them shows the bug the fix fixes.",
				"Way forward: make a test fail on the merge base and pass with the fix, reaching the bug through code the base already has, and commit it.",
				"Full log: ", "green-marker",
			},
		},
		{
			name:    "a check that exits 4 fails naming the No test seam record",
			labels:  bugLabels,
			record:  validDecisionRecord,
			command: "exit 4",
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
			name:    "a check that exits 5 fails naming the scoper it cannot judge under and the No test seam record",
			labels:  bugLabels,
			record:  validDecisionRecord,
			command: "exit 5",
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

			got, err := NewRedFirstGate(worktree, verifyDir, "Darn-Write red-first gate", tt.command, tt.labels, recordPath, nil)()

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("gate() error = %v; want %v", err, tt.wantErr)
			}
			if got.Passed != tt.wantPassed || got.Terminal != tt.wantTerminal {
				t.Errorf("gate() = passed %v, terminal %v; want passed %v, terminal %v (findings %q)", got.Passed, got.Terminal, tt.wantPassed, tt.wantTerminal, got.Findings)
			}
			for _, want := range tt.wantFindings {
				if !strings.Contains(got.Findings, want) {
					t.Errorf("Findings = %q; want it to contain %q", got.Findings, want)
				}
			}
			if ran := len(verifyLogs(t, verifyDir)) > 0; ran != tt.wantRan {
				t.Errorf("command ran = %v; want %v", ran, tt.wantRan)
			}
		})
	}
}
