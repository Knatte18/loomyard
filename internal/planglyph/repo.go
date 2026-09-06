// repo.go is planglyph's one call site for quarry.Open and (*quarry.Repo).Resolve — the package's
// two entry points into quarry.Repo — and declares Finding, this package's own finding type.

package planglyph

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/quarry/quarry"
)

// Severity is the closed vocabulary a Finding's own Severity is drawn from.
type Severity string

const (
	// SeverityBlocking marks a finding that fails the gate it is reported against.
	SeverityBlocking Severity = "blocking"
	// SeverityInformational marks a finding surfaced for visibility that never fails a gate.
	SeverityInformational Severity = "informational"
)

// Finding is this package's own finding type: the same Check, Card and Detail fields
// planparser.ValidationError carries, plus a Severity every converted planparser finding is
// stamped with by fromValidationError.
type Finding struct {
	Check    string
	Card     string
	Detail   string
	Severity Severity
}

// fromValidationError converts v into a Finding stamped SeverityBlocking — the severity every
// planparser check reports today, per the plan's blocking-policy Shared Decision.
func fromValidationError(v planparser.ValidationError) Finding {
	return Finding{Check: v.Check, Card: v.Card, Detail: v.Detail, Severity: SeverityBlocking}
}

// openRepo opens a quarry.Repo rooted at worktreeRoot, wrapping any error with a "planglyph:"
// prefix. It is this package's one call to quarry.Open.
func openRepo(worktreeRoot string) (*quarry.Repo, error) {
	repo, err := quarry.Open(worktreeRoot)
	if err != nil {
		return nil, fmt.Errorf("planglyph: open %q: %w", worktreeRoot, err)
	}
	return repo, nil
}

// resolveTargets resolves every entry of targets against repo, positionally, wrapping any error
// with a "planglyph:" prefix. It is this package's one call to (*quarry.Repo).Resolve.
func resolveTargets(repo *quarry.Repo, targets []string) ([]quarry.ResolveResult, error) {
	results, err := repo.Resolve(targets)
	if err != nil {
		return nil, fmt.Errorf("planglyph: resolve: %w", err)
	}
	return results, nil
}
