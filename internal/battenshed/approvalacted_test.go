package battenshed

import "testing"

func TestParseDecisionActed(t *testing.T) {
	id := decisionIdentity(ChildDecision{Kind: DecisionApprove, At: "2026-01-02T03:04:05Z", HeadSHA: "abc123"})
	tests := []struct {
		name    string
		raw     string
		wantID  string
		wantLen int
		wantHas bool
	}{
		{"round trip zero", decisionActedContent(id, 0), id, 0, true},
		{"round trip multi-digit", decisionActedContent(id, 12345), id, 12345, true},
		{"old one-line layout", id, id, 0, false},
		{"non-numeric second line", id + "abc\n", id, 0, false},
		{"negative second line", id + "-3\n", id, 0, false},
		{"length without trailing newline", id + "12", id, 12, true},
		{"no newline", "abc", "abc", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotID, gotLen, gotHas := parseDecisionActed(tt.raw)
			if gotID != tt.wantID || gotLen != tt.wantLen || gotHas != tt.wantHas {
				t.Errorf("parseDecisionActed(%q) = (%q, %d, %v); want (%q, %d, %v)",
					tt.raw, gotID, gotLen, gotHas, tt.wantID, tt.wantLen, tt.wantHas)
			}
		})
	}
}
