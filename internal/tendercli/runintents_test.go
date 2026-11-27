// runintents_test.go pins how the pause and resume verbs answer the push of a run's intent.

package tendercli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestOwedIntentPush asserts a push that is owed is answered on stderr and any other failure is returned.
// The owed branch runs here with a built error, since a real unreachable origin waits out the push backoff.
func TestOwedIntentPush(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	failure := func(class fabriclayout.PushFailureClass) error {
		return &fabricengine.PushFailedError{Record: fabriclayout.FailedPush{Class: class}, Err: errors.New("push refused")}
	}
	notFastForward := failure(fabriclayout.PushFailureNotFastForward)
	tests := []struct {
		name       string
		err        error
		wantErr    error
		wantOutput []string
	}{
		{name: "NoError"},
		{name: "OwedTransportFailure", err: failure(fabriclayout.PushFailureTransport), wantOutput: []string{"some-slug", "push refused", "lym fabric push"}},
		{name: "NotFastForwardIsReturned", err: notFastForward, wantErr: notFastForward},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var stderr bytes.Buffer

			err := owedIntentPush("some-slug", tt.err, &stderr)

			if err != tt.wantErr {
				t.Errorf("owedIntentPush() error = %v; want %v", err, tt.wantErr)
			}
			if len(tt.wantOutput) == 0 && stderr.Len() != 0 {
				t.Errorf("owedIntentPush() wrote %q; want nothing", stderr.String())
			}
			for _, fragment := range tt.wantOutput {
				if !strings.Contains(stderr.String(), fragment) {
					t.Errorf("owedIntentPush() wrote %q; want it to contain %q", stderr.String(), fragment)
				}
			}
		})
	}
}
