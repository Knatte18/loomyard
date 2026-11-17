// memoryevents_linux_test.go covers how a scope's memory.events content tells a kill at the scope's own limit.

package proc_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestKilledAtOwnLimit(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name   string
		events string
		want   bool
	}{
		{"AKillAtTheScopesOwnLimit", "low 0\nhigh 0\nmax 3\noom 1\noom_kill 1\noom_group_kill 0\n", true},
		{"AKillAtAnEnclosingLimitRaisesNoOOMCountOnTheScope", "max 0\noom 0\noom_kill 1\noom_group_kill 0\n", false},
		{"NoKill", "max 0\noom 0\noom_kill 0\n", false},
		{"AnOOMWithNoKill", "max 4\noom 1\noom_kill 0\n", false},
		{"NoEventsRead", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := proc.KilledAtOwnLimit(tt.events); got != tt.want {
				t.Errorf("KilledAtOwnLimit(%q) = %v; want %v", tt.events, got, tt.want)
			}
		})
	}
}
