// playoutside_test.go pins what the outside served to a reed fails its test for at cleanup.

package tmuxrec_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxrec"
)

// heldTB is a test that holds its cleanups for the caller to run, and records its errors instead of failing.
type heldTB struct {
	*testing.T
	cleanups []func()
	errors   []string
}

func (h *heldTB) Cleanup(f func()) { h.cleanups = append(h.cleanups, f) }

func (h *heldTB) Errorf(format string, args ...any) {
	h.errors = append(h.errors, fmt.Sprintf(format, args...))
}

func TestPlayOutside_FailsAtCleanup(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name string
		// asked lists the session names a has-session call is made for.
		asked      []string
		wantErrors []string
	}{
		{"AnAnsweredCall", []string{"pr-bound"}, nil},
		{
			"ACallTheRecordingCannotAnswer",
			[]string{"pr-other"},
			[]string{`recording session-not-started holds no answer to ["tmux has-session -t =pr-other"]`},
		},
		{"NoCallAtAll", nil, []string{"recording session-not-started answered no call"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			held := &heldTB{T: t}
			outside := tmuxrec.PlayOutside(held, "session-not-started", "pr-bound")

			for _, session := range tt.asked {
				// The recorded answer is itself a failure, so a refusal shows only at cleanup.
				_, _ = outside.Tmux(false, nil, "has-session", "-t", "="+session)
			}
			for _, cleanup := range slices.Backward(held.cleanups) {
				cleanup()
			}

			if diff := cmp.Diff(tt.wantErrors, held.errors); diff != "" {
				t.Errorf("errors at cleanup mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
