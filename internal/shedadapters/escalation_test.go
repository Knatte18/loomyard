// escalation_test.go covers the escalation record over a temp run directory.

package shedadapters

import (
	"errors"
	"io/fs"
	"os"
	"testing"
)

func TestEscalation_RoundTrip(t *testing.T) {
	for _, cause := range []EscalationCause{EscalationCircling, EscalationBudget} {
		dir := t.TempDir()
		if err := writeEscalation(dir, 3, cause, "the brief body\n", "parent: read the brief"); err != nil {
			t.Fatalf("writeEscalation(%q) = %v; want nil", cause, err)
		}

		gotCause, gotNotice, exists, err := readEscalation(dir, 3)
		if err != nil {
			t.Fatalf("readEscalation(%q) error = %v; want nil", cause, err)
		}
		if !exists {
			t.Errorf("readEscalation(%q) exists = false; want true", cause)
		}
		if gotCause != cause {
			t.Errorf("readEscalation cause = %q; want %q", gotCause, cause)
		}
		if gotNotice != "parent: read the brief" {
			t.Errorf("readEscalation notice = %q; want %q", gotNotice, "parent: read the brief")
		}

		raw, err := os.ReadFile(escalationPath(dir, 3))
		if err != nil {
			t.Fatalf("ReadFile(escalation) = %v; want nil", err)
		}
		if got := frontmatterProse(raw); got != "the brief body" {
			t.Errorf("escalation body = %q; want %q", got, "the brief body")
		}
	}
}

func TestEscalation_EmptyBriefStillReadable(t *testing.T) {
	dir := t.TempDir()
	if err := writeEscalation(dir, 2, EscalationBudget, "", "notice"); err != nil {
		t.Fatalf("writeEscalation = %v; want nil", err)
	}
	cause, _, exists, err := readEscalation(dir, 2)
	if err != nil {
		t.Fatalf("readEscalation error = %v; want nil", err)
	}
	if !exists || cause != EscalationBudget {
		t.Errorf("readEscalation = (%q, exists %v); want (%q, true)", cause, exists, EscalationBudget)
	}
}

func TestEscalation_EmptyNoticeWritesNoNoticeFile(t *testing.T) {
	dir := t.TempDir()
	if err := writeEscalation(dir, 1, EscalationCircling, "brief", ""); err != nil {
		t.Fatalf("writeEscalation = %v; want nil", err)
	}
	if _, err := os.Stat(parentNoticePath(dir, 1)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Stat(parent notice) error = %v; want fs.ErrNotExist", err)
	}
	_, notice, exists, err := readEscalation(dir, 1)
	if err != nil || !exists || notice != "" {
		t.Errorf("readEscalation = (notice %q, exists %v, err %v); want (\"\", true, nil)", notice, exists, err)
	}
}

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
			if err == nil {
				t.Fatal("readEscalation error = nil; want an error")
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
