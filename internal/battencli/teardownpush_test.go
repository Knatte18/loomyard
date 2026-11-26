// teardownpush_test.go pins the sentinels the teardown's push maps its refusals to.

package battencli

import (
	"errors"
	"fmt"
	"testing"

	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestTeardownPushError(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	refused := errors.New("push refused")
	tests := []struct {
		name string
		// cause is wrapped into the push's error, and nil means the push succeeded.
		cause error
		// wantSentinel is the sentinel the mapped error must match, nil for none.
		wantSentinel error
	}{
		{name: "NoErrorPassesThrough"},
		{name: "EndedEntryBecomesTheEndedSentinel", cause: fabricengine.ErrEntryEnded, wantSentinel: battenshed.ErrPushEntryEnded},
		{name: "HandOverBecomesTheHandOverSentinel", cause: fabricengine.ErrHandedOver, wantSentinel: battenshed.ErrPushHandedOver},
		{name: "AnyOtherErrorPassesThrough", cause: refused},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var pushErr error
			if tt.cause != nil {
				pushErr = fmt.Errorf("push of the pair: %w", tt.cause)
			}

			got := teardownPushError(pushErr)

			if tt.cause == nil {
				if got != nil {
					t.Fatalf("teardownPushError(nil) = %v; want nil", got)
				}
				return
			}
			if !errors.Is(got, tt.cause) {
				t.Errorf("teardownPushError() = %v; want it to keep wrapping %v", got, tt.cause)
			}
			for _, sentinel := range []error{battenshed.ErrPushEntryEnded, battenshed.ErrPushHandedOver} {
				if want := sentinel == tt.wantSentinel; errors.Is(got, sentinel) != want {
					t.Errorf("errors.Is(teardownPushError(), %v) = %v; want %v", sentinel, !want, want)
				}
			}
		})
	}
}
