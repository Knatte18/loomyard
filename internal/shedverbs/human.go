// human.go renders the one-shot status for a person at a terminal: a header, the recipe progress,
// the activity lines and the steps still ahead. The JSON envelope stays the answer for every
// non-terminal writer and for --json.

package shedverbs

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// writerIsTerminal reports whether w is a terminal: an *os.File whose Stat mode has
// os.ModeCharDevice. It is a package-level function value so a Tier 1 test can drive the terminal
// branch without a pseudo-terminal.
var writerIsTerminal = func(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// RenderStatusHuman composes the human status view, one trailing newline included.
//
// Lines, in order: the header (label, run-id when non-empty, state); the progress line "step
// <i>/<n> <name>", followed by " | bounce <count>/<budget>" while the current producer sits inside
// a segment, and omitted when routing carries no producers; "now", then "last" when non-empty, then
// "wait" when non-empty and the state is blocked, failed or awaiting; and "next" naming the
// main-line steps still ahead, omitted when none remain.
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

	fmt.Fprintf(&b, "now  %s\n", st.Activity.Now)
	writeActivityTail(&b, st)
	if len(remaining) > 0 {
		fmt.Fprintf(&b, "next %s\n", strings.Join(remaining, ", "))
	}
	return b.String()
}

// writeActivityTail writes the "last" line when non-empty and the "wait" line when non-empty and
// the state is one that halts the run.
func writeActivityTail(b *strings.Builder, st shedengine.Status) {
	if st.Activity.Last != "" {
		fmt.Fprintf(b, "last %s\n", st.Activity.Last)
	}
	switch st.State {
	case shedengine.StateBlocked, shedengine.StateFailed, shedengine.StateAwaiting:
		if st.Activity.Wait != "" {
			fmt.Fprintf(b, "wait %s\n", st.Activity.Wait)
		}
	}
}
