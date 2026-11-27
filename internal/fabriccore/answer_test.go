// answer_test.go tests the message a refusal carries.

package fabriccore_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/fabriccore"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestRefusal_Error(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name    string
		refusal fabriccore.Refusal
		want    string
	}{
		{"ReasonOnly", fabriccore.Refusal{Reason: "branch is gone"}, "branch is gone"},
		{"WithWayForward", fabriccore.Refusal{Reason: "branch is gone", WayForward: "restore it"}, "branch is gone; restore it"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.refusal.Error(); got != tt.want {
				t.Errorf("Error() = %q; want %q", got, tt.want)
			}
		})
	}
}
