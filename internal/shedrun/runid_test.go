package shedrun

import "testing"

// TestRunIDVocabulary pins which run-ids are valid and which one is reserved.
func TestRunIDVocabulary(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		runID        string
		wantInvalid  bool
		wantReserved bool
	}{
		{"dotdot", "..", true, false},
		{"nested_slash", "a/b", true, false},
		{"absolute", "/abs", true, false},
		{"empty", "", true, false},
		{"self", "self", false, true},
		{"ordinary_slug", "seeded-shed-core", false, false},
		{"dot", ".", true, false},
		{"backslash", `a\b`, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := ValidateRunID(tt.runID); (err != nil) != tt.wantInvalid {
				t.Errorf("ValidateRunID(%q) = %v; want error = %v", tt.runID, err, tt.wantInvalid)
			}
			if got := IsReserved(tt.runID); got != tt.wantReserved {
				t.Errorf("IsReserved(%q) = %v; want %v", tt.runID, got, tt.wantReserved)
			}
		})
	}
}
