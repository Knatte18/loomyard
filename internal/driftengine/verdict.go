// verdict.go renders a task's regression check from its scope record, as the markdown section a landing carries in its pull request and landing commit.

package driftengine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

// verdictHeading opens the section Verdict renders.
const verdictHeading = "## Regression check"

// acceptedByText names who made an acceptance, keyed by Acceptance.By.
var acceptedByText = map[string]string{
	AcceptedByAgent:    "the Drift-Verify agent",
	AcceptedByCaller:   "the run's parent",
	AcceptedByOperator: "the operator",
}

// Verdict renders the task's regression check as a markdown section:
// the scope with its skipped tests, the commit the baseline was captured at, the last compare's result or the waiver, and each accepted drifted test with who accepted it and why.
// It returns "" when no scope record exists, so a task no Drift row recorded a scope for carries no section.
func Verdict(p Paths) (string, error) {
	// The file is stat'ed first because ReadScope creates the ephemeral drift directory for its lock, which a run of a recipe with no Drift rows never needs.
	if _, err := os.Stat(p.Scope); errors.Is(err, fs.ErrNotExist) {
		return "", nil
	} else if err != nil {
		return "", fmt.Errorf("drift: stat the scope record %s: %w", p.Scope, err)
	}
	scope, found, err := ReadScope(p)
	if err != nil || !found {
		return "", err
	}
	return renderVerdict(scope), nil
}

// renderVerdict renders scope as Verdict's section.
func renderVerdict(scope ScopeRecord) string {
	lines := []string{verdictHeading, "", "- Scope: " + scopeText(scope)}
	if scope.Scope == ScopeOff {
		return strings.Join(lines, "\n")
	}
	if scope.BaselineHead != "" {
		lines = append(lines, fmt.Sprintf("- Baseline: captured at `%s`, the task's branch point.", scope.BaselineHead))
	}
	lines = append(lines, "- Compare: "+compareText(scope))
	if len(scope.Accepted) == 0 {
		return strings.Join(append(lines, "- Accepted drift: none."), "\n")
	}
	lines = append(lines, "- Accepted drift:")
	for _, acceptance := range scope.Accepted {
		line := fmt.Sprintf("  - `%s`, accepted by %s: %s", acceptance.Test, acceptedByText[acceptance.By], acceptance.Reason)
		if acceptance.Ruling == RulingUnruled {
			line += " (not yet ruled on by the run's parent)"
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// scopeText renders the scope line's value.
func scopeText(scope ScopeRecord) string {
	switch scope.Scope {
	case ScopeOff:
		return "off; the task is not regression-checked."
	case ScopePartial:
		quoted := make([]string, len(scope.Skip))
		for i, test := range scope.Skip {
			quoted[i] = "`" + test + "`"
		}
		return "partial, skipping " + strings.Join(quoted, ", ") + "."
	}
	return "full."
}

// compareText renders the compare line's value.
func compareText(scope ScopeRecord) string {
	switch {
	case scope.WaivedAt != "":
		return "waived: " + scope.WaivedReason
	case scope.ComparedAt == "":
		return "not run."
	case scope.ComparePassed:
		return fmt.Sprintf("passed at `%s`.", scope.ComparedHead)
	}
	return fmt.Sprintf("showed drift at `%s`.", scope.ComparedHead)
}
