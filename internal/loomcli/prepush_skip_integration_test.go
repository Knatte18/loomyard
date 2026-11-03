// prepush_skip_integration_test.go drives the skip closure newPrePushCheck builds against real hub pairs, which prepush_test.go's stub seams cannot reach.

package loomcli

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestNewPrePushCheckSkip asserts the skip closure newPrePushCheck builds reads its environment and location as skipsCheck expects:
// pushes off skip the check, the hub's prime skips it, a pair at Finalize skips it, and a pair with no pause and no status does not.
//
// It is serial because it sets environment variables.
func TestNewPrePushCheckSkip(t *testing.T) {
	tiergate.Require(t, 1)
	hub := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPair(t, hub, "task")
	prime, err := lymcwd.ResolveWorktree(hub.PrimeWorktree())
	if err != nil {
		t.Fatalf("ResolveWorktree(prime) error = %v; want nil", err)
	}
	pair, err := lymcwd.ResolveWorktree(hub.PairCodeWorktree("task"))
	if err != nil {
		t.Fatalf("ResolveWorktree(pair) error = %v; want nil", err)
	}
	hubforge.AddPair(t, hub, "finalizing")
	finalizing, err := lymcwd.ResolveWorktree(hub.PairCodeWorktree("finalizing"))
	if err != nil {
		t.Fatalf("ResolveWorktree(finalizing) error = %v; want nil", err)
	}
	writeStatusFile(t, finalizing, `{"current_producer":"`+loomshed.NameFinalize+`","state":"running"}`)
	tests := []struct {
		name     string
		skipPush string
		location *lymcwd.Location
		want     bool
	}{
		{name: "PushesOff", skipPush: "1", location: pair, want: true},
		{name: "Prime", location: prime, want: true},
		{name: "PairWithNoPauseAndNoStatus", location: pair},
		{name: "PairAtFinalize", location: finalizing, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("FABRIC_SKIP_GIT", "")
			t.Setenv("FABRIC_SKIP_PUSH", tt.skipPush)

			got, err := newPrePushCheck(tt.location, shedrun.SelfRunID).skip()

			if err != nil || got != tt.want {
				t.Errorf("skip() = %t, %v; want %t, nil", got, err, tt.want)
			}
		})
	}
}
