// human.go renders the one-shot status for a person at a terminal: a header, the recipe progress,
// the activity lines and the steps still ahead. The JSON envelope stays the answer for every
// non-terminal writer and for --json.

package shedverbs

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// writerIsTerminal reports whether w is a terminal by the check every operator verb shares.
// It is a package-level function value so a Tier 1 test can drive the terminal
// branch without a pseudo-terminal.
var writerIsTerminal = clihelp.WriterIsTerminal

// RenderStatusHuman composes the human status view, one trailing newline included.
//
// Lines, in order: the header (label, run-id when non-empty, state); the progress line "step
// <i>/<n> <name>", followed by " | bounce <count>/<budget>" while the current producer sits inside
// a segment, and omitted when routing carries no producers; "now", then "last" when non-empty, then
// "wait" when non-empty and the state is blocked, failed or awaiting; and "next" naming the
// main-line steps still ahead, omitted when none remain.
// A paused run with a recorded reason prints a "why" line after "last".
// The "now" line names the current row's block when its routing definition carries one, and the "last" line reads a block row's Done as entered and names an inlined row's block.
// A non-empty waiting note prints "waiting <note>" in the header in place of the state.
func RenderStatusHuman(label, runID string, st shedengine.Status, routing shedengine.Routing, waiting string) string {
	var b strings.Builder

	header := label
	if runID != "" {
		header += " " + runID
	}
	if waiting != "" {
		fmt.Fprintf(&b, "%s | waiting %s\n", header, waiting)
	} else {
		fmt.Fprintf(&b, "%s | %s\n", header, st.State)
	}

	var remaining []string
	if len(routing.Producers) > 0 {
		p := routing.ProgressAt(st.CurrentProducer)
		line := fmt.Sprintf("step %d/%d %s", p.Step, p.Steps, p.Name)
		if count, budget, inSegment := routing.Bounces(st.CurrentProducer, st.History); inSegment {
			line += fmt.Sprintf(" | bounce %d/%d", count, budget)
		}
		b.WriteString(line + "\n")
		remaining = p.Remaining
	}

	now := st.Activity.Now
	if def, inRouting := routingDef(routing, st.CurrentProducer); inRouting && def.Block != "" && st.State != shedengine.StateDone {
		now += " in " + def.Block
	}
	fmt.Fprintf(&b, "now  %s\n", now)
	st.Activity.Last = blockAwareLast(st, routing)
	writeActivityTail(&b, st)
	if len(remaining) > 0 {
		fmt.Fprintf(&b, "next %s\n", strings.Join(remaining, ", "))
	}
	return b.String()
}

// routingDef returns the routing's definition of the row named name, and false when it holds none.
func routingDef(routing shedengine.Routing, name string) (shedengine.ProducerDef, bool) {
	for _, def := range routing.Producers {
		if def.Name == name {
			return def, true
		}
	}
	return shedengine.ProducerDef{}, false
}

// blockAwareLast returns the status's "last" line read against its blocks.
// A latest history entry that is a Done of a row whose routing definition carries a skip target, a block row, reads "<row> → entered";
// one that carries a block names its row "<row> in <block>" in the activity's line.
// Any other run keeps the activity's line as it is.
func blockAwareLast(st shedengine.Status, routing shedengine.Routing) string {
	last := st.Activity.Last
	if len(st.History) == 0 {
		return last
	}
	latest := st.History[len(st.History)-1]
	if def, inRouting := routingDef(routing, latest.Producer); inRouting && latest.Outcome == shedengine.Done && def.SkipTo != "" {
		return fmt.Sprintf("%s → entered", latest.Producer)
	}
	if latest.Block != "" && strings.HasPrefix(last, latest.Producer) {
		return latest.Producer + " in " + latest.Block + strings.TrimPrefix(last, latest.Producer)
	}
	return last
}

// writeActivityTail writes the "last" line when non-empty, the "why" line when the run is paused with a recorded reason,
// and the "wait" line when non-empty and the state is one that halts the run.
func writeActivityTail(b *strings.Builder, st shedengine.Status) {
	if st.Activity.Last != "" {
		fmt.Fprintf(b, "last %s\n", st.Activity.Last)
	}
	if st.State == shedengine.StatePaused && st.PauseReason != "" {
		fmt.Fprintf(b, "why  %s\n", st.PauseReason)
	}
	switch st.State {
	case shedengine.StateBlocked, shedengine.StateFailed, shedengine.StateAwaiting:
		if st.Activity.Wait != "" {
			fmt.Fprintf(b, "wait %s\n", st.Activity.Wait)
		}
	}
}
