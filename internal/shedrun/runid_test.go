package shedrun

import "testing"

func TestValidateRunID(t *testing.T) {
	tests := []struct {
		name    string
		runID   string
		wantErr bool
	}{
		{"dotdot", "..", true},
		{"nested_slash", "a/b", true},
		{"absolute", "/abs", true},
		{"empty", "", true},
		{"self", "self", false},
		{"ordinary_slug", "seeded-shed-core", false},
		{"dot", ".", true},
		{"backslash", `a\b`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRunID(tt.runID)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateRunID(%q) = %v; want error = %v", tt.runID, err, tt.wantErr)
			}
		})
	}
}

func TestIsReserved(t *testing.T) {
	tests := []struct {
		name  string
		runID string
		want  bool
	}{
		{"dotdot", "..", false},
		{"nested_slash", "a/b", false},
		{"absolute", "/abs", false},
		{"empty", "", false},
		{"self", "self", true},
		{"ordinary_slug", "seeded-shed-core", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsReserved(tt.runID); got != tt.want {
				t.Errorf("IsReserved(%q) = %v; want %v", tt.runID, got, tt.want)
			}
		})
	}
}
