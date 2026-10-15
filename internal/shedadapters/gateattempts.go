// gateattempts.go implements the two helpers every gated producer (MultiLLMProducer, BurlerFixProducer, BurlerReviewProducer, WebsterProducer) shares to carry a gate's re-prompt count and spans onto shedengine.OutputPointer.GateAttempts and GateSpans.

package shedadapters

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// gateFailedReason is the one-line Stuck reason for a gate that did not pass, shared by both gated
// producers so the wording never diverges.
// A terminal entry's own explanation (GateOutcome.Reason) replaces the generic text.
func gateFailedReason(gate *shuttleengine.GateOutcome) string {
	if gate.Reason != "" {
		return gate.Reason
	}
	return fmt.Sprintf("gate did not pass after %d attempts; findings: %s", gate.Attempts, gate.FindingsPath)
}

// gateAttemptsPointer returns a pointer to gate.Attempts, or nil when gate itself is nil (the
// producer applied no gate to this call). Read shedengine.OutputPointer.GateAttempts's own doc
// comment for why a nil/non-nil distinction is kept rather than collapsing an ungated call and a
// gate that passed on its first attempt onto the same zero value.
func gateAttemptsPointer(gate *shuttleengine.GateOutcome) *int {
	if gate == nil {
		return nil
	}
	attempts := gate.Attempts
	return &attempts
}

// gateSpansOf flattens gate's entries into the history entry's span list: one span per recorded span, in entry order and, within an entry, span order.
// Each span carries its entry's name, its kind's string and its start and end as RFC 3339 UTC times with fractional seconds.
// A nil gate, and a gate whose entries recorded no span, give nil, so a call that applied no gate and a call whose shuttle recorded nothing keep the field off disk.
func gateSpansOf(gate *shuttleengine.GateOutcome) []shedengine.GateSpan {
	if gate == nil {
		return nil
	}
	var spans []shedengine.GateSpan
	for _, entry := range gate.Entries {
		for _, span := range entry.Spans {
			spans = append(spans, shedengine.GateSpan{
				Gate:  entry.Name,
				Kind:  gateSpanKindName(span.Kind),
				Start: span.Start.UTC().Format(time.RFC3339Nano),
				End:   span.End.UTC().Format(time.RFC3339Nano),
			})
		}
	}
	return spans
}

// gateSpanKindName maps a shuttle span kind onto the history entry's kind string.
func gateSpanKindName(kind shuttleengine.GateSpanKind) string {
	switch kind {
	case shuttleengine.GateSpanHeld:
		return shedengine.GateSpanKindHeld
	case shuttleengine.GateSpanRework:
		return shedengine.GateSpanKindRework
	default:
		return shedengine.GateSpanKindGate
	}
}
