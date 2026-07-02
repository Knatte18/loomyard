// name_test.go table-tests FormatStrandName's token substitution and
// verifies newGUID produces unique, well-formed hex identifiers.

package muxengine

import (
	"strings"
	"testing"
)

func TestFormatStrandName(t *testing.T) {
	tests := []struct {
		name     string
		template string
		parts    map[string]string
		want     string
	}{
		{
			name:     "all tokens filled in template order",
			template: "<ROLE>:<ROUND>:<SHORT_GUID>",
			parts:    map[string]string{"<ROLE>": "main", "<ROUND>": "1", "<SHORT_GUID>": "abc12345"},
			want:     "main:1:abc12345",
		},
		{
			name:     "tokens reordered",
			template: "<SHORT_GUID>-<ROLE>",
			parts:    map[string]string{"<ROLE>": "review", "<SHORT_GUID>": "deadbeef"},
			want:     "deadbeef-review",
		},
		{
			name:     "unfilled token resolves to empty string",
			template: "<ROLE>:<ROUND>:<SHORT_GUID>",
			parts:    map[string]string{"<ROLE>": "main"},
			want:     "main::",
		},
		{
			name:     "short_guid fallback: template is bare token",
			template: "<SHORT_GUID>",
			parts:    map[string]string{"<SHORT_GUID>": "cafebabe"},
			want:     "cafebabe",
		},
		{
			name:     "worktree token substitutes",
			template: "<WORKTREE>/<ROLE>",
			parts:    map[string]string{"<WORKTREE>": "internal-mux", "<ROLE>": "main"},
			want:     "internal-mux/main",
		},
		{
			name:     "override: repeated token substitutes every occurrence",
			template: "<ROLE>-<ROLE>",
			parts:    map[string]string{"<ROLE>": "x"},
			want:     "x-x",
		},
		{
			name:     "no tokens in template",
			template: "static-name",
			parts:    map[string]string{"<ROLE>": "main"},
			want:     "static-name",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatStrandName(tt.template, tt.parts)
			if got != tt.want {
				t.Errorf("FormatStrandName(%q, %v) = %q, want %q", tt.template, tt.parts, got, tt.want)
			}
		})
	}
}

func TestNewGUID_UniqueAndHex(t *testing.T) {
	const n = 50
	seen := make(map[string]bool, n)
	for i := 0; i < n; i++ {
		guid, err := newGUID()
		if err != nil {
			t.Fatalf("newGUID: %v", err)
		}
		if len(guid) != 32 {
			t.Fatalf("newGUID() = %q, want 32 hex chars (128 bits)", guid)
		}
		if strings.ToLower(guid) != guid {
			t.Errorf("newGUID() = %q, want lowercase hex", guid)
		}
		for _, c := range guid {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				t.Fatalf("newGUID() = %q has non-hex char %c", guid, c)
			}
		}
		if seen[guid] {
			t.Fatalf("newGUID() produced duplicate %q across %d calls", guid, n)
		}
		seen[guid] = true
	}
}
