// gateattempts.go holds the helpers that gated producers share.
// They carry a gate's attempt count and spans onto shedengine.OutputPointer.
// The findings-file helpers keep an exhausted gate's findings in a file and name it in the Stuck reason,
// and they clear it once a gate passes.

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

// gateFindingsReasonLimit caps the findings text a Stuck reason carries in runes.
// It applies only when the findings file could not be written.
const gateFindingsReasonLimit = 400

// gateFailedReason is the one-line Stuck reason for a gate that did not pass.
// The gated producers share it so the wording never diverges.
// A terminal entry's own explanation (GateOutcome.Reason) replaces the generic text and writes no file.
// A run that ended with re-prompts left says why, so a gate that failed its first check never reads as a spent budget.
// With a non-empty findingsPath it keeps gate.Findings there, overwriting an earlier file, and names that file;
// a failed write is logged and the reason carries the findings text, cut to one line, in place of the path.
// With an empty findingsPath the reason names gate.FindingsPath.
func gateFailedReason(gate *shuttleengine.GateOutcome, findingsPath string) string {
	if gate.Reason != "" {
		return gate.Reason
	}
	if findingsPath == "" {
		return fmt.Sprintf("%s; findings: %s", gateFailedLead(gate), gate.FindingsPath)
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
	return fmt.Sprintf("%s; findings: %s", gateFailedLead(gate), findingsPath)
}

// gateFailedLead opens the reason of a gate that did not pass with no terminal entry's explanation.
// It counts the re-prompts sent, and when the run ended with re-prompts left it says why.
func gateFailedLead(gate *shuttleengine.GateOutcome) string {
	switch {
	case gate.EndedEarly == "":
		return fmt.Sprintf("gate did not pass after %d attempts", gate.Attempts)
	case gate.Attempts == 0:
		return fmt.Sprintf("gate failed its first check and the session was never re-prompted, since %s", gate.EndedEarly)
	default:
		return fmt.Sprintf("gate did not pass after %d attempts and the run ended with re-prompts left, since %s", gate.Attempts, gate.EndedEarly)
	}
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

// gateAttemptsPointer returns a pointer to gate.Attempts, or nil when gate is nil.
// A nil pointer means the producer applied no gate to the call,
// so an ungated call and a gate that passed on its first attempt stay apart.
func gateAttemptsPointer(gate *shuttleengine.GateOutcome) *int {
	if gate == nil {
		return nil
	}
	attempts := gate.Attempts
	return &attempts
}

// gateSpansOf flattens gate's entries into one span per recorded span, in entry order and, within an entry, span order.
// Each span carries its entry's name and its kind's string;
// its start and end are RFC 3339 UTC times with fractional seconds.
// Both a nil gate and a gate whose entries recorded no span yield nil.
// That keeps the spans field off disk for a call that applied no gate and for a call whose shuttle recorded nothing.
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
