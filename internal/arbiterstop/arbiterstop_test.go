//go:build tier0

// arbiterstop_test.go pins the eligibility answer the Go driver's escalation fact takes.

package arbiterstop

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedverbs"
)

// TestCheck_RefusesLandingHalts pins that Check refuses a blocked landing halt with a refusal ending in HandBackWayForward, and admits a Webster reroute-cap halt and an ordinary blocked halt.
func TestCheck_RefusesLandingHalts(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		producer string
		reason   string
		wantErr  bool
		// wantHandsBack is HandsBack's answer over producer and reason.
		wantHandsBack bool
	}{
		{name: "landing halt at Finalize", producer: "Finalize", reason: landingshed.LandingStopPrefix + "the fixer escalated; way forward: resolve it, then `lym loom resume`", wantErr: true, wantHandsBack: true},
		{name: "decision halt at Discussion-Bouncer", producer: "Discussion-Bouncer", reason: shedadapters.DecisionHaltPrefix + "bouncer Discussion-Bouncer: round 2's recorded accept is held by a refusal; way forward: x", wantErr: true, wantHandsBack: true},
		{name: "blocked halt at Discussion-Bouncer without the prefix", producer: "Discussion-Bouncer", reason: "a plain halt"},
		{name: "reroute-cap halt at Webster", producer: "Webster", reason: shedengine.ReasonRerouteCapReached + ": Webster rerouted to Plan-Write 3 times; way forward: x"},
		{name: "ordinary blocked halt", producer: "Plan-Write", reason: "a plain halt"},
		{name: "escalate halt at Finalize", producer: "Finalize", reason: landingshed.EscalateStopPrefix + "the fixer of Finalize found a parent change that conflicts with a decision of the record; way forward: record the deciding decision, then `lym loom resume`"},
		{name: "escalate halt at Parent-Align", producer: "Parent-Align", reason: landingshed.EscalateStopPrefix + "the fixer of Parent-Align found a parent change that conflicts with a decision of the record; way forward: record the deciding decision, then `lym loom resume`"},
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
			if got := HandsBack(tt.producer, tt.reason); got != tt.wantHandsBack {
				t.Errorf("HandsBack(%q, %q) = %v; want %v", tt.producer, tt.reason, got, tt.wantHandsBack)
			}
		})
	}
}

// TestEligibility_BattenRule pins batten's rule through Check: a tackler-escalated stop is eligible unless it is a spent budget, an unmet precondition or a pause-honoured Land stop, which end in BattenHandBackWayForward, while loom-shaped readers keep loom's answer for the budget and precondition stops.
func TestEligibility_BattenRule(t *testing.T) {
	t.Parallel()
	loomReaders := Readers{
		PendingEscalation: func(string) (shedadapters.EscalationCause, string, bool, error) { return "", "", false, nil },
		InterruptPolicy:   func(string) string { return "" },
	}
	tests := []struct {
		name     string
		readers  Readers
		status   shedengine.Status
		kind     string
		wantErr  bool
		wantWays string
	}{
		{name: "blocked Land with a deploy error", readers: NewBattenReaders(), status: shedengine.Status{State: shedengine.StateBlocked, CurrentProducer: battenrecipe.NameLand, Error: "deploy failed"}},
		{name: "failed Worktree-Teardown of kind producer", readers: NewBattenReaders(), status: shedengine.Status{State: shedengine.StateFailed, CurrentProducer: battenrecipe.NameWorktreeTeardown, Error: "teardown failed"}, kind: shedverbs.KindProducer},
		{name: "spent budget", readers: NewBattenReaders(), status: shedengine.Status{State: shedengine.StateBlocked, CurrentProducer: "Code-Write", Error: shedengine.ReasonBounceBudgetExhausted + ": x"}, wantErr: true, wantWays: BattenHandBackWayForward},
		{name: "unmet precondition", readers: NewBattenReaders(), status: shedengine.Status{State: shedengine.StateBlocked, CurrentProducer: "Code-Write", Error: shedengine.ReasonPreconditionUnmet + ": x"}, wantErr: true, wantWays: BattenHandBackWayForward},
		{name: "pause-honoured Land stop", readers: NewBattenReaders(), status: shedengine.Status{State: shedengine.StateBlocked, CurrentProducer: battenrecipe.NameLand, Error: battenshed.PauseHonouredPrefix + ": a pause was requested"}, wantErr: true, wantWays: BattenHandBackWayForward},
		{name: "loom keeps a spent budget eligible", readers: loomReaders, status: shedengine.Status{State: shedengine.StateBlocked, CurrentProducer: "Code-Write", Error: shedengine.ReasonBounceBudgetExhausted + ": x"}},
		{name: "loom keeps an unmet precondition eligible", readers: loomReaders, status: shedengine.Status{State: shedengine.StateBlocked, CurrentProducer: "Code-Write", Error: shedengine.ReasonPreconditionUnmet + ": x"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := Check(tt.status, tt.kind, "", tt.readers)

			if (err != nil) != tt.wantErr {
				t.Fatalf("Check() error = %v; want an error: %v", err, tt.wantErr)
			}
			if tt.wantErr && !strings.HasSuffix(err.Error(), tt.wantWays) {
				t.Errorf("Check() error = %q; want it to end with %q", err, tt.wantWays)
			}
		})
	}
}

// TestEscalationEligible pins EscalationEligible over fake readers: only a pending hard-cap or circling escalation is eligible.
// Every refused row also runs Check over an awaiting status and wants the refusal to end with HandBackWayForward.
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
		{name: "pending decision", cause: shedadapters.EscalationDecision, pending: true},
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
			if !tt.want {
				status := shedengine.Status{State: shedengine.StateAwaiting, CurrentProducer: producer}
				_, err := Check(status, "", "", readers)
				if err == nil || !strings.HasSuffix(err.Error(), HandBackWayForward) {
					t.Errorf("Check(awaiting) error = %v; want a refusal ending with %q", err, HandBackWayForward)
				}
			}
		})
	}
}
