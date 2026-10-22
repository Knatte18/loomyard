// gateattempts.go implements the helpers every gated producer (MultiLLMProducer, BurlerFixProducer, BurlerReviewProducer, WebsterProducer) shares to carry a gate's re-prompt count and spans onto shedengine.OutputPointer.GateAttempts and GateSpans.
// It also holds the findings-file helpers: keeping an exhausted gate's findings in a told file, naming it in the Stuck reason and clearing it once a gate passes.

package shedadapters

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// gateFindingsReasonLimit is the most characters of the gate's findings text a Stuck reason carries when the findings file could not be written.
const gateFindingsReasonLimit = 400

// gateFailedReason is the one-line Stuck reason for a gate that did not pass, shared by both gated
// producers so the wording never diverges.
// A terminal entry's own explanation (GateOutcome.Reason) replaces the generic text and writes no file.
// With a non-empty findingsPath it keeps gate.Findings there, overwriting an earlier file, and names that file;
// a failed write is logged and the reason carries the findings text, cut to one line, in place of the path.
// With an empty findingsPath the reason names gate.FindingsPath.
func gateFailedReason(gate *shuttleengine.GateOutcome, findingsPath string) string {
	if gate.Reason != "" {
		return gate.Reason
	}
	if findingsPath == "" {
		return fmt.Sprintf("gate did not pass after %d attempts; findings: %s", gate.Attempts, gate.FindingsPath)
	}
	err := os.MkdirAll(filepath.Dir(findingsPath), 0o755)
	if err == nil {
		err = os.WriteFile(findingsPath, []byte(gate.Findings), 0o644)
	}
	if err != nil {
		logger.Warn("shedadapters: gate findings were not kept", "findingsPath", findingsPath, "error", err)
		text := strings.Join(strings.Fields(gate.Findings), " ")
		if runes := []rune(text); len(runes) > gateFindingsReasonLimit {
			text = string(runes[:gateFindingsReasonLimit])
		}
		return fmt.Sprintf("findings could not be kept at %s: %v; findings: %s", findingsPath, err, text)
	}
	return fmt.Sprintf("gate did not pass after %d attempts; findings: %s", gate.Attempts, findingsPath)
}

// clearGateFindings removes the gate findings file at path, so a call whose gate passed leaves none behind.
// An empty path and an absent file are no-ops, and any other removal failure is logged and changes no verdict.
func clearGateFindings(path string) {
	if path == "" {
		return
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		logger.Warn("shedadapters: gate findings file was not removed", "findingsPath", path, "error", err)
	}
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
