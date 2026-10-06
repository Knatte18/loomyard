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

//testtiming:keep pins the decision file RecordCirclingDecision writes per decision, escalation record and cause, and its round and cause return values
func TestRecordCirclingDecision_WritesPendingFile(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		decision  CirclingDecision
		layout    func(t *testing.T, dir string)
		wantRound int
		wantCause EscalationCause
	}{
		{
			name:     "accept over a circling verdict",
			decision: CirclingAccept,
			layout: func(t *testing.T, dir string) {
				layoutCirclingRun(t, dir, 1, "CONTINUE")
				layoutCirclingRun(t, dir, 2, "CIRCLING")
			},
			wantRound: 2,
			wantCause: EscalationCircling,
		},
		{
			name:     "continue over a circling verdict",
			decision: CirclingContinue,
			layout: func(t *testing.T, dir string) {
				layoutCirclingRun(t, dir, 1, "CONTINUE")
				layoutCirclingRun(t, dir, 2, "CIRCLING")
			},
			wantRound: 2,
			wantCause: EscalationCircling,
		},
		{
			name:     "accept over a budget escalation on a continue verdict",
			decision: CirclingAccept,
			layout: func(t *testing.T, dir string) {
				layoutCirclingRun(t, dir, 1, "CONTINUE")
				writeEscalationRecord(t, dir, EscalationBudget)
			},
			wantRound: 1,
			wantCause: EscalationBudget,
		},
		{
			name:     "continue over a budget escalation on a continue verdict",
			decision: CirclingContinue,
			layout: func(t *testing.T, dir string) {
				layoutCirclingRun(t, dir, 1, "CONTINUE")
				writeEscalationRecord(t, dir, EscalationBudget)
			},
			wantRound: 1,
			wantCause: EscalationBudget,
		},
		{
			name:     "continue over a circling escalation record",
			decision: CirclingContinue,
			layout: func(t *testing.T, dir string) {
				layoutCirclingRun(t, dir, 1, "CIRCLING")
				writeEscalationRecord(t, dir, EscalationCircling)
			},
			wantRound: 1,
			wantCause: EscalationCircling,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			tt.layout(t, dir)

			round, cause, err := RecordCirclingDecision(dir, tt.decision)
			if err != nil || round != tt.wantRound || cause != tt.wantCause {
				t.Fatalf("RecordCirclingDecision(%q) = (%d, %q, %v); want (%d, %q, nil)", tt.decision, round, cause, err, tt.wantRound, tt.wantCause)
			}
			got, gotCause, settled, exists, err := readCirclingDecision(dir, tt.wantRound)
			if err != nil || !exists || settled || got != tt.decision || gotCause != tt.wantCause {
				t.Errorf("readCirclingDecision = (%q, %q, settled %v, exists %v, %v); want (%q, %q, false, true, nil)", got, gotCause, settled, exists, err, tt.decision, tt.wantCause)
			}
		})
	}
}

// writeEscalationRecord writes round 1's escalation record with the given cause.
func writeEscalationRecord(t *testing.T, dir string, cause EscalationCause) {
	t.Helper()
	if err := writeEscalation(dir, 1, cause, "brief\n", "notice"); err != nil {
		t.Fatalf("writeEscalation error = %v; want nil", err)
	}
}

func TestRecordCirclingDecision_SecondRecordIsRefused(t *testing.T) {
	dir := t.TempDir()
	layoutCirclingRun(t, dir, 1, "CIRCLING")
	if _, _, err := RecordCirclingDecision(dir, CirclingAccept); err != nil {
		t.Fatalf("first RecordCirclingDecision error = %v; want nil", err)
	}
	before, err := os.ReadFile(circlingDecisionPath(dir, 1))
	if err != nil {
		t.Fatalf("ReadFile = %v; want nil", err)
	}

	if _, _, err := RecordCirclingDecision(dir, CirclingContinue); !errors.Is(err, ErrCirclingDecided) {
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

func TestRecordCirclingDecision_NotEscalatedWritesNothing(t *testing.T) {
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
			if _, _, err := RecordCirclingDecision(dir, CirclingAccept); !errors.Is(err, ErrNotEscalated) {
				t.Errorf("RecordCirclingDecision error = %v; want ErrNotEscalated", err)
			}
			for round := 1; round <= 2; round++ {
				if _, err := os.Stat(circlingDecisionPath(dir, round)); !errors.Is(err, os.ErrNotExist) {
					t.Errorf("round %d decision file stat error = %v; want not-exist", round, err)
				}
			}
		})
	}
}

func TestReadCirclingDecision(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		// content is the decision file's content; empty leaves the file absent.
		content      string
		wantExists   bool
		wantErr      bool
		wantDecision CirclingDecision
		wantCause    EscalationCause
	}{
		{name: "absent file"},
		{
			name:         "a legacy file without a cause reads as circling",
			content:      "---\nround: 1\ndecision: accept\nsettled: false\n---\n",
			wantExists:   true,
			wantDecision: CirclingAccept,
			wantCause:    EscalationCircling,
		},
		{name: "round disagrees with filename", content: "---\nround: 2\ndecision: accept\nsettled: false\n---\n", wantExists: true, wantErr: true},
		{name: "unknown decision", content: "---\nround: 1\ndecision: maybe\nsettled: false\n---\n", wantExists: true, wantErr: true},
		{name: "unknown cause", content: "---\nround: 1\ndecision: accept\ncause: weather\nsettled: false\n---\n", wantExists: true, wantErr: true},
		{name: "settled continue", content: "---\nround: 1\ndecision: continue\nsettled: true\n---\n", wantExists: true, wantErr: true},
		{name: "no frontmatter", content: "accept\n", wantExists: true, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			if tt.content != "" {
				if err := os.WriteFile(circlingDecisionPath(dir, 1), []byte(tt.content), 0o644); err != nil {
					t.Fatalf("WriteFile = %v; want nil", err)
				}
			}

			got, cause, _, exists, err := readCirclingDecision(dir, 1)
			if (err != nil) != tt.wantErr || exists != tt.wantExists {
				t.Fatalf("readCirclingDecision = (exists %v, %v); want (exists %v, error %v)", exists, err, tt.wantExists, tt.wantErr)
			}
			if got != tt.wantDecision || cause != tt.wantCause {
				t.Errorf("readCirclingDecision = (%q, %q); want (%q, %q)", got, cause, tt.wantDecision, tt.wantCause)
			}
		})
	}
}
