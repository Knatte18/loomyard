// recoverbatch_test.go covers the recover-batch verb's refusal texts over an unfinished or failed recovery.

package webstercli

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestBatchFailedMessage pins that the strand's final message precedes the failure and its way forward.
func TestBatchFailedMessage(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	for _, tt := range []struct {
		name        string
		lastMessage string
		want        string
	}{
		{"WithMessage", "Keep the old flag?", `webster: the recovery's last message was "Keep the old flag?"; failed; way forward: x`},
		{"WithoutMessage", "", "failed; way forward: x"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := batchFailedMessage("failed; way forward: x", tt.lastMessage); got != tt.want {
				t.Errorf("batchFailedMessage() = %q; want %q", got, tt.want)
			}
		})
	}
}

// TestRecoveryNotDoneMessage pins that a refusal over an unfinished recovery quotes the strand's final message.
func TestRecoveryNotDoneMessage(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	for _, tt := range []struct {
		name        string
		lastMessage string
		want        string
	}{
		{"WithMessage", "Keep the old flag?", "webster: batch 01-only recovery (session s1) ended dead (its last message: Keep the old flag?); way forward: run `lym webster recover-batch 1` once more"},
		{"WithoutMessage", "", "webster: batch 01-only recovery (session s1) ended dead; way forward: run `lym webster recover-batch 1` once more"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := recoveryNotDoneMessage("01-only", 1, "dead", "s1", tt.lastMessage, true); got != tt.want {
				t.Errorf("recoveryNotDoneMessage() = %q; want %q", got, tt.want)
			}
		})
	}
}
