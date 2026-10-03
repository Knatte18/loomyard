// circling_test.go covers the circling decision record over a temp run directory.

package shedadapters

import (
	"bytes"
	"errors"
	"os"
	"testing"
)

// layoutCirclingRun writes a review, verdict and ledger for round with the given verdict word.
func layoutCirclingRun(t *testing.T, dir string, round int, verdict string) {
	t.Helper()
	if err := os.WriteFile(roundReviewPath(dir, round), []byte("review\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(review) = %v; want nil", err)
	}
	if verdict == "" {
		return
	}
	if err := os.WriteFile(verdictPath(dir, round), []byte(bouncerVerdictContent(verdict)), 0o644); err != nil {
		t.Fatalf("WriteFile(verdict) = %v; want nil", err)
	}
	if err := os.WriteFile(ledgerPath(dir, round), []byte(bouncerLedgerContent(round)), 0o644); err != nil {
		t.Fatalf("WriteFile(ledger) = %v; want nil", err)
	}
}

func TestRecordCirclingDecision_WritesPendingFile(t *testing.T) {
	for _, d := range []CirclingDecision{CirclingAccept, CirclingContinue} {
		dir := t.TempDir()
		layoutCirclingRun(t, dir, 1, "CONTINUE")
		layoutCirclingRun(t, dir, 2, "CIRCLING")

		round, err := RecordCirclingDecision(dir, d)
		if err != nil {
			t.Fatalf("RecordCirclingDecision(%q) error = %v; want nil", d, err)
		}
		if round != 2 {
			t.Errorf("RecordCirclingDecision(%q) round = %d; want 2", d, round)
		}
		got, settled, exists, err := readCirclingDecision(dir, 2)
		if err != nil || !exists || settled || got != d {
			t.Errorf("readCirclingDecision = (%q, settled %v, exists %v, %v); want (%q, false, true, nil)", got, settled, exists, err, d)
		}
	}
}

func TestRecordCirclingDecision_SecondRecordIsRefused(t *testing.T) {
	dir := t.TempDir()
	layoutCirclingRun(t, dir, 1, "CIRCLING")
	if _, err := RecordCirclingDecision(dir, CirclingAccept); err != nil {
		t.Fatalf("first RecordCirclingDecision error = %v; want nil", err)
	}
	before, err := os.ReadFile(circlingDecisionPath(dir, 1))
	if err != nil {
		t.Fatalf("ReadFile = %v; want nil", err)
	}

	if _, err := RecordCirclingDecision(dir, CirclingContinue); !errors.Is(err, ErrCirclingDecided) {
		t.Errorf("second RecordCirclingDecision error = %v; want ErrCirclingDecided", err)
	}
	after, err := os.ReadFile(circlingDecisionPath(dir, 1))
	if err != nil {
		t.Fatalf("ReadFile = %v; want nil", err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("decision file changed: %q -> %q", before, after)
	}
}

func TestRecordCirclingDecision_NotCirclingWritesNothing(t *testing.T) {
	tests := []struct {
		name   string
		layout func(t *testing.T, dir string)
	}{
		{"converged", func(t *testing.T, dir string) { layoutCirclingRun(t, dir, 1, "CONVERGED") }},
		{"continue", func(t *testing.T, dir string) { layoutCirclingRun(t, dir, 1, "CONTINUE") }},
		{"unjudged", func(t *testing.T, dir string) { layoutCirclingRun(t, dir, 1, "") }},
		{"earlier circling, later continue", func(t *testing.T, dir string) {
			layoutCirclingRun(t, dir, 1, "CIRCLING")
			layoutCirclingRun(t, dir, 2, "CONTINUE")
		}},
		{"empty", func(t *testing.T, dir string) {}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			tt.layout(t, dir)
			if _, err := RecordCirclingDecision(dir, CirclingAccept); !errors.Is(err, ErrNotCircling) {
				t.Errorf("RecordCirclingDecision error = %v; want ErrNotCircling", err)
			}
			for round := 1; round <= 2; round++ {
				if _, err := os.Stat(circlingDecisionPath(dir, round)); !errors.Is(err, os.ErrNotExist) {
					t.Errorf("round %d decision file stat error = %v; want not-exist", round, err)
				}
			}
		})
	}
}

func TestSettleCirclingAccept_SettlesPendingAccept(t *testing.T) {
	dir := t.TempDir()
	layoutCirclingRun(t, dir, 1, "CIRCLING")
	if _, err := RecordCirclingDecision(dir, CirclingAccept); err != nil {
		t.Fatalf("RecordCirclingDecision error = %v; want nil", err)
	}
	if err := settleCirclingAccept(dir, 1); err != nil {
		t.Fatalf("settleCirclingAccept error = %v; want nil", err)
	}
	got, settled, exists, err := readCirclingDecision(dir, 1)
	if err != nil || !exists || !settled || got != CirclingAccept {
		t.Errorf("readCirclingDecision = (%q, settled %v, exists %v, %v); want (accept, true, true, nil)", got, settled, exists, err)
	}
}

func TestReadCirclingDecision_AbsentFile(t *testing.T) {
	_, _, exists, err := readCirclingDecision(t.TempDir(), 1)
	if err != nil || exists {
		t.Errorf("readCirclingDecision(absent) = (exists %v, %v); want (false, nil)", exists, err)
	}
}

func TestReadCirclingDecision_MalformedIsAnError(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"round disagrees with filename", "---\nround: 2\ndecision: accept\nsettled: false\n---\n"},
		{"unknown decision", "---\nround: 1\ndecision: maybe\nsettled: false\n---\n"},
		{"settled continue", "---\nround: 1\ndecision: continue\nsettled: true\n---\n"},
		{"no frontmatter", "accept\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(circlingDecisionPath(dir, 1), []byte(tt.content), 0o644); err != nil {
				t.Fatalf("WriteFile = %v; want nil", err)
			}
			if _, _, exists, err := readCirclingDecision(dir, 1); err == nil || !exists {
				t.Errorf("readCirclingDecision = (exists %v, %v); want a present-file error", exists, err)
			}
		})
	}
}
