// name_test.go verifies newGUID produces unique, well-formed hex identifiers.

package reedengine

import (
	"strings"
	"testing"
)

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
