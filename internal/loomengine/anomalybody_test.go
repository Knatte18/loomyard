// anomalybody_test.go is the TDD driver for RenderAnomalyBody: table tests over hand-built Anomaly
// values, asserting the required content and its determinism. It is untagged (Tier 1): no spawn,
// no git, no filesystem I/O -- RenderAnomalyBody is pure.

package loomengine

import (
	"strconv"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// allKinds lists every AnomalyKind, used to iterate the "contains slug and parent" and
// "no recurring-finding leakage" assertions across all five kinds.
var allKinds = []AnomalyKind{
	AnomalyCrashResume,
	AnomalyEscalation,
	AnomalyBudgetExhausted,
	AnomalyProducerFailure,
	AnomalyRecurringFinding,
}

// anomalyOfKind returns a fully-populated Anomaly of the given kind for body-rendering tests.
func anomalyOfKind(kind AnomalyKind) Anomaly {
	a := Anomaly{
		Kind:            kind,
		Title:           "loom anomaly: " + string(kind) + " — loom-contracts — Loom-Preflight#0",
		Slug:            "loom-contracts",
		Parent:          "main",
		State:           shedengine.StateBlocked,
		CurrentProducer: "Discussion-Review",
		Error:           "bounce budget exhausted",
		History: []shedengine.HistoryEntry{
			{Producer: "Discussion-Review", Outcome: shedengine.Stuck, At: "2026-07-17T10:01:30Z"},
		},
	}
	if kind == AnomalyRecurringFinding {
		a.BouncerRow = "Discussion-Review"
		a.LedgerKey = "finding-a"
		a.LedgerRounds = []int{1, 2, 3}
		a.LedgerStatus = "open"
	}
	return a
}

func TestRenderAnomalyBody_ContainsSlugAndParent(t *testing.T) {
	for _, kind := range allKinds {
		t.Run(string(kind), func(t *testing.T) {
			body := RenderAnomalyBody(anomalyOfKind(kind))
			if !strings.Contains(body, "loom-contracts") {
				t.Errorf("body for kind %q missing slug: %s", kind, body)
			}
			if !strings.Contains(body, "main") {
				t.Errorf("body for kind %q missing parent: %s", kind, body)
			}
		})
	}
}

func TestRenderAnomalyBody_HaltKindContainsStateProducerAndError(t *testing.T) {
	haltKinds := []AnomalyKind{AnomalyEscalation, AnomalyBudgetExhausted, AnomalyProducerFailure}
	for _, kind := range haltKinds {
		t.Run(string(kind), func(t *testing.T) {
			a := anomalyOfKind(kind)
			body := RenderAnomalyBody(a)
			if !strings.Contains(body, string(a.State)) {
				t.Errorf("body for kind %q missing state %q: %s", kind, a.State, body)
			}
			if !strings.Contains(body, a.CurrentProducer) {
				t.Errorf("body for kind %q missing current producer %q: %s", kind, a.CurrentProducer, body)
			}
			if !strings.Contains(body, a.Error) {
				t.Errorf("body for kind %q missing error text %q: %s", kind, a.Error, body)
			}
		})
	}
}

func TestRenderAnomalyBody_RecurringFindingSection(t *testing.T) {
	recurring := anomalyOfKind(AnomalyRecurringFinding)
	body := RenderAnomalyBody(recurring)

	if !strings.Contains(body, recurring.BouncerRow) {
		t.Errorf("recurring-finding body missing Bouncer row: %s", body)
	}
	if !strings.Contains(body, recurring.LedgerKey) {
		t.Errorf("recurring-finding body missing ledger key: %s", body)
	}
	for _, round := range recurring.LedgerRounds {
		if !strings.Contains(body, strconv.Itoa(round)) {
			t.Errorf("recurring-finding body missing round %d: %s", round, body)
		}
	}
	if !strings.Contains(body, recurring.LedgerStatus) {
		t.Errorf("recurring-finding body missing ledger status: %s", body)
	}

	for _, kind := range allKinds {
		if kind == AnomalyRecurringFinding {
			continue
		}
		t.Run("NoLeakage_"+string(kind), func(t *testing.T) {
			body := RenderAnomalyBody(anomalyOfKind(kind))
			if strings.Contains(body, recurring.BouncerRow) && strings.Contains(body, recurring.LedgerKey) {
				t.Errorf("body for non-recurring kind %q unexpectedly carries recurring-finding fields: %s", kind, body)
			}
			if strings.Contains(body, "Recurring finding:") {
				t.Errorf("body for non-recurring kind %q unexpectedly renders the recurring-finding heading: %s", kind, body)
			}
		})
	}
}

func TestRenderAnomalyBody_HistoryRows(t *testing.T) {
	a := anomalyOfKind(AnomalyEscalation)
	a.History = []shedengine.HistoryEntry{
		{Producer: "Discussion-Review", Outcome: shedengine.Stuck, At: "2026-07-17T10:01:30Z"},
		{Producer: "Discussion-Review", Outcome: shedengine.Done, At: "2026-07-17T09:00:00Z"},
	}
	body := RenderAnomalyBody(a)
	for _, h := range a.History {
		if !strings.Contains(body, h.Producer) || !strings.Contains(body, string(h.Outcome)) || !strings.Contains(body, h.At) {
			t.Errorf("body missing a history row for %+v: %s", h, body)
		}
	}
}

func TestRenderAnomalyBody_EmptyHistory(t *testing.T) {
	a := anomalyOfKind(AnomalyEscalation)
	a.History = nil
	body := RenderAnomalyBody(a)
	if !strings.Contains(body, "no history entries") {
		t.Errorf("body with empty history missing the explicit no-entries line: %s", body)
	}
}

func TestRenderAnomalyBody_Deterministic(t *testing.T) {
	a := anomalyOfKind(AnomalyRecurringFinding)
	first := RenderAnomalyBody(a)
	second := RenderAnomalyBody(a)
	if first != second {
		t.Errorf("RenderAnomalyBody() is non-deterministic:\nfirst:  %q\nsecond: %q", first, second)
	}
}
