// anomalybody.go implements RenderAnomalyBody, the pure, no-I/O function that renders one
// detected Anomaly into the markdown body of the issue self-report files. Rendering lives here --
// not in the I/O layer -- so both the title (anomaly.go) and the body are table-testable in
// Tier 1.

package loomengine

import (
	"fmt"
	"strings"
)

// RenderAnomalyBody renders a into the markdown body an issue filer posts verbatim. It performs no
// lookup, no read, and no derivation -- every field it renders is already carried on a.
// The rendered markdown must be actionable by someone reading it cold with no access to the
// worktree, because the status file lives under _lyx and the ledgers under .lyx, in a worktree
// that may already be torn down. It carries, in this order: the anomaly kind and a one-line
// statement of what was detected; the task slug and parent branch; the final state, current
// producer, and error verbatim; the relevant history entries rendered as producer/outcome/at rows;
// and, for the recurring-finding kind only, the Bouncer row name plus the ledger entry's key, its
// rounds list, and its status.
// The error text is rendered inside a fenced block so a producer error containing markdown cannot
// corrupt the issue body. The recurring-finding section is omitted entirely for the other four
// kinds rather than emitting empty headings. An empty history renders an explicit "no history
// entries" line rather than an empty section.
func RenderAnomalyBody(a Anomaly) string {
	var b strings.Builder

	fmt.Fprintf(&b, "## %s\n\n", anomalySummary(a))

	fmt.Fprintf(&b, "**Slug:** %s\n", a.Slug)
	fmt.Fprintf(&b, "**Parent:** %s\n\n", a.Parent)

	fmt.Fprintf(&b, "**State:** %s\n", a.State)
	fmt.Fprintf(&b, "**Current producer:** %s\n\n", a.CurrentProducer)

	b.WriteString("**Error:**\n")
	fmt.Fprintf(&b, "```\n%s\n```\n\n", a.Error)

	b.WriteString("**History:**\n")
	if len(a.History) == 0 {
		b.WriteString("no history entries\n")
	} else {
		for _, h := range a.History {
			fmt.Fprintf(&b, "- %s / %s / %s\n", h.Producer, h.Outcome, h.At)
		}
	}

	if a.Kind == AnomalyRecurringFinding {
		b.WriteString("\n**Recurring finding:**\n")
		fmt.Fprintf(&b, "- Bouncer row: %s\n", a.BouncerRow)
		fmt.Fprintf(&b, "- Ledger key: %s\n", a.LedgerKey)
		fmt.Fprintf(&b, "- Rounds: %v\n", a.LedgerRounds)
		fmt.Fprintf(&b, "- Status: %s\n", a.LedgerStatus)
	}

	return b.String()
}

// anomalySummary renders the one-line statement of what was detected, keyed on a.Kind.
func anomalySummary(a Anomaly) string {
	switch a.Kind {
	case AnomalyCrashResume:
		return fmt.Sprintf("%s: the driver was resumed after an unclean exit mid-run", a.Kind)
	case AnomalyEscalation:
		return fmt.Sprintf("%s: the run halted with no OnStuck target and needs a human", a.Kind)
	case AnomalyBudgetExhausted:
		return fmt.Sprintf("%s: the run halted after exhausting a producer's bounce budget", a.Kind)
	case AnomalyProducerFailure:
		return fmt.Sprintf("%s: a producer returned an engine-level failure", a.Kind)
	case AnomalyRecurringFinding:
		return fmt.Sprintf("%s: a ledger finding stayed open across multiple rounds", a.Kind)
	default:
		return string(a.Kind)
	}
}
