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
		{"a call holding --wait", "lym gate test --wait --card 3", true},
		{"a call with --wait after targets", "lym gate test ./internal/foo --wait", true},
		{"a call without --wait", "lym gate test --card 3", false},
		{"--wait only after the double dash", "lym gate test ./... -- --wait", false},
		{"a chained call", "lym gate test --wait ./a && echo done", false},
		{"a piped call", "lym gate test --wait ./a | tee out", false},
		{"a command substitution", "lym gate test --wait $(pwd)", false},
		{"another lym verb", "lym board list --wait", false},
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
