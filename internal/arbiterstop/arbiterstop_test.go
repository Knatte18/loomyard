//go:build tier0

// arbiterstop_test.go pins the eligibility answer the Go driver's escalation fact takes.

package arbiterstop

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// TestCheck_RefusesLandingHalts pins that Check refuses a blocked landing halt with a refusal ending in HandBackWayForward, and admits a Webster reroute-cap halt and an ordinary blocked halt.
func TestCheck_RefusesLandingHalts(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		producer string
		reason   string
		wantErr  bool
	}{
		{name: "landing halt at Finalize", producer: "Finalize", reason: landingshed.LandingStopPrefix + "the fixer escalated; way forward: resolve it, then `lyx loom resume`", wantErr: true},
		{name: "reroute-cap halt at Webster", producer: "Webster", reason: shedengine.ReasonRerouteCapReached + ": Webster rerouted to Plan-Write 3 times; way forward: x"},
		{name: "ordinary blocked halt", producer: "Plan-Write", reason: "a plain halt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			readers := Readers{
				PendingEscalation: func(string) (shedadapters.EscalationCause, string, bool, error) { return "", "", false, nil },
				InterruptPolicy:   func(string) string { return "" },
			}
			status := shedengine.Status{State: shedengine.StateBlocked, CurrentProducer: tt.producer, Error: tt.reason}

			_, err := Check(status, "", "", readers)

			if (err != nil) != tt.wantErr {
				t.Fatalf("Check() error = %v; want an error: %v", err, tt.wantErr)
			}
			if tt.wantErr && !strings.HasSuffix(err.Error(), HandBackWayForward) {
				t.Errorf("Check() error = %q; want it to end with %q", err, HandBackWayForward)
			}
		})
	}
}

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
