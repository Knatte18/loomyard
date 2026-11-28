// dobbyautonomystamps_test.go pins when a refused run's episode lets the refusal be posted again.
// It is in package beamcli, since no hub can be made to fail a read or a post between two stamps.

package beamcli

import (
	"errors"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestRunAutonomyStamps_PostsARefusalOncePerEpisode feeds one run's reads through the refusal episode in order.
func TestRunAutonomyStamps_PostsARefusalOncePerEpisode(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	refused := errors.New("refused value")
	type read struct {
		refusal   error
		succeeded bool
		postFails bool
	}
	tests := []struct {
		name      string
		reads     []read
		wantPosts int
	}{
		{"RepeatedRefusal", []read{{refusal: refused}, {refusal: refused}}, 1},
		{"FailureWithNoRefusalKeepsTheEpisode", []read{{refusal: refused}, {}, {refusal: refused}}, 1},
		{"SuccessfulReadEndsTheEpisode", []read{{refusal: refused}, {succeeded: true}, {refusal: refused}}, 2},
		{"FailedPostIsTriedAgain", []read{{refusal: refused, postFails: true}, {refusal: refused}, {refusal: refused}}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			posts, postFails := 0, false
			stamps := &runAutonomyStamps{post: func(string) error {
				posts++
				if postFails {
					return errors.New("no orch")
				}
				return nil
			}}
			for _, r := range tt.reads {
				postFails = r.postFails
				stamps.tellRefusal("task1", r.refusal, r.succeeded)
			}
			if posts != tt.wantPosts {
				t.Errorf("posts after %+v = %d; want %d", tt.reads, posts, tt.wantPosts)
			}
		})
	}
}
