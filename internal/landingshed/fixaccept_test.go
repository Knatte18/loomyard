//go:build tier0

// fixaccept_test.go pins AcceptFixReview: what it writes into the change description and the attempt record, and what it refuses.

package landingshed

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAcceptFixReview(t *testing.T) {
	t.Parallel()
	const description = "# Fix the widget\n\nThe widget now spins.\n"
	const section = "## Unresolved fix review findings\n\n" +
		"Finalize's fix review did not approve the merge fixer's change since t2 within its rounds, and the operator accepted the change as it stands with these findings open:\n\n" +
		"- f1 loosens the assertion\n" +
		"- f2 skips a test\n"
	const wantAccepted = description + "\n" + section
	capped := FixerAttempts{Entry: 1, Spent: 1, Baseline: "t2", Halt: fixerHaltReviewCapped, ReviewRounds: 2, Findings: []string{"f1 loosens the assertion", "f2 skips a test"}}

	tests := []struct {
		name string
		rec  FixerAttempts
		// seededDescription is the change description before the accept; empty means the file is absent.
		seededDescription string
		// calls is how many times AcceptFixReview runs over the entry.
		calls           int
		wantErr         bool
		wantErrIs       error
		wantDescription string
		wantAcceptedAt  string
	}{
		{name: "an entry halted at its round cap appends the findings and is accepted at the task HEAD", rec: capped, seededDescription: description, calls: 1, wantDescription: wantAccepted, wantAcceptedAt: "h7"},
		{name: "a second accept writes nothing more", rec: capped, seededDescription: description, calls: 2, wantDescription: wantAccepted, wantAcceptedAt: "h7"},
		{name: "a retry after a failed record write finds the section and appends nothing", rec: capped, seededDescription: wantAccepted, calls: 1, wantDescription: wantAccepted, wantAcceptedAt: "h7"},
		{name: "a missing change description is an error and the entry stays unaccepted", rec: capped, calls: 1, wantErr: true},
		{name: "an entry halted on a spent cap is refused and nothing is written", rec: FixerAttempts{Entry: 1, Spent: 2, Baseline: "t2", Halt: fixerHaltSpent}, seededDescription: description, calls: 1, wantErr: true, wantErrIs: ErrNoFixReviewHalt, wantDescription: description},
		{name: "a record of another entry is refused", rec: FixerAttempts{Entry: 0, Halt: fixerHaltReviewCapped}, seededDescription: description, calls: 1, wantErr: true, wantErrIs: ErrNoFixReviewHalt, wantDescription: description},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			scratch := t.TempDir()
			descriptionPath := filepath.Join(t.TempDir(), "summary.md")
			if tt.seededDescription != "" {
				if err := os.WriteFile(descriptionPath, []byte(tt.seededDescription), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			attemptsPath := FixerAttemptsPath(scratch, finalizeName)
			if err := WriteFixerAttempts(attemptsPath, tt.rec); err != nil {
				t.Fatal(err)
			}

			var err error
			for range tt.calls {
				err = AcceptFixReview(scratch, 1, descriptionPath, func() (string, error) { return "h7", nil })
			}

			if (err != nil) != tt.wantErr || (tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs)) {
				t.Fatalf("AcceptFixReview() error = %v; want an error %v (is %v)", err, tt.wantErr, tt.wantErrIs)
			}
			if tt.seededDescription != "" {
				got, readErr := os.ReadFile(descriptionPath)
				if readErr != nil {
					t.Fatal(readErr)
				}
				if string(got) != tt.wantDescription {
					t.Errorf("change description = %q; want %q", got, tt.wantDescription)
				}
			}
			rec, readErr := ReadFixerAttempts(attemptsPath, 1)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if rec.AcceptedHead != tt.wantAcceptedAt {
				t.Errorf("attempt record AcceptedHead = %q; want %q", rec.AcceptedHead, tt.wantAcceptedAt)
			}
		})
	}

	t.Run("a task head that cannot be read is an error and the entry stays unaccepted", func(t *testing.T) {
		t.Parallel()
		scratch := t.TempDir()
		descriptionPath := filepath.Join(t.TempDir(), "summary.md")
		if err := os.WriteFile(descriptionPath, []byte(description), 0o644); err != nil {
			t.Fatal(err)
		}
		attemptsPath := FixerAttemptsPath(scratch, finalizeName)
		if err := WriteFixerAttempts(attemptsPath, capped); err != nil {
			t.Fatal(err)
		}
		headErr := errors.New("no head")

		err := AcceptFixReview(scratch, 1, descriptionPath, func() (string, error) { return "", headErr })

		if !errors.Is(err, headErr) {
			t.Fatalf("AcceptFixReview() error = %v; want %v", err, headErr)
		}
		if got, _ := os.ReadFile(descriptionPath); string(got) != description {
			t.Errorf("change description = %q; want it untouched", got)
		}
		rec, readErr := ReadFixerAttempts(attemptsPath, 1)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if rec.AcceptedHead != "" {
			t.Errorf("attempt record AcceptedHead = %q; want none", rec.AcceptedHead)
		}
	})
}
