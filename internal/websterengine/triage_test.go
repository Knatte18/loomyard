// triage_test.go drives the verify gate's flaky-failure helpers: the warning and the friction note.

package websterengine

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// failOutput renders go test output failing the named tests in pkg, each at its subtest nesting depth.
func failOutput(pkg string, tests ...string) string {
	var b strings.Builder
	for _, name := range tests {
		indent := strings.Repeat("    ", strings.Count(name, "/"))
		b.WriteString(indent + "--- FAIL: " + name + " (0.00s)\n" + indent + "    x_test.go:1: boom " + name + "\n")
	}
	b.WriteString("FAIL\nFAIL\t" + pkg + "\t0.01s\n")
	return b.String()
}

func TestTriageWarnings(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{"flaky", []string{"p.A", "p.B"}, []string{"integration verify: flaky failures passed or vanished on rerun: p.A, p.B"}},
		{"none", nil, nil},
	}
	for _, c := range cases {
		if got := triageWarnings(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %q; want %q", c.name, got, c.want)
		}
	}
}

func TestWriteTriageFrictionNote(t *testing.T) {
	t.Run("content", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "friction")
		if err := writeTriageFrictionNote(dir, []string{"p.A", "p.B"}); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join(dir, "webster-verify-triage.md"))
		if err != nil {
			t.Fatal(err)
		}
		want := "Integration-suite triage: non-regression verify failures\n\n" +
			"These identities failed the first verify run and passed or vanished on rerun (flaky):\n\n- p.A\n- p.B\n\n"
		if string(got) != want {
			t.Errorf("note = %q; want %q", got, want)
		}
	})
	t.Run("empty dir", func(t *testing.T) {
		cwd := t.TempDir()
		t.Chdir(cwd)
		if err := writeTriageFrictionNote("", []string{"p.A"}); err != nil {
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

// TestBackgroundShellNoteAndWarning asserts the friction note and the warning for an expired shell state, per outcome, the shell, the bound, the counted turn end, that lyx did not stop the shell, what ends it and the outcome, in the same words.
func TestBackgroundShellNoteAndWarning(t *testing.T) {
	const removal = "shuttle removes Master's strand when the run finishes"
	const reclaim = "the next `lyx webster run` reclaims it at entry"
	cases := []struct {
		name     string
		outcome  backgroundShellOutcome
		wantEnds string
		wantTail string
	}{
		{"done", finishedShellOutcome(RunResult{Outcome: outcomeDone}), removal, "the run's outcome after that turn end: done"},
		{"stuck with a reason", finishedShellOutcome(RunResult{Outcome: outcomeStuck, StuckReason: "batch 2 red"}), removal, "the run's outcome after that turn end: stuck (batch 2 red)"},
		{"paused", finishedShellOutcome(RunResult{Outcome: outcomePaused}), removal, "the run's outcome after that turn end: paused"},
		{"mapping error after a shuttle-done end", errorShellOutcome(errors.New("summary.md malformed"), false), removal, "the run's outcome after that turn end: error (summary.md malformed)"},
		{"asking, died or timeout error", errorShellOutcome(errors.New("master died"), true), reclaim, "the run's outcome after that turn end: error (master died)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			if err := writeBackgroundShellFrictionNote(dir, []string{"sleep 9999"}, 15, tc.outcome); err != nil {
				t.Fatalf("writeBackgroundShellFrictionNote() error = %v", err)
			}
			note, err := os.ReadFile(filepath.Join(dir, "webster-background-shell.md"))
			if err != nil {
				t.Fatalf("read note: %v", err)
			}
			warning := expiredShellWarning("sleep 9999", 15, tc.outcome)

			if !strings.Contains(string(note), "- "+warning+"\n") {
				t.Errorf("note = %q; want the warning %q as a bullet", note, warning)
			}
			for _, want := range []string{
				"`sleep 9999`",
				"`background_shell_wait_min` (15 minutes)",
				"counted Master's turn end",
				"lyx did not stop the shell",
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
