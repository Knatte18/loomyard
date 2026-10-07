// messagetail_test.go verifies WithMessageTail appends the fixed tail exactly once.

package shuttleengine_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

func TestWithMessageTail(t *testing.T) {
	t.Parallel()

	tail := shuttleengine.MessageTail
	tests := []struct {
		name string
		text string
		want string
	}{
		{"plain text gains the tail", "read the plan", "read the plan " + tail},
		{"text ending with the tail is unchanged", "read the plan " + tail, "read the plan " + tail},
		{"tail followed by spaces is unchanged", "read the plan " + tail + "  ", "read the plan " + tail + "  "},
		{"tail followed by a tab is unchanged", "read the plan " + tail + "\t", "read the plan " + tail + "\t"},
		{"tail mid-sentence gains the tail", tail + " Also check the build.", tail + " Also check the build. " + tail},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := shuttleengine.WithMessageTail(tt.text); got != tt.want {
				t.Errorf("WithMessageTail(%q) = %q, want %q", tt.text, got, tt.want)
			}
		})
	}
}
