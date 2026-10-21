// identity_test.go tests IdentityText: the overview's identity ignores the approval line and nothing else.

package planparser

import (
	"strings"
	"testing"
)

func TestIdentityText(t *testing.T) {
	t.Parallel()

	base := "---\nformat: 5\napproved: false\nroot: internal/foo\n---\n\n# Plan: rich\n\nFraming paragraph.\n\n" +
		"## Card Index\n\n1 — only — the only card\n\n## Shared Decisions\n\n- **Decision:** a plan-level decision.\n"
	tests := []struct {
		name      string
		other     string
		wantEqual bool
	}{
		{"ApprovedTrue", strings.Replace(base, "approved: false", "approved: true", 1), true},
		{"ApprovedKeyAbsent", strings.Replace(base, "approved: false\n", "", 1), true},
		{"CardIndexLineChanged", strings.Replace(base, "the only card", "another card", 1), false},
		{"OtherFrontmatterKeyChanged", strings.Replace(base, "root: internal/foo", "root: internal/bar", 1), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := string(IdentityText([]byte(tt.other)))
			want := string(IdentityText([]byte(base)))
			if (got == want) != tt.wantEqual {
				t.Errorf("IdentityText(%s) == IdentityText(base) is %t; want %t", tt.name, got == want, tt.wantEqual)
			}
		})
	}
}
