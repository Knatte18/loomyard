// inflight_test.go covers which live reed sessions count as another pair's when a pair is removed.

package pairteardown

import (
	"slices"
	"testing"
)

func TestSessionsOfOtherPairs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		live         []string
		pairSessions []string
		want         []string
	}{
		{name: "no live sessions", pairSessions: []string{"pair-a"}},
		{name: "no other pairs", live: []string{"prime", "orch"}},
		{
			name:         "the prime's own sessions never count",
			live:         []string{"prime", "pair-a", "orch"},
			pairSessions: []string{"pair-a", "pair-b"},
			want:         []string{"pair-a"},
		},
		{
			name:         "live order is kept",
			live:         []string{"pair-b", "pair-a"},
			pairSessions: []string{"pair-a", "pair-b"},
			want:         []string{"pair-b", "pair-a"},
		},
		{
			name:         "a pair without a live session is absent",
			live:         []string{"pair-a"},
			pairSessions: []string{"pair-a", "pair-b"},
			want:         []string{"pair-a"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := sessionsOfOtherPairs(tt.live, tt.pairSessions); !slices.Equal(got, tt.want) {
				t.Errorf("sessionsOfOtherPairs(%v, %v) = %v; want %v", tt.live, tt.pairSessions, got, tt.want)
			}
		})
	}
}
