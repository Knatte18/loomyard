//go:build tier0

// gatecall_test.go covers IsGateCall, the check that picks a line the runner leaves unslotted and unbounded.

package cardverify

import (
	"testing"
)

func TestIsGateCall(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		line string
		want bool
	}{
		{"a call holding --wait", "lyx gate test --wait --card 3", true},
		{"a call with --wait after targets", "lyx gate test ./internal/foo --wait", true},
		{"a call without --wait", "lyx gate test --card 3", false},
		{"--wait only after the double dash", "lyx gate test ./... -- --wait", false},
		{"a chained call", "lyx gate test --wait ./a && echo done", false},
		{"a piped call", "lyx gate test --wait ./a | tee out", false},
		{"a command substitution", "lyx gate test --wait $(pwd)", false},
		{"another lyx verb", "lyx board list --wait", false},
		{"a plain shell line", "go build ./...", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsGateCall(tt.line); got != tt.want {
				t.Errorf("IsGateCall(%q) = %t; want %t", tt.line, got, tt.want)
			}
		})
	}
}
