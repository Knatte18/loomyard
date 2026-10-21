package discussionparser

import (
	"strings"
	"testing"
)

func TestIdentityText(t *testing.T) {
	t.Parallel()
	base := recordWithOpenRisks("\nrisk text.\n\n")

	tests := []struct {
		name      string
		change    func(t *testing.T, base string) string
		wantEqual bool
	}{
		{
			name: "CarryOverBlockWrittenByWriteCarryOver",
			change: func(t *testing.T, base string) string {
				path := writeRecord(t, base)
				if err := WriteCarryOver(path, carryOverEntry("Plan-Review", "cache-key")); err != nil {
					t.Fatalf("WriteCarryOver() error = %v; want nil", err)
				}
				return readFile(t, path)
			},
			wantEqual: true,
		},
		{
			name: "AddedDecisionWrittenByAppendDecision",
			change: func(t *testing.T, base string) string {
				decisionPath, supportPath := writeFixture(t, t.TempDir(), base, "log")
				if findings, err := AppendDecision(decisionPath, supportPath, addedDecision()); err != nil || len(findings) != 0 {
					t.Fatalf("AppendDecision() = %v, %v; want no findings and nil", findings, err)
				}
				return readFile(t, decisionPath)
			},
			wantEqual: true,
		},
		{
			name: "TwoAddedDecisionsAndACarryOverBlock",
			change: func(t *testing.T, base string) string {
				decisionPath, supportPath := writeFixture(t, t.TempDir(), base, "log")
				second := addedDecision()
				second.Title = "Drop the cache"
				for _, d := range []AddedDecision{addedDecision(), second} {
					if findings, err := AppendDecision(decisionPath, supportPath, d); err != nil || len(findings) != 0 {
						t.Fatalf("AppendDecision() = %v, %v; want no findings and nil", findings, err)
					}
				}
				if err := WriteCarryOver(decisionPath, carryOverEntry("Plan-Review", "cache-key")); err != nil {
					t.Fatalf("WriteCarryOver() error = %v; want nil", err)
				}
				return readFile(t, decisionPath)
			},
			wantEqual: true,
		},
		{
			name: "DecisionProseEdited",
			change: func(t *testing.T, base string) string {
				return strings.Replace(base, "## Decisions\n\nbody text.", "## Decisions\n\nbody text, edited.", 1)
			},
		},
		{
			name: "OpenRisksProseEdited",
			change: func(t *testing.T, base string) string {
				return strings.Replace(base, "risk text.", "risk text, edited.", 1)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			changed := tt.change(t, base)
			got := string(IdentityText([]byte(changed)))
			want := string(IdentityText([]byte(base)))
			if tt.wantEqual && got != want {
				t.Errorf("IdentityText(changed) mismatch:\n got %q\nwant %q", got, want)
			}
			if !tt.wantEqual && got == want {
				t.Errorf("IdentityText(changed) = IdentityText(base); want a different text")
			}
		})
	}
}
