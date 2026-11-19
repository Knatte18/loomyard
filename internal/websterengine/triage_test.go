// triage_test.go drives the verify gate's flaky-failure helpers: the warning and the friction note.

package websterengine

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// testFlaky returns a flaky failure of kind with id, its first run's log and its output tail.
func testFlaky(id, kind, log, tail string) flakyFailure {
	return flakyFailure{failure: VerifyFailure{ID: id, Kind: kind, Tail: tail}, log: log}
}

func TestTriageWarnings(t *testing.T) {
	tiergate.Require(t, 0)
	cases := []struct {
		name string
		in   []flakyFailure
		want []string
	}{
		{
			"flaky tests",
			[]flakyFailure{testFlaky("p.A", FailureKindTest, "/v/1.log", "boom"), testFlaky("p", FailureKindPackage, "/v/1.log", "")},
			[]string{"integration verify: flaky failures passed or vanished on rerun: p.A, p"},
		},
		{
			"a verify command failure is no flaky test",
			[]flakyFailure{testFlaky(opaqueFailureID, FailureKindOpaque, "/v/1.log", "gate error")},
			[]string{"integration verify: the verify command failed naming no failing test and passed on rerun; first run's log: /v/1.log"},
		},
		{"none", nil, nil},
	}
	for _, c := range cases {
		if got := triageWarnings(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %q; want %q", c.name, got, c.want)
		}
	}
}

func TestWriteTriageFrictionNote(t *testing.T) {
	tiergate.Require(t, 0)
	t.Run("content", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "friction")
		flaky := []flakyFailure{
			testFlaky("p.A", FailureKindTest, "/v/1.log", "a_test.go:3: boom\nmore"),
			testFlaky("p", FailureKindPackage, "/v/1.log", ""),
			testFlaky(opaqueFailureID, FailureKindOpaque, "/v/3.log", "gate test: build failed"),
		}
		if err := writeTriageFrictionNote(dir, flaky); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join(dir, "webster-verify-triage.md"))
		if err != nil {
			t.Fatal(err)
		}
		want := "Integration-suite triage: non-regression verify failures\n\n" +
			"These identities failed the first verify run and passed or vanished on rerun (flaky):\n\n" +
			"- p.A, first run's log /v/1.log\n\n  ```text\n  a_test.go:3: boom\n  more\n  ```\n\n" +
			"- p, first run's log /v/1.log\n\n  Its output was empty.\n\n" +
			"The verify command failed its first run naming no failing test, and passed on rerun; the failure may be the verify step's own, not a test's:\n\n" +
			"- first run's log /v/3.log\n\n  ```text\n  gate test: build failed\n  ```\n\n"
		if string(got) != want {
			t.Errorf("note = %q; want %q", got, want)
		}
	})
	t.Run("empty dir", func(t *testing.T) {
		cwd := t.TempDir()
		t.Chdir(cwd)
		if err := writeTriageFrictionNote("", []flakyFailure{testFlaky("p.A", FailureKindTest, "/v/1.log", "")}); err != nil {
			t.Fatal(err)
		}
		if entries, _ := os.ReadDir(cwd); len(entries) != 0 {
			t.Errorf("wrote %d entries; want none", len(entries))
		}
	})
	t.Run("no flaky", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "friction")
		if err := writeTriageFrictionNote(dir, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("friction dir exists (err = %v); want nothing written", err)
		}
	})
}

// TestBackgroundShellNoteAndWarning asserts the friction note and the warning for an ended shell state, per outcome.
// Both name the shell, its signal and time outstanding, and say it was still running when the run ended.
// Both say the run never read its result and that lym did not stop the shell,
// and they state what ends it and the final outcome, in the same words.
func TestBackgroundShellNoteAndWarning(t *testing.T) {
	tiergate.Require(t, 0)
	endedShell := shuttleengine.EndedShell{Label: "sleep 9999", ID: "sh-1", Signal: shuttleengine.SignalTranscript, Outstanding: 90 * time.Second}
	const removal = "shuttle removes Master's strand when the run finishes"
	const reclaim = "the next `lym webster run` reclaims it at entry"
	cases := []struct {
		name     string
		outcome  backgroundShellOutcome
		wantEnds string
		wantTail string
	}{
		{"done", finishedShellOutcome(RunResult{Outcome: outcomeDone}), removal, "the run's final outcome: done"},
		{"stuck with a reason", finishedShellOutcome(RunResult{Outcome: outcomeStuck, StuckReason: "batch 2 red"}), removal, "the run's final outcome: stuck (batch 2 red)"},
		{"mapping error after a shuttle-done end", errorShellOutcome(errors.New("summary.md malformed"), false), removal, "the run's final outcome: error (summary.md malformed)"},
		{"died or timeout error", errorShellOutcome(errors.New("master died"), true), reclaim, "the run's final outcome: error (master died)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			if err := writeBackgroundShellFrictionNote(dir, []shuttleengine.EndedShell{endedShell}, tc.outcome); err != nil {
				t.Fatalf("writeBackgroundShellFrictionNote() error = %v", err)
			}
			note, err := os.ReadFile(filepath.Join(dir, "webster-background-shell.md"))
			if err != nil {
				t.Fatalf("read note: %v", err)
			}
			warning := expiredShellWarning(endedShell, tc.outcome)

			if !strings.Contains(string(note), "- "+warning+"\n") {
				t.Errorf("note = %q; want the warning %q as a bullet", note, warning)
			}
			for _, want := range []string{
				"`sleep 9999`",
				"(reported by the transcript signal)",
				"outstanding for 1m30s",
				"was still running",
				"when the run ended",
				"the run never read the shell's result",
				"lym did not stop the shell",
				tc.wantEnds,
				tc.wantTail,
			} {
				if !strings.Contains(warning, want) {
					t.Errorf("warning = %q; want it to contain %q", warning, want)
				}
			}
		})
	}
}
