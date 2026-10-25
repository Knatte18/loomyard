//go:build tier0

// skiplist_test.go — tier 0 tests for the skip list a compare is told.

package driftengine

import (
	"slices"
	"testing"
)

// TestSkipList covers off and full scope with no acceptance, partial scope's skips, added acceptances, a withdrawn test and a test both skipped and accepted.
func TestSkipList(t *testing.T) {
	t.Parallel()

	accepted := func(tests ...string) []Acceptance {
		var list []Acceptance
		for _, test := range tests {
			list = append(list, Acceptance{Test: test})
		}
		return list
	}
	tests := []struct {
		name  string
		scope ScopeRecord
		want  []string
	}{
		{"off with no acceptance", ScopeRecord{Scope: ScopeOff}, nil},
		{"full with no acceptance", ScopeRecord{Scope: ScopeFull}, nil},
		{"partial gives its skips", ScopeRecord{Scope: ScopePartial, Skip: []string{"TestA", "TestB"}}, []string{"TestA", "TestB"}},
		{"acceptances are added after the skips", ScopeRecord{Scope: ScopePartial, Skip: []string{"TestA"}, Accepted: accepted("TestC", "TestD")}, []string{"TestA", "TestC", "TestD"}},
		{"a withdrawn test is absent", ScopeRecord{Scope: ScopeFull, Accepted: accepted("TestC"), Withdrawn: []string{"TestD"}}, []string{"TestC"}},
		{"a test both skipped and accepted appears once", ScopeRecord{Scope: ScopePartial, Skip: []string{"TestA"}, Accepted: accepted("TestA")}, []string{"TestA"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := SkipList(tt.scope); !slices.Equal(got, tt.want) {
				t.Errorf("SkipList() = %v; want %v", got, tt.want)
			}
		})
	}
}
