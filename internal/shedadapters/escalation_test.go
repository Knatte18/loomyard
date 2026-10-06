// escalation_test.go covers the escalation record over a temp run directory.

package shedadapters

import (
	"errors"
	"io/fs"
	"os"
	"testing"
)

//testtiming:keep pins the escalation record's round trip: cause, notice, brief body, an empty brief, and no notice file for an empty notice
func TestEscalation_RoundTrip(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		cause  EscalationCause
		brief  string
		notice string
		// wantBody is the brief after the frontmatter is stripped.
		wantBody string
	}{
		{"circling", EscalationCircling, "the brief body\n", "parent: read the brief", "the brief body"},
		{"budget", EscalationBudget, "the brief body\n", "parent: read the brief", "the brief body"},
		{"empty brief stays readable", EscalationBudget, "", "notice", ""},
		{"empty notice writes no notice file", EscalationCircling, "brief", "", "brief"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			if err := writeEscalation(dir, 3, tt.cause, tt.brief, tt.notice); err != nil {
				t.Fatalf("writeEscalation(%q) = %v; want nil", tt.cause, err)
			}

			gotCause, gotNotice, exists, err := readEscalation(dir, 3)
			if err != nil {
				t.Fatalf("readEscalation(%q) error = %v; want nil", tt.cause, err)
			}
			if !exists {
				t.Errorf("readEscalation(%q) exists = false; want true", tt.cause)
			}
			if gotCause != tt.cause {
				t.Errorf("readEscalation cause = %q; want %q", gotCause, tt.cause)
			}
			if gotNotice != tt.notice {
				t.Errorf("readEscalation notice = %q; want %q", gotNotice, tt.notice)
			}

			raw, err := os.ReadFile(escalationPath(dir, 3))
			if err != nil {
				t.Fatalf("ReadFile(escalation) = %v; want nil", err)
			}
			if got := frontmatterProse(raw); got != tt.wantBody {
				t.Errorf("escalation body = %q; want %q", got, tt.wantBody)
			}
			if tt.notice == "" {
				if _, err := os.Stat(parentNoticePath(dir, 3)); !errors.Is(err, fs.ErrNotExist) {
					t.Errorf("Stat(parent notice) error = %v; want fs.ErrNotExist", err)
				}
			}
		})
	}
}

//testtiming:keep pins that an absent escalation record reads as absent with no error
func TestEscalation_AbsentRecord(t *testing.T) {
	cause, notice, exists, err := readEscalation(t.TempDir(), 1)
	if err != nil {
		t.Fatalf("readEscalation error = %v; want nil", err)
	}
	if exists || cause != "" || notice != "" {
		t.Errorf("readEscalation = (%q, %q, %v); want (\"\", \"\", false)", cause, notice, exists)
	}
}

func TestEscalation_MalformedRecordIsAnError(t *testing.T) {
	cases := map[string]string{
		"round mismatch": "---\nround: 9\ncause: circling\n---\nbrief\n",
		"unknown cause":  "---\nround: 4\ncause: mystery\n---\nbrief\n",
		"no frontmatter": "brief only\n",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(escalationPath(dir, 4), []byte(content), 0o644); err != nil {
				t.Fatalf("WriteFile = %v; want nil", err)
			}
			_, _, exists, err := readEscalation(dir, 4)
			if !errors.Is(err, ErrEscalationMalformed) {
				t.Fatalf("readEscalation error = %v; want it to wrap ErrEscalationMalformed", err)
			}
			if !exists {
				t.Error("readEscalation exists = false; want true for a present malformed record")
			}
		})
	}
}

func TestWriteEscalation_RejectsUnknownCause(t *testing.T) {
	if err := writeEscalation(t.TempDir(), 1, EscalationCause("mystery"), "b", "n"); err == nil {
		t.Error("writeEscalation(unknown cause) = nil; want an error")
	}
}
