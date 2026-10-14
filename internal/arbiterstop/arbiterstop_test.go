// arbiterstop_test.go pins the eligibility answer the Go driver's escalation fact takes.

package arbiterstop

import (
	"errors"
	"fmt"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedadapters"
)

// TestEscalationEligible pins EscalationEligible over fake readers: only a pending hard-cap or circling escalation is eligible.
func TestEscalationEligible(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		// producer is the awaiting row; empty means a Bouncer row.
		producer string
		cause    shedadapters.EscalationCause
		pending  bool
		err      error
		want     bool
	}{
		{name: "pending hard-cap", cause: shedadapters.EscalationHardCap, pending: true, want: true},
		{name: "pending circling", cause: shedadapters.EscalationCircling, pending: true, want: true},
		{name: "pending budget", cause: shedadapters.EscalationBudget, pending: true},
		{name: "malformed record", err: fmt.Errorf("record: %w", shedadapters.ErrEscalationMalformed)},
		{name: "plain read failure", err: errors.New("disk gone")},
		{name: "nothing pending at a Bouncer"},
		{name: "non-Bouncer row", producer: "Plan-Write"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			readers := Readers{
				PendingEscalation: func(string) (shedadapters.EscalationCause, string, bool, error) {
					return tt.cause, "brief.md", tt.pending, tt.err
				},
				InterruptPolicy: func(string) string { return "" },
			}
			producer := tt.producer
			if producer == "" {
				producer = "Plan-Bouncer"
			}
			if got := EscalationEligible(producer, readers); got != tt.want {
				t.Errorf("EscalationEligible = %v; want %v", got, tt.want)
			}
		})
	}
}
